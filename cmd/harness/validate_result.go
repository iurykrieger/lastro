package main

import (
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
)

// buildValidateResult shapes the spec §6.2 "result" object from raw
// per-use-case run results. The output is what lands inside RunResult.Result.
func buildValidateResult(results []UseCaseRunResult) any {
	type sensorJSON struct {
		SensorID    string                `json:"sensor_id"`
		Angle       enums.ValidationAngle `json:"angle"`
		Verdict     enums.Verdict         `json:"verdict"`
		Confidence  float64               `json:"confidence"`
		StartedAt   time.Time             `json:"started_at"`
		EndedAt     time.Time             `json:"ended_at"`
		Rollup      any                   `json:"rollup"`
		RuntimePath string                `json:"runtime_path,omitempty"`
		HealHint    any                   `json:"heal_hint,omitempty"`
	}
	type verdictJSON struct {
		UseCaseID           string                  `json:"use_case_id"`
		Archetype           enums.Archetype         `json:"archetype"`
		Verdict             enums.Verdict           `json:"verdict"`
		Confidence          float64                 `json:"confidence"`
		ObligatorySatisfied bool                    `json:"obligatory_satisfied"`
		EvaluatedAngles     []enums.ValidationAngle `json:"evaluated_angles"`
		FailingAngles       []enums.ValidationAngle `json:"failing_angles"`
		WarningAngles       []enums.ValidationAngle `json:"warning_angles"`
		HealHints           any                     `json:"heal_hints"`
		Sensors             []sensorJSON            `json:"sensors"`
	}
	type summary struct {
		TotalUseCases     int `json:"total_use_cases"`
		PassCount         int `json:"pass_count"`
		FailCount         int `json:"fail_count"`
		InconclusiveCount int `json:"inconclusive_count"`
	}

	verdicts := make([]verdictJSON, 0, len(results))
	s := summary{TotalUseCases: len(results)}
	for _, r := range results {
		v := r.Verdict
		sensors := make([]sensorJSON, 0, len(r.Sensors))
		for _, sig := range r.Sensors {
			sensors = append(sensors, sensorJSON{
				SensorID:   sig.SensorID,
				Angle:      sig.Angle,
				Verdict:    sig.Verdict,
				Confidence: sig.Confidence,
				StartedAt:  sig.StartedAt,
				EndedAt:    sig.EndedAt,
				Rollup:     sig.Rollup,
				HealHint:   sig.HealHint,
			})
		}
		verdicts = append(verdicts, verdictJSON{
			UseCaseID:           v.UseCaseID,
			Archetype:           v.Archetype,
			Verdict:             v.Verdict,
			Confidence:          v.Confidence,
			ObligatorySatisfied: v.ObligatorySatisfied,
			EvaluatedAngles:     v.EvaluatedAngles,
			FailingAngles:       v.FailingAngles,
			WarningAngles:       v.WarningAngles,
			HealHints:           v.HealHints,
			Sensors:             sensors,
		})
		switch v.Verdict {
		case enums.VerdictPass:
			s.PassCount++
		case enums.VerdictFail:
			s.FailCount++
		case enums.VerdictInconclusive:
			s.InconclusiveCount++
		}
	}
	return map[string]any{
		"verdicts": verdicts,
		"summary":  s,
	}
}
