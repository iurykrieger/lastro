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
	"github.com/iurykrieger/lastro/internal/runtime/servicemgr"
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

// ServiceManager is the slice of *servicemgr.Manager the runner needs.
type ServiceManager interface {
	Acquire(ctx context.Context, serviceID string, expectedObs []string) (servicemgr.Attachment, error)
	Release(ctx context.Context, serviceID string) error
}

// serviceDepsOf maps each regular sensor id to the service ids it attaches to
// (via a uses-step or a depends_on edge that names a known service).
func serviceDepsOf(sensors []sensor.Sensor, isService map[string]bool) map[string][]string {
	deps := map[string][]string{}
	for _, s := range sensors {
		seen := map[string]bool{}
		add := func(id string) {
			if isService[id] && !seen[id] {
				seen[id] = true
				deps[s.ID] = append(deps[s.ID], id)
			}
		}
		for _, d := range s.DependsOn {
			add(d)
		}
		for _, st := range s.Steps {
			if st.Uses != "" {
				add(st.Uses)
			}
		}
	}
	return deps
}

// stripServiceEdges returns copies of sensors with every depends_on edge that
// points at a shared service removed. Service ordering is handled by the
// ref-counted Acquire/Release around each consumer's run, so services are not
// scheduled as wavefront nodes; leaving the edge in would make
// ResolveExecutionOrder reject the consumer as having a dangling dependency.
// The set of service ids is the union of serviceDeps' values.
func stripServiceEdges(sensors []sensor.Sensor, serviceDeps map[string][]string) []sensor.Sensor {
	isService := map[string]bool{}
	for _, svcs := range serviceDeps {
		for _, id := range svcs {
			isService[id] = true
		}
	}
	out := make([]sensor.Sensor, len(sensors))
	for i, s := range sensors {
		out[i] = s
		if len(s.DependsOn) == 0 {
			continue
		}
		filtered := make([]string, 0, len(s.DependsOn))
		for _, dep := range s.DependsOn {
			if !isService[dep] {
				filtered = append(filtered, dep)
			}
		}
		out[i].DependsOn = filtered
	}
	return out
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
//
// mgr is the ServiceManager that owns the lifecycle of shared services.
// serviceDeps maps each sensor id to the service ids it must Acquire before
// running and Release on exit.
func runUseCaseSensors(
	ctx context.Context,
	runner SensorRunner,
	sensors []sensor.Sensor,
	observationKeys map[string][]string,
	mgr ServiceManager,
	serviceDeps map[string][]string,
) ([]aggregate.AggregateSignal, error) {
	// Shared services are excluded from the scheduled set; their lifetime is
	// managed by Acquire/Release, not the wavefront DAG. Strip service edges
	// from depends_on so the resolver does not treat the (absent) service as a
	// dangling dependency. A consumer attaches via serviceDeps regardless.
	scheduled := stripServiceEdges(sensors, serviceDeps)

	sorted, err := sensor.ResolveExecutionOrder(scheduled)
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
		for _, s := range layer {
			wg.Add(1)
			go func(s sensor.Sensor) {
				defer wg.Done()
				// Acquire the services this sensor attaches to; release on exit.
				acquired := make([]string, 0, len(serviceDeps[s.ID]))
				defer func() {
					for _, id := range acquired {
						_ = mgr.Release(ctx, id)
					}
				}()
				for _, svc := range serviceDeps[s.ID] {
					if _, err := mgr.Acquire(ctx, svc, nil); err != nil {
						resultsMu.Lock()
						results = append(results, inconclusiveFromError(s, err))
						resultsMu.Unlock()
						return
					}
					acquired = append(acquired, svc)
				}
				agg, err := runner.RunSensor(ctx, s.ID, observationKeys[s.ID])
				if err != nil {
					agg = inconclusiveFromError(s, err)
				}
				resultsMu.Lock()
				results = append(results, agg)
				resultsMu.Unlock()
			}(s)
		}
		wg.Wait()
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
//   2. Classify sensors into shared services (core+observational) and regular.
//   3. Run regular sensors via wavefront layers, acquiring/releasing services.
//   4. Aggregate via internal/runtime/aggregator/usecase.UseCase.
//
// Returns the use-case verdict plus the per-sensor AggregateSignal
// slice so the renderer can show both.
func RunUseCase(
	ctx context.Context,
	runner SensorRunner,
	mgr ServiceManager,
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

	services, regular := classifyServices(owned)
	isService := make(map[string]bool, len(services))
	for _, s := range services {
		isService[s.ID] = true
	}
	deps := serviceDepsOf(regular, isService)

	signals, err := runUseCaseSensors(ctx, runner, regular, nil, mgr, deps)
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

	verdict, err := aggregateUseCase(uc, archetype, signals, regular, arts.Policy)
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
