package sensor

import (
	"errors"
	"fmt"

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
