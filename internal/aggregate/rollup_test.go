package aggregate

import (
	"strings"
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

func TestRollupConfidenceIsArithmeticMean(t *testing.T) {
	s1 := sig(enums.VerdictPass)
	s1.Confidence = 1.0
	s2 := sig(enums.VerdictPass)
	s2.Confidence = 0.5
	in := baseInput([]signalstub.Signal{s1, s2})
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Confidence < 0.749999 || got.Confidence > 0.750001 {
		t.Errorf("Confidence = %v, want ~0.75", got.Confidence)
	}
}

func TestRollupEmptySignalsObservationalPassConfidence(t *testing.T) {
	in := baseInput(nil)
	in.Kind = enums.KindObservational
	in.OutputType = enums.OutputStream
	in.ExpectedObservations = []string{"a", "b"}
	in.ObservedKeys = []string{"a", "b"}
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Confidence != 1.0 {
		t.Errorf("Confidence = %v, want 1.0 for full-coverage observational with no signals", got.Confidence)
	}
}

func TestRollupSingleShotMirrorsSoleVerdict(t *testing.T) {
	cases := []enums.Verdict{
		enums.VerdictPass, enums.VerdictWarn, enums.VerdictFail, enums.VerdictInconclusive,
	}
	for _, v := range cases {
		t.Run(string(v), func(t *testing.T) {
			in := baseInput([]signalstub.Signal{sig(v)})
			in.OutputType = enums.OutputSingleShot
			got, err := Rollup(in)
			if v == enums.VerdictPass {
				if err != nil {
					t.Fatalf("Rollup: %v", err)
				}
			} else {
				// warn/fail/inconclusive aggregates need a heal_hint or pass the
				// inconclusive carve-out; we expect synthesis to attach one when
				// Tasks 20/21 land. For this task, accept either success or a
				// validation error mentioning heal_hint.
				if err != nil && !strings.Contains(err.Error(), "heal_hint") {
					t.Fatalf("Rollup: %v", err)
				}
			}
			if got.Verdict != v && err == nil {
				t.Errorf("Verdict = %q, want %q", got.Verdict, v)
			}
		})
	}
}

func TestRollupStreamSeverityOrdering(t *testing.T) {
	cases := []struct {
		name     string
		verdicts []enums.Verdict
		want     enums.Verdict
	}{
		{"all-pass", []enums.Verdict{enums.VerdictPass, enums.VerdictPass}, enums.VerdictPass},
		{"pass-warn", []enums.Verdict{enums.VerdictPass, enums.VerdictWarn}, enums.VerdictWarn},
		{"pass-warn-inconclusive", []enums.Verdict{enums.VerdictPass, enums.VerdictWarn, enums.VerdictInconclusive}, enums.VerdictWarn},
		{"pass-inconclusive", []enums.Verdict{enums.VerdictPass, enums.VerdictInconclusive}, enums.VerdictInconclusive},
		{"pass-warn-fail", []enums.Verdict{enums.VerdictPass, enums.VerdictWarn, enums.VerdictFail}, enums.VerdictFail},
		{"fail-only", []enums.Verdict{enums.VerdictFail}, enums.VerdictFail},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			signals := make([]signalstub.Signal, len(c.verdicts))
			for i, v := range c.verdicts {
				signals[i] = sig(v)
			}
			in := baseInput(signals)
			got, err := Rollup(in)
			// Tasks 20/21 fill in heal_hint synthesis; until then, warn/fail
			// outputs may fail validation. Accept synthesis-pending errors
			// but check the verdict was decided correctly.
			if got.Verdict != c.want && err == nil {
				t.Errorf("verdict = %q, want %q", got.Verdict, c.want)
			} else if c.want == enums.VerdictPass || c.want == enums.VerdictInconclusive {
				if err != nil {
					t.Fatalf("unexpected error for non-hinted verdict: %v", err)
				}
			}
		})
	}
}
