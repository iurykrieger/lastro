package aggregator

import (
	"fmt"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/policy"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// UseCase computes the verdict for one use case under one archetype context.
// See spec §6 for the algorithm.
func UseCase(
	uc *usecase.UseCase,
	archetype enums.Archetype,
	signals []aggregate.AggregateSignal,
	sensors []sensor.Sensor,
	pol *policy.EffectivePolicy,
) (UseCaseVerdict, error) {
	if uc == nil {
		return UseCaseVerdict{}, fmt.Errorf("aggregator: nil UseCase")
	}
	if pol == nil {
		return UseCaseVerdict{}, fmt.Errorf("aggregator: nil EffectivePolicy")
	}

	// Step 1 — input validation: archetype scope, signal ownership, angle uniqueness.

	if !archetypeInScope(uc, archetype) {
		return UseCaseVerdict{}, fmt.Errorf("aggregator: archetype-not-in-scope: %q is not in use case %q archetype_scope", archetype, uc.ID)
	}
	for _, s := range signals {
		// Core sensors have use_case_id="" (scope: core). They are DAG
		// preconditions, not policy-graded facets. Allow them through —
		// they are excluded from signalByAngle below so they don't affect
		// policy evaluation. Their verdicts reach worstExitCode via the
		// caller's full aggs slice.
		if s.UseCaseID != uc.ID && s.UseCaseID != "" {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: signal-foreign-use-case: signal use_case_id=%q does not match %q", s.UseCaseID, uc.ID)
		}
	}
	seen := make(map[enums.ValidationAngle]struct{}, len(signals))
	signalByAngle := make(map[enums.ValidationAngle]aggregate.AggregateSignal, len(signals))
	for _, s := range signals {
		if s.UseCaseID == "" {
			// Core sensor signal — not policy-graded; skip.
			continue
		}
		if _, dup := seen[s.Angle]; dup {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: duplicate-angle-signal: angle %q has more than one AggregateSignal", s.Angle)
		}
		seen[s.Angle] = struct{}{}
		signalByAngle[s.Angle] = s
	}

	// Step 2 — resolve angle statuses; verify obligatory coverage.

	statuses := pol.PerArchetype[archetype]
	for _, angle := range enums.AllAngles() {
		if statuses[angle] != policy.StatusObligatory {
			continue
		}
		if _, ok := signalByAngle[angle]; !ok {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: missing-obligatory-signal: angle %q has no AggregateSignal", angle)
		}
	}

	// Step 3 — walk signals in canonical order, applying floor demotion and recording outcomes.

	natureBySensorID := make(map[string]enums.SensorNature, len(sensors))
	for _, s := range sensors {
		natureBySensorID[s.ID] = s.Nature
	}

	v := UseCaseVerdict{
		UseCaseID:       uc.ID,
		Archetype:       archetype,
		EvaluatedAngles: []enums.ValidationAngle{},
		FailingAngles:   []enums.ValidationAngle{},
		WarningAngles:   []enums.ValidationAngle{},
		HealHints:       []AngleHint{},
	}

	anyObligatoryFail := false
	allObligatoryPassGrade := true
	var weightSum, weightedSum float64

	for _, angle := range enums.AllAngles() {
		status, hasStatus := statuses[angle]
		if !hasStatus || status == policy.StatusDisabled {
			continue
		}
		sig, hasSig := signalByAngle[angle]
		if !hasSig {
			// status == optional with no signal: skip.
			continue
		}

		nature := natureBySensorID[sig.SensorID]
		effective := effectiveVerdict(sig, nature, pol.InferentialFloor)

		// Accumulate weighted confidence using RAW sig.Confidence (spec §6.2 step 6).
		// Floor demotion affects verdict only, not the confidence contribution.
		var weight float64
		if nature == enums.NatureComputational {
			weight = 1.0
		} else {
			weight = sig.Confidence
		}
		weightSum += weight
		weightedSum += weight * sig.Confidence

		v.EvaluatedAngles = append(v.EvaluatedAngles, angle)

		switch effective {
		case enums.VerdictFail:
			if sig.HealHint == nil {
				return UseCaseVerdict{}, fmt.Errorf("aggregator: signal angle=%q verdict=fail but heal_hint is nil (E8 invariant violated)", angle)
			}
			v.FailingAngles = append(v.FailingAngles, angle)
			v.HealHints = append(v.HealHints, AngleHint{Angle: angle, Verdict: enums.VerdictFail, Hint: *sig.HealHint})
			if status == policy.StatusObligatory {
				anyObligatoryFail = true
			}
		case enums.VerdictWarn:
			if sig.HealHint == nil {
				return UseCaseVerdict{}, fmt.Errorf("aggregator: signal angle=%q verdict=warn but heal_hint is nil (E8 invariant violated)", angle)
			}
			v.WarningAngles = append(v.WarningAngles, angle)
			v.HealHints = append(v.HealHints, AngleHint{Angle: angle, Verdict: enums.VerdictWarn, Hint: *sig.HealHint})
		case enums.VerdictInconclusive:
			if status == policy.StatusObligatory {
				allObligatoryPassGrade = false
			}
		case enums.VerdictPass:
			// pass-grade; nothing to surface.
		}
	}

	// Step 4 — verdict per plan §6.3.

	switch {
	case anyObligatoryFail:
		v.Verdict = enums.VerdictFail
	case allObligatoryPassGrade:
		v.Verdict = enums.VerdictPass
	default:
		v.Verdict = enums.VerdictInconclusive
	}
	v.ObligatorySatisfied = v.Verdict == enums.VerdictPass

	// Confidence: weighted average using RAW signal.Confidence (spec §6.2 step 6).
	// Floor demotion affects verdict only, not the confidence contribution.
	if weightSum > 0 {
		v.Confidence = weightedSum / weightSum
	}

	return v, nil
}

// effectiveVerdict returns the post-floor-demotion verdict for a signal.
// Inferential signals with confidence below the floor are demoted to inconclusive,
// regardless of whether the raw verdict is fail or warn.
func effectiveVerdict(sig aggregate.AggregateSignal, nature enums.SensorNature, floor float64) enums.Verdict {
	if nature == enums.NatureInferential && sig.Confidence < floor {
		return enums.VerdictInconclusive
	}
	return sig.Verdict
}

func archetypeInScope(uc *usecase.UseCase, arch enums.Archetype) bool {
	for _, a := range uc.ArchetypeScope {
		if a == arch {
			return true
		}
	}
	return false
}
