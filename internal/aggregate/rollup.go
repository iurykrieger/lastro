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
	a.HealHint = synthesizeHealHint(in, a)

	if err := Validate(a); err != nil {
		return AggregateSignal{}, fmt.Errorf("rollup output failed validation: %w", err)
	}
	return a, nil
}

func computeVerdict(in RollupInput, a AggregateSignal) enums.Verdict {
	// Rule 1: observational + missing observations → fail (overrides all).
	if a.Completeness != nil && len(a.Completeness.MissingObservations) > 0 {
		return enums.VerdictFail
	}

	// Rules 2 + 3: timeout / error termination → fail wins, else inconclusive.
	if in.TerminationReason == enums.TerminationTimeout || in.TerminationReason == enums.TerminationError {
		for _, s := range in.Signals {
			if s.Verdict == enums.VerdictFail {
				return enums.VerdictFail
			}
		}
		return enums.VerdictInconclusive
	}

	// Rule 4: single-shot → mirror.
	if in.OutputType == enums.OutputSingleShot && len(in.Signals) == 1 {
		return in.Signals[0].Verdict
	}

	// Rule 5: severity ordering (completed / stopped).
	return severityVerdict(in.Signals)
}

func severityVerdict(signals []signalstub.Signal) enums.Verdict {
	var hasWarn, hasInconclusive bool
	for _, s := range signals {
		switch s.Verdict {
		case enums.VerdictFail:
			return enums.VerdictFail
		case enums.VerdictWarn:
			hasWarn = true
		case enums.VerdictInconclusive:
			hasInconclusive = true
		}
	}
	switch {
	case hasWarn:
		return enums.VerdictWarn
	case hasInconclusive:
		return enums.VerdictInconclusive
	default:
		return enums.VerdictPass
	}
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
