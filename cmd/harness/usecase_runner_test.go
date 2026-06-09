package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
)

// fakeRunner records every RunSensor invocation order + duration to
// let tests assert wavefront semantics.
type fakeRunner struct {
	mu        sync.Mutex
	starts    []string                   // sensor IDs in order RunSensor was entered
	completes []string                   // sensor IDs in order RunSensor returned
	delay     map[string]time.Duration   // per-sensor sleep before returning
	verdicts  map[string]enums.Verdict   // override verdict per sensor
	calls     atomic.Int64               // total RunSensor calls
}

func (f *fakeRunner) RunSensor(ctx context.Context, sensorID string, expectedObs []string) (aggregate.AggregateSignal, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.starts = append(f.starts, sensorID)
	f.mu.Unlock()

	if d, ok := f.delay[sensorID]; ok {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return aggregate.AggregateSignal{}, ctx.Err()
		}
	}

	verdict := enums.VerdictPass
	if v, ok := f.verdicts[sensorID]; ok {
		verdict = v
	}

	f.mu.Lock()
	f.completes = append(f.completes, sensorID)
	f.mu.Unlock()

	return aggregate.AggregateSignal{
		SchemaVersion: "1.0.0",
		Type:          aggregate.TypeAggregate,
		SensorID:      sensorID,
		Verdict:       verdict,
		Confidence:    1.0,
		Rollup:        aggregate.RollupCounts{TotalSignals: 1, PassCount: 1},
	}, nil
}

func sensorWith(id, dep string) sensor.Sensor {
	s := sensor.Sensor{ID: id, Kind: enums.KindAssertion}
	if dep != "" {
		s.DependsOn = []string{dep}
	}
	return s
}

func TestWavefronts_LinearChain(t *testing.T) {
	sensors := []sensor.Sensor{
		sensorWith("a", ""),
		sensorWith("b", "a"),
		sensorWith("c", "b"),
	}
	layers := wavefronts(sensors)
	if len(layers) != 3 {
		t.Fatalf("layers = %d, want 3", len(layers))
	}
	for i, want := range []string{"a", "b", "c"} {
		if len(layers[i]) != 1 || layers[i][0].ID != want {
			t.Errorf("layer %d = %v, want [%s]", i, layerIDs(layers[i]), want)
		}
	}
}

func TestWavefronts_Diamond(t *testing.T) {
	sensors := []sensor.Sensor{
		sensorWith("a", ""),
		sensorWith("b", "a"),
		sensorWith("c", "a"),
		sensor.Sensor{ID: "d", DependsOn: []string{"b", "c"}, Kind: enums.KindAssertion},
	}
	layers := wavefronts(sensors)
	if len(layers) != 3 {
		t.Fatalf("layers = %d, want 3", len(layers))
	}
	if layerIDs(layers[1]) != "b,c" {
		t.Errorf("layer 1 = %s, want b,c", layerIDs(layers[1]))
	}
	if layerIDs(layers[2]) != "d" {
		t.Errorf("layer 2 = %s, want d", layerIDs(layers[2]))
	}
}

func TestRunUseCaseSensors_LayerSerialization(t *testing.T) {
	sensors := []sensor.Sensor{
		sensorWith("a", ""),
		sensorWith("b", "a"),
	}
	runner := &fakeRunner{
		delay: map[string]time.Duration{"a": 50 * time.Millisecond},
	}

	results, err := runUseCaseSensors(context.Background(), runner, sensors, nil)
	if err != nil {
		t.Fatalf("runUseCaseSensors: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	// `b` must not have started until `a` completed.
	if runner.completes[0] != "a" || runner.starts[len(runner.starts)-1] != "b" {
		t.Errorf("layer ordering wrong: starts=%v completes=%v", runner.starts, runner.completes)
	}
}

func TestRunUseCaseSensors_ParallelWithinLayer(t *testing.T) {
	sensors := []sensor.Sensor{
		sensorWith("a", ""),
		sensorWith("b", ""),
		sensorWith("c", ""),
	}
	runner := &fakeRunner{
		delay: map[string]time.Duration{
			"a": 50 * time.Millisecond,
			"b": 50 * time.Millisecond,
			"c": 50 * time.Millisecond,
		},
	}

	start := time.Now()
	results, err := runUseCaseSensors(context.Background(), runner, sensors, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("runUseCaseSensors: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	// With three 50ms sensors at layer 0, parallel execution must finish
	// well under the serial 150ms total.
	if elapsed > 120*time.Millisecond {
		t.Errorf("elapsed = %v, expected parallel finish (<120ms)", elapsed)
	}
}

func TestInconclusiveFromError_Shape(t *testing.T) {
	s := sensor.Sensor{ID: "x", UseCaseID: "uc", Angle: enums.AngleBuild}
	agg := inconclusiveFromError(s, errwrap("boom: %w", context.Canceled))

	if agg.Verdict != enums.VerdictInconclusive {
		t.Errorf("Verdict = %s, want inconclusive", agg.Verdict)
	}
	if agg.HealHint == nil || agg.HealHint.Summary == "" {
		t.Errorf("HealHint missing")
	}
	if agg.TerminationReason != enums.TerminationError {
		t.Errorf("TerminationReason = %s, want error", agg.TerminationReason)
	}
}

// layerIDs returns a comma-joined string of sensor IDs in a layer, in
// the order they appear (the scheduler sorts within layer by ID).
func layerIDs(layer []sensor.Sensor) string {
	out := ""
	for i, s := range layer {
		if i > 0 {
			out += ","
		}
		out += s.ID
	}
	return out
}

func TestClassifyServices_SplitsCoreObservationalTargets(t *testing.T) {
	sensors := []sensor.Sensor{
		{ID: "run-dev", Scope: enums.ScopeCore, Kind: enums.KindObservational},
		{ID: "logs", UseCaseID: "uc", Kind: enums.KindObservational, Steps: []sensor.Step{{ID: "watch", Uses: "run-dev"}}},
		{ID: "unit", UseCaseID: "uc", Kind: enums.KindAssertion, Steps: []sensor.Step{{ID: "t", Run: "go test ./..."}}},
	}
	services, regular := classifyServices(sensors)
	if len(services) != 1 || services[0].ID != "run-dev" {
		t.Fatalf("services = %v, want [run-dev]", services)
	}
	ids := []string{}
	for _, s := range regular {
		ids = append(ids, s.ID)
	}
	if len(ids) != 2 {
		t.Fatalf("regular = %v, want logs+unit", ids)
	}
}
