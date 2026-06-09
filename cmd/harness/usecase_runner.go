package main

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/policy"
	aggregator "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// SensorRunner is the minimal seam the use-case runner needs from
// lifecycle. Tests inject a fake; production wires *lifecycle.Lifecycle.
type SensorRunner interface {
	RunSensor(ctx context.Context, sensorID string, expectedObs []string) (aggregate.AggregateSignal, error)
}

// classifyServices partitions gathered sensors into shared services (core +
// observational sensors that other sensors attach to) and regular sensors.
// Services are managed by servicemgr — started on first attach, reaped on
// last detach — and are NOT scheduled as run-to-completion wavefront nodes.
func classifyServices(sensors []sensor.Sensor) (services, regular []sensor.Sensor) {
	for _, s := range sensors {
		if s.Scope == enums.ScopeCore && s.Kind == enums.KindObservational {
			services = append(services, s)
			continue
		}
		regular = append(regular, s)
	}
	return services, regular
}

// wavefronts groups sensors into layers of independent work. Layer 0
// contains every sensor with no DependsOn; layer N contains sensors
// whose DependsOn ids all resolved to layers 0..N-1.
//
// Pre-condition: sensors is already topologically sorted by
// sensor.ResolveExecutionOrder (this preserves determinism and lets
// the wavefront pass walk in a single sweep).
func wavefronts(sensors []sensor.Sensor) [][]sensor.Sensor {
	if len(sensors) == 0 {
		return nil
	}
	layerOf := make(map[string]int, len(sensors))
	var layers [][]sensor.Sensor
	for _, s := range sensors {
		layer := 0
		for _, dep := range s.DependsOn {
			if l, ok := layerOf[dep]; ok && l+1 > layer {
				layer = l + 1
			}
		}
		layerOf[s.ID] = layer
		for len(layers) <= layer {
			layers = append(layers, nil)
		}
		layers[layer] = append(layers[layer], s)
	}
	// Sort each layer by sensor ID for deterministic launch order
	// inside the layer (matches the resolver's tiebreak).
	for i := range layers {
		sort.Slice(layers[i], func(a, b int) bool {
			return layers[i][a].ID < layers[i][b].ID
		})
	}
	return layers
}

// runUseCaseSensors runs every sensor for a use case, layer by layer.
// Within a layer, sensors run in parallel; layers advance only after
// the current layer finishes (matches spec §5.1 step 3 + B2's
// "parallel where DAG allows, serial within a chain").
//
// observationKeys maps sensorID -> expected_observations slice for
// observational sensors; assertion sensors receive nil. The caller
// (usually derived from sensor metadata) supplies it.
func runUseCaseSensors(
	ctx context.Context,
	runner SensorRunner,
	sensors []sensor.Sensor,
	observationKeys map[string][]string,
) ([]aggregate.AggregateSignal, error) {
	sorted, err := sensor.ResolveExecutionOrder(sensors)
	if err != nil {
		return nil, fmt.Errorf("topo sort: %w", err)
	}
	layers := wavefronts(sorted)

	results := make([]aggregate.AggregateSignal, 0, len(sorted))
	resultsMu := sync.Mutex{}

	for _, layer := range layers {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		var wg sync.WaitGroup
		errCh := make(chan error, len(layer))

		for _, s := range layer {
			wg.Add(1)
			go func(s sensor.Sensor) {
				defer wg.Done()
				agg, err := runner.RunSensor(ctx, s.ID, observationKeys[s.ID])
				if err != nil {
					// Convert sensor execution errors into an
					// inconclusive AggregateSignal so the use case
					// aggregator can still produce a verdict.
					agg = inconclusiveFromError(s, err)
				}
				resultsMu.Lock()
				results = append(results, agg)
				resultsMu.Unlock()
			}(s)
		}
		wg.Wait()
		close(errCh)
	}

	// Re-sort results by sensor id so downstream consumers see a
	// deterministic order (parallel goroutines append in arrival order).
	sort.Slice(results, func(i, j int) bool {
		return results[i].SensorID < results[j].SensorID
	})

	return results, nil
}

