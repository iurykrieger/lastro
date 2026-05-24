package aggregator

import (
	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/policy"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// UseCase computes the verdict for one use case under one archetype context.
// Real implementation comes in subsequent tasks.
func UseCase(
	uc *usecase.UseCase,
	archetype enums.Archetype,
	signals []aggregate.AggregateSignal,
	sensors []sensor.Sensor,
	pol *policy.EffectivePolicy,
) (UseCaseVerdict, error) {
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
