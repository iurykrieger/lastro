package sensor

import (
	"errors"
	"fmt"

	"github.com/iurykrieger/lastro/internal/stack"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// ValidateAgainstStack asserts that every id in s.Uses is present in
// the given stack manifest (grounding invariant 1: sensors may only
// reference detected components). Returns a joined error naming every
// unknown id; returns nil when s is fully grounded.
func ValidateAgainstStack(s Sensor, manifest stack.StackManifest) error {
	var errs []error
	for _, id := range s.Uses {
		if _, ok := manifest.ByID(id); !ok {
			errs = append(errs, fmt.Errorf("uses references unknown stack component %q", id))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// ValidateAgainstFixtures asserts that every fixture id referenced via
// ${{ fixtures.<id> }} in any step's run command or with values is in
// the set of fixtures the sensor's use case owns (grounding invariant 2).
// Errors are reported per step, naming the step id and every unknown fixture
// referenced in that step.
//
// A nil return from owner.OwnedFixtureIDs (e.g., use case unknown) is
// treated as the empty set, which causes every step with a fixture
// reference to fail. That's intentional — a sensor pointing at a
// missing use case should not ground silently.
func ValidateAgainstFixtures(s Sensor, owner UseCaseFixtureOwnership) error {
	owned := make(map[string]bool)
	for _, id := range owner.OwnedFixtureIDs(s.UseCaseID) {
		owned[id] = true
	}
	var errs []error
	for _, st := range s.Steps {
		refs, err := collectStepFixtureRefs(st)
		if err != nil {
			errs = append(errs, fmt.Errorf("step %q: %w", st.ID, err))
			continue
		}
		var unknown []string
		for _, id := range refs {
			if !owned[id] {
				unknown = append(unknown, id)
			}
		}
		if len(unknown) > 0 {
			errs = append(errs, fmt.Errorf("step %q: unknown fixture(s) %v", st.ID, unknown))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// collectStepFixtureRefs returns the fixture ids referenced by a step's run
// and with values via ${{ fixtures.<id> }}.
func collectStepFixtureRefs(st Step) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	scan := func(src string) error {
		segs, err := template.Parse(src)
		if err != nil {
			return err
		}
		_, refs, err := template.Compile(segs)
		if err != nil {
			return err
		}
		for _, id := range refs.Fixtures {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		return nil
	}
	if err := scan(st.Run); err != nil {
		return nil, err
	}
	for _, v := range st.With {
		if err := scan(v); err != nil {
			return nil, err
		}
	}
	return out, nil
}