// inconclusiveFromError synthesizes an AggregateSignal carrying an
// inconclusive verdict when lifecycle.RunSensor returns an error.
// Matches the spec §5.1 error semantics: "Sensor execution errors
// (lifecycle returns non-nil error) bubble up as inconclusive."
func inconclusiveFromError(s sensor.Sensor, err error) aggregate.AggregateSignal {
	return aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              aggregate.TypeAggregate,
		SensorID:          s.ID,
		UseCaseID:         s.UseCaseID,
		Angle:             s.Angle,
		Verdict:           enums.VerdictInconclusive,
		Confidence:        0,
		TerminationReason: enums.TerminationError,
		Rollup: aggregate.RollupCounts{
			InconclusiveCount: 1,
		},
		HealHint: &aggregate.HealHint{
			Summary:   "sensor execution failed: " + err.Error(),
			Rationale: "lifecycle.RunSensor returned a non-nil error; verdict demoted to inconclusive",
		},
	}
}

// UseCaseRunResult bundles the per-sensor AggregateSignals with the
// rolled-up UseCaseVerdict. Renderers consume both — sensors for the
// per-angle breakdown, verdict for the aggregate state.
type UseCaseRunResult struct {
	Verdict aggregator.UseCaseVerdict
	Sensors []aggregate.AggregateSignal
}

// RunUseCase orchestrates the full per-use-case validation pipeline:
//   1. Filter the sensor store to sensors owned by this use case.
//   2. Run them via wavefront layers (assertion sensors only in v1;
//      observational support tracked under the same handler set).
//   3. Aggregate via internal/runtime/aggregator/usecase.UseCase.
//
// Returns the use-case verdict plus the per-sensor AggregateSignal
// slice so the renderer can show both.
func RunUseCase(
	ctx context.Context,
	runner SensorRunner,
	arts *HarnessArtifacts,
	useCaseID string,
) (UseCaseRunResult, error) {
	uc, ok := arts.UseCases[useCaseID]
	if !ok {
		return UseCaseRunResult{}, fmt.Errorf("use case %q not found", useCaseID)
	}
	// Gather use-case sensors plus the transitive depends_on closure into
	// scope:core sensors (e.g. run-dev, database-query). Core sensors have
	// no use_case_id and are excluded by a plain ForUseCase filter.
	owned := arts.Sensors.GatherForUseCase(useCaseID)
	if len(owned) == 0 {
		return UseCaseRunResult{}, fmt.Errorf("no sensors found for use case %q", useCaseID)
	}

	// observationKeys: future feature; empty in v1.
	signals, err := runUseCaseSensors(ctx, runner, owned, nil)
	if err != nil {
		return UseCaseRunResult{}, err
	}

	// Pick the archetype the use case carries (first scope entry — the
	// resolver assumes one archetype per use case in v1; multi-archetype
	// scopes pick the first applicable).
	if len(uc.ArchetypeScope) == 0 {
		return UseCaseRunResult{}, fmt.Errorf("use case %q has empty archetype_scope", useCaseID)
	}
	archetype := uc.ArchetypeScope[0]

	verdict, err := aggregateUseCase(uc, archetype, signals, owned, arts.Policy)
	if err != nil {
		return UseCaseRunResult{}, fmt.Errorf("aggregate: %w", err)
	}

	return UseCaseRunResult{
		Verdict: verdict,
		Sensors: signals,
	}, nil
}

// aggregateUseCase is a thin wrapper over aggregator.UseCase that
// exists only to keep imports localized to this file.
func aggregateUseCase(
	uc *usecase.UseCase,
	archetype enums.Archetype,
	signals []aggregate.AggregateSignal,
	sensors []sensor.Sensor,
	pol *policy.EffectivePolicy,
) (aggregator.UseCaseVerdict, error) {
	return aggregator.UseCase(uc, archetype, signals, sensors, pol)
}
