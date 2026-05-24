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
	if !archetypeInScope(uc, archetype) {
		return UseCaseVerdict{}, fmt.Errorf("aggregator: archetype-not-in-scope: %q is not in use case %q archetype_scope", archetype, uc.ID)
	}
	for _, s := range signals {
		if s.UseCaseID != uc.ID {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: signal-foreign-use-case: signal use_case_id=%q does not match %q", s.UseCaseID, uc.ID)
		}
	}
	seen := make(map[enums.ValidationAngle]struct{}, len(signals))
	signalByAngle := make(map[enums.ValidationAngle]aggregate.AggregateSignal, len(signals))
	for _, s := range signals {
		if _, dup := seen[s.Angle]; dup {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: duplicate-angle-signal: angle %q has more than one AggregateSignal", s.Angle)
		}
		seen[s.Angle] = struct{}{}
		signalByAngle[s.Angle] = s
	}

	statuses := pol.PerArchetype[archetype]
	for angle, status := range statuses {
		if status != policy.StatusObligatory {
			continue
		}
		if _, ok := signalByAngle[angle]; !ok {
			return UseCaseVerdict{}, fmt.Errorf("aggregator: missing-obligatory-signal: angle %q has no AggregateSignal", angle)
		}
	}

	// Verdict + confidence computation arrives in Task 12.
	return UseCaseVerdict{
		UseCaseID:       uc.ID,
		Archetype:       archetype,
		Verdict:         enums.VerdictInconclusive,
		EvaluatedAngles: []enums.ValidationAngle{},
		FailingAngles:   []enums.ValidationAngle{},
		WarningAngles:   []enums.ValidationAngle{},
		HealHints:       []AngleHint{},
	}, nil
}

func archetypeInScope(uc *usecase.UseCase, arch enums.Archetype) bool {
	for _, a := range uc.ArchetypeScope {
		if a == arch {
			return true
		}
	}
	return false
}
