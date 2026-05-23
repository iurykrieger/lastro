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
		Completeness:      computeCompleteness(in),
	}
	a.Verdict = computeVerdict(in, a)
	a.Confidence = computeConfidence(in.Signals, a.Verdict)

	if err := Validate(a); err != nil {
		return AggregateSignal{}, fmt.Errorf("rollup output failed validation: %w", err)
	}
	return a, nil
}

// computeVerdict is a placeholder until Task 17/18/19. It picks pass for
// now so we can land tests incrementally.
func computeVerdict(in RollupInput, a AggregateSignal) enums.Verdict {
	return enums.VerdictPass
}

func computeConfidence(signals []signalstub.Signal, v enums.Verdict) float64 {
	if len(signals) == 0 {
		switch v {
		case enums.VerdictInconclusive:
			return 0.0
		default:
			return 1.0
		}
	}
	var sum float64
	for _, s := range signals {
		sum += s.Confidence
	}
	return sum / float64(len(signals))
}

func computeCompleteness(in RollupInput) *Completeness {
	if in.Kind != enums.KindObservational {
		return nil
	}
	expected := append([]string(nil), in.ExpectedObservations...)
	observed := make(map[string]bool, len(in.ObservedKeys))
	for _, k := range in.ObservedKeys {
		observed[k] = true
	}
	missing := make([]string, 0)
	for _, k := range expected {
		if !observed[k] {
			missing = append(missing, k)
		}
	}
	return &Completeness{
		ExpectedObservations: expected,
		MissingObservations:  missing,
	}
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
