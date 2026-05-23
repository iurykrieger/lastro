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

	return errors.Join(errs...)
}

func validateRollupArithmetic(r RollupCounts) error {
	sum := r.PassCount + r.WarnCount + r.FailCount + r.InconclusiveCount
	if sum != r.TotalSignals {
		return fmt.Errorf("rollup: counts sum to %d but total_signals is %d (pass=%d warn=%d fail=%d inconclusive=%d)",
			sum, r.TotalSignals, r.PassCount, r.WarnCount, r.FailCount, r.InconclusiveCount)
	}
	return nil
}
