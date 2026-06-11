package sensor

import (
	"errors"
	"fmt"
	"sort"

	"github.com/iurykrieger/lastro/internal/enums"
)

// ValidateComposition checks every uses-step in s: the target exists, is
// scope=core, and every required input it declares is satisfied by the
// step's `with` or by an input default. Call after the global Store is built.
func ValidateComposition(s Sensor, store *Store) error {
	var errs []error
	for _, st := range s.Steps {
		if st.Uses == "" {
			continue
		}
		prim, ok := store.LookupSensor(st.Uses)
		if !ok {
			errs = append(errs, fmt.Errorf("step %q: uses unknown sensor %q", st.ID, st.Uses))
			continue
		}
		if prim.Scope != enums.ScopeCore {
			errs = append(errs, fmt.Errorf("step %q: uses %q which is not scope=core", st.ID, st.Uses))
			continue
		}
		for name, spec := range prim.Inputs {
			if !spec.Required {
				continue
			}
			if _, bound := st.With[name]; bound {
				continue
			}
			if spec.HasDefault {
				continue
			}
			errs = append(errs, fmt.Errorf("step %q: required input %q of %q is unbound", st.ID, name, st.Uses))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// ValidateWithKeys checks every uses-step in s: each `with:` key must be a
// declared input of the composed core primitive. An undeclared key would
// silently bind nothing — the caller must evolve the core primitive first
// (add the input with a backward-compatible default, re-persist it), then
// retry. Unknown uses-targets are skipped here; ValidateComposition owns
// that finding.
func ValidateWithKeys(s Sensor, store *Store) error {
	var errs []error
	for _, st := range s.Steps {
		if st.Uses == "" {
			continue
		}
		prim, ok := store.LookupSensor(st.Uses)
		if !ok {
			continue
		}
		var unknown []string
		for key := range st.With {
			if _, declared := prim.Inputs[key]; !declared {
				unknown = append(unknown, key)
			}
		}
		if len(unknown) == 0 {
			continue
		}
		sort.Strings(unknown)
		declared := make([]string, 0, len(prim.Inputs))
		for name := range prim.Inputs {
			declared = append(declared, name)
		}
		sort.Strings(declared)
		errs = append(errs, fmt.Errorf(
			"step %q: with key(s) %v are not declared inputs of %q (declared: %v) — evolve the core primitive first: add the input with a default, re-persist it, then retry",
			st.ID, unknown, st.Uses, declared))
	}
	return errors.Join(errs...)
}
