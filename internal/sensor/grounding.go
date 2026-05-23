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
