package aggregate

import (
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

// sig builds a minimal signal with the given verdict and confidence 1.0.
// Warn + Fail also carry a heal_hint so the resulting signals are
// individually schema-valid (mirrors what a real sensor would emit).
func sig(verdict enums.Verdict) signalstub.Signal {
	s := signalstub.Signal{
		SchemaVersion: "1.0.0",
		SensorID:      "sensor-x",
		UseCaseID:     "uc-x",
		Angle:         enums.AngleUnitTest,
		EmittedAt:     time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC),
		Verdict:       verdict,
		Confidence:    1.0,
		Evidence:      map[string]any{"summary": "x"},
	}
	if verdict == enums.VerdictFail || verdict == enums.VerdictWarn {
		s.HealHint = &HealHint{Summary: "fix x", Rationale: "x is wrong"}
	}
	return s
}

func baseInput(signals []signalstub.Signal) RollupInput {
	return RollupInput{
		Signals:           signals,
		SensorID:          "sensor-x",
		UseCaseID:         "uc-x",
		Angle:             enums.AngleUnitTest,
		Kind:              enums.KindAssertion,
		OutputType:        enums.OutputStream,
		StartedAt:         time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC),
		EndedAt:           time.Date(2026, 5, 23, 14, 0, 42, 0, time.UTC),
		TerminationReason: enums.TerminationCompleted,
	}
}

func TestRollupCountsMatchVerdictDistribution(t *testing.T) {
	in := baseInput([]signalstub.Signal{
		sig(enums.VerdictPass), sig(enums.VerdictPass), sig(enums.VerdictPass),
		sig(enums.VerdictWarn),
		sig(enums.VerdictFail), sig(enums.VerdictFail),
		sig(enums.VerdictInconclusive),
	})
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Rollup.TotalSignals != 7 {
		t.Errorf("TotalSignals = %d, want 7", got.Rollup.TotalSignals)
	}
	if got.Rollup.PassCount != 3 || got.Rollup.WarnCount != 1 || got.Rollup.FailCount != 2 || got.Rollup.InconclusiveCount != 1 {
		t.Errorf("counts mismatch: %+v", got.Rollup)
	}
}
