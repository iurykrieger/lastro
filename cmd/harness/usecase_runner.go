package main

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
)

// SensorRunner is the minimal seam the use-case runner needs from
// lifecycle. Tests inject a fake; production wires *lifecycle.Lifecycle.
type SensorRunner interface {
	RunSensor(ctx context.Context, sensorID string, expectedObs []string) (aggregate.AggregateSignal, error)
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
