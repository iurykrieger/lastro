package sensor

import (
	"errors"
	"fmt"
)

// validateIntrinsic runs every post-schema invariant on s and returns
// the joined set of violations, or nil if s is intrinsically valid.
// JSON Schema has already enforced field presence, enum values, id
// patterns, etc. — the checks here are the array-level / cross-field
// rules JSON Schema can't express.
func validateIntrinsic(s Sensor) error {
	var errs []error
	if err := checkUniqueStepIDs(s); err != nil {
		errs = append(errs, err)
	}
	if err := checkUniqueTopLevelUses(s); err != nil {
		errs = append(errs, err)
	}
	if err := checkUniqueStepUses(s); err != nil {
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func checkUniqueStepIDs(s Sensor) error {
	seen := make(map[string]bool, len(s.Steps))
	var dups []string
	for _, st := range s.Steps {
		if seen[st.ID] {
			dups = append(dups, st.ID)
			continue
		}
		seen[st.ID] = true
	}
	if len(dups) == 0 {
		return nil
	}
	return fmt.Errorf("duplicate step id(s): %v", dups)
}

func checkUniqueTopLevelUses(s Sensor) error {
	seen := make(map[string]bool, len(s.Uses))
	var dups []string
	for _, id := range s.Uses {
		if seen[id] {
			dups = append(dups, id)
			continue
		}
		seen[id] = true
	}
	if len(dups) == 0 {
		return nil
	}
	return fmt.Errorf("duplicate uses id(s): %v", dups)
}

func checkUniqueStepUses(s Sensor) error {
	var errs []error
	for _, st := range s.Steps {
		seen := make(map[string]bool, len(st.Uses))
		var dups []string
		for _, id := range st.Uses {
			if seen[id] {
				dups = append(dups, id)
				continue
			}
			seen[id] = true
		}
		if len(dups) > 0 {
			errs = append(errs, fmt.Errorf("step %q: duplicate uses id(s): %v", st.ID, dups))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
