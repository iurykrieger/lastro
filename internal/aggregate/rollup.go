package aggregate

import (
	"fmt"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

// Rollup is the deterministic per-sensor aggregator. Given a slice of
// emitted Signals plus execution metadata, it returns the terminal
// AggregateSignal that ends the sensor's JSON Lines stream.
//
// Rollup is pure: same RollupInput → byte-identical AggregateSignal.
func Rollup(in RollupInput) (AggregateSignal, error) {
	a := AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              TypeAggregate,
		SensorID:          in.SensorID,
		UseCaseID:         in.UseCaseID,
		Angle:             in.Angle,
		StartedAt:         in.StartedAt,
		EndedAt:           in.EndedAt,
		TerminationReason: in.TerminationReason,
		Rollup:            computeCounts(in.Signals),
	}

	if err := Validate(a); err != nil {
		return AggregateSignal{}, fmt.Errorf("rollup output failed validation: %w", err)
	}
	return a, nil
}

func computeCounts(signals []signalstub.Signal) RollupCounts {
	c := RollupCounts{TotalSignals: len(signals)}
	for _, s := range signals {
		switch s.Verdict {
		case enums.VerdictPass:
			c.PassCount++
		case enums.VerdictWarn:
			c.WarnCount++
		case enums.VerdictFail:
			c.FailCount++
		case enums.VerdictInconclusive:
			c.InconclusiveCount++
		}
	}
	return c
}
