package aggregate

import (
	"errors"
	"fmt"
)

// Validate runs the hand-written rules that the embedded JSON Schema
// cannot express. ParseAggregate runs the schema first, then this; Rollup
// runs only this on its own output. Errors are collected via errors.Join
// so callers see every violation in one pass.
func Validate(a AggregateSignal) error {
	var errs []error

	if err := validateRollupArithmetic(a.Rollup); err != nil {
		errs = append(errs, err)
	}
	if err := validateHealHintAbsence(a); err != nil {
		errs = append(errs, err)
	}
	if err := validateTimeOrder(a); err != nil {
		errs = append(errs, err)
	}
	if err := validateCompleteness(a.Completeness); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func validateTimeOrder(a AggregateSignal) error {
	if a.EndedAt.Before(a.StartedAt) {
		return fmt.Errorf("ended_at (%s) is before started_at (%s)",
			a.EndedAt.Format("2006-01-02T15:04:05Z07:00"),
			a.StartedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	return nil
}

func validateCompleteness(c *Completeness) error {
	if c == nil {
		return nil
	}
	expected := make(map[string]bool, len(c.ExpectedObservations))
	for _, k := range c.ExpectedObservations {
		expected[k] = true
	}
	for _, k := range c.MissingObservations {
		if !expected[k] {
			return fmt.Errorf("completeness: missing_observations contains %q which is not in expected_observations", k)
		}
	}
	return nil
}

func validateHealHintAbsence(a AggregateSignal) error {
	// Schema enforces "required when verdict in {warn, fail}". This adds
	// the other direction: forbidden when verdict is pass or inconclusive.
	if a.HealHint == nil {
		return nil
	}
	switch a.Verdict {
	case "pass", "inconclusive":
		return fmt.Errorf("heal_hint must be absent when verdict is %q (only warn and fail carry heal hints)", a.Verdict)
	}
	return nil
}

func validateRollupArithmetic(r RollupCounts) error {
	sum := r.PassCount + r.WarnCount + r.FailCount + r.InconclusiveCount
	if sum != r.TotalSignals {
		return fmt.Errorf("rollup: counts sum to %d but total_signals is %d (pass=%d warn=%d fail=%d inconclusive=%d)",
			sum, r.TotalSignals, r.PassCount, r.WarnCount, r.FailCount, r.InconclusiveCount)
	}
	return nil
}
