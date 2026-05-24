package sensor

import (
	"errors"
	"fmt"

	"github.com/iurykrieger/lastro/internal/stack"
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

// ValidateAgainstFixtures asserts that every fixture id referenced by
// any step is in the set of fixtures the sensor's use case owns
// (grounding invariant 2). Errors are reported per step, naming the
// step id and every unknown fixture in that step.
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
		var unknown []string
		for _, id := range st.Uses {
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
