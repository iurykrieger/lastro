package healloop

import (
	"context"
	"fmt"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/policy"
	usecase "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/internal/sensor"
)

// lifecycleRevalidator re-runs every assertion sensor in a use case and
// carries forward the original observational AggregateSignals into the
// re-aggregation. See spec §9.
type lifecycleRevalidator struct {
	runner          SensorRunner
	sensors         SensorLookup
	ucs             UseCaseLookup
	policy          *policy.EffectivePolicy
	originalSignals map[string]aggregate.AggregateSignal
	archetype       enums.Archetype
}

// DefaultRevalidator wires the lifecycleRevalidator with its dependencies.
// runner is satisfied by *lifecycle.Lifecycle. originalSignals MUST contain
// an entry for every observational sensor in the use case; missing entries
// cause the aggregator to flag the angle as missing.
func DefaultRevalidator(
	runner SensorRunner,
	sensors SensorLookup,
	ucs UseCaseLookup,
	pol *policy.EffectivePolicy,
	originalSignals map[string]aggregate.AggregateSignal,
	archetype enums.Archetype,
) Revalidator {
	return newLifecycleRevalidator(runner, sensors, ucs, pol, originalSignals, archetype)
}

func newLifecycleRevalidator(
	runner SensorRunner,
	sensors SensorLookup,
	ucs UseCaseLookup,
	pol *policy.EffectivePolicy,
	originalSignals map[string]aggregate.AggregateSignal,
	archetype enums.Archetype,
) *lifecycleRevalidator {
	return &lifecycleRevalidator{
		runner:          runner,
		sensors:         sensors,
		ucs:             ucs,
		policy:          pol,
		originalSignals: originalSignals,
		archetype:       archetype,
	}
}

// Revalidate re-runs assertion sensors and aggregates the result.
// Observational sensors are skipped; the carry-forward map supplies their
// AggregateSignals.
func (r *lifecycleRevalidator) Revalidate(ctx context.Context, useCaseID string) (usecase.UseCaseVerdict, error) {
	uc, ok := r.ucs.Lookup(useCaseID)
	if !ok {
		return usecase.UseCaseVerdict{}, ErrUseCaseNotFound
	}

	sensors := r.sensors.SensorsForUseCase(useCaseID)
	ordered, err := sensor.ResolveExecutionOrder(sensors)
	if err != nil {
		return usecase.UseCaseVerdict{}, fmt.Errorf("healloop: resolve order: %w", err)
	}

	aggs := make([]aggregate.AggregateSignal, 0, len(ordered))
	for _, s := range ordered {
		if s.Kind == enums.KindObservational {
			if orig, ok := r.originalSignals[s.ID]; ok {
				aggs = append(aggs, orig)
			}
			continue
		}
		agg, err := r.runner.RunSensor(ctx, s.ID, nil)
		if err != nil {
			return usecase.UseCaseVerdict{}, fmt.Errorf("healloop: run sensor %q: %w", s.ID, err)
		}
		aggs = append(aggs, agg)
	}

	return usecase.UseCase(uc, r.archetype, aggs, ordered, r.policy)
}
