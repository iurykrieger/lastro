package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// TestSharedRunDev_SingleInstance_NoLockCollision reproduces issue #33:
// a use case whose logs sensor attaches to a shared run-dev observational
// service. Before the shared-service wiring, run-dev was spawned twice (once
// as its own observational sensor, once inline-expanded into the logs sensor),
// colliding on Next.js's .next/dev/lock and crashing the second instance with
// no signals → inconclusive.
//
// The consumer uses the realistic shape that flows through the validate path:
//   - depends_on: [run-dev] so GatherForUseCase pulls the core service in and
//     classifyServices recognizes it as a shared service;
//   - a step `uses: run-dev` so the executor attaches to the live signal stream
//     instead of inline-expanding run-dev's boot step.
//
// Expectation after the fix: run-dev starts exactly once, the logs sensor
// produces its `served` signal from run-dev's stream, and the use-case sensor
// verdict is NOT the crash-induced inconclusive.
func TestSharedRunDev_SingleInstance_NoLockCollision(t *testing.T) {
	// A long-lived fake "server": announce readiness, then emit request log
	// lines forever (killed on Release). The `ready` matcher unblocks
	// servicemgr.Acquire; the `log-line` firehose carries each line so the
	// attaching logs sensor can grep matched_line for its own pattern.
	runDev := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "run-dev", Scope: enums.ScopeCore,
		Angle: enums.AngleEnvironment, Kind: enums.KindObservational,
		Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses: []string{},
		SignalMatches: []sensor.SignalMatch{
			{Key: "ready", Pattern: "ready", Verdict: enums.VerdictPass, Expected: true},
			{Key: "log-line", Pattern: ".+", Verdict: enums.VerdictPass},
		},
		Steps: []sensor.Step{
			{ID: "boot", Run: `echo "ready - started server"; while true; do echo "GET /item 200"; sleep 0.2; done`},
		},
	}
	logs := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "logs", UseCaseID: "uc-x",
		Angle: enums.AngleLogs, Kind: enums.KindObservational,
		Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses:      []string{},
		DependsOn: []string{"run-dev"},
		SignalMatches: []sensor.SignalMatch{
			{Key: "served", Pattern: "GET /", Verdict: enums.VerdictPass, Expected: true},
		},
		ObserveWindow: "10s",
		Steps:         []sensor.Step{{ID: "watch", Uses: "run-dev"}},
	}

	store, err := sensor.NewStore(runDev, logs)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	fixtures, err := fixture.LoadDirectory(t.TempDir())
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	runtimeRoot := t.TempDir()
	arts := &HarnessArtifacts{
		Sensors:     store,
		UseCases:    map[string]*usecase.UseCase{"uc-x": {ID: "uc-x"}},
		Fixtures:    fixtures,
		RuntimeRoot: runtimeRoot,
	}

	runner, mgr, cleanup, err := defaultRunnerFactory(arts, t.TempDir())
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	defer cleanup()

	// Mirror RunUseCase's gather → classify → service-dep derivation so the
	// test exercises the real scheduling path (services excluded from the
	// wavefront node set; their lifetime managed by acquire/release).
	owned := arts.Sensors.GatherForUseCase("uc-x")
	services, regular := classifyServices(owned)
	if len(services) != 1 || services[0].ID != "run-dev" {
		t.Fatalf("services = %+v, want exactly [run-dev]", services)
	}
	isService := map[string]bool{}
	for _, s := range services {
		isService[s.ID] = true
	}
	deps := serviceDepsOf(regular, isService)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	results, err := runUseCaseSensors(ctx, runner, regular, nil, mgr, deps)
	if err != nil {
		t.Fatalf("runUseCaseSensors: %v", err)
	}

	// The logs sensor must have attached to run-dev's stream and produced a
	// verdict — never the collision-induced inconclusive.
	var sawLogs bool
	for _, r := range results {
		if r.SensorID == "logs" {
			sawLogs = true
			if r.Verdict == enums.VerdictInconclusive {
				t.Fatalf("logs verdict inconclusive — shared run-dev attach regression (#33): %+v", r)
			}
		}
	}
	if !sawLogs {
		t.Fatalf("no aggregate for the logs sensor in results = %+v", results)
	}

	// run-dev must have started exactly once. Each StartSensor mints one run
	// dir under RuntimeRoot/run-dev/<runID>; a second spawn would add another.
	entries, err := os.ReadDir(filepath.Join(runtimeRoot, "run-dev"))
	if err != nil {
		t.Fatalf("read run-dev runtime dir: %v", err)
	}
	starts := 0
	for _, e := range entries {
		if e.IsDir() {
			starts++
		}
	}
	if starts != 1 {
		t.Fatalf("run-dev started %d times, want exactly 1", starts)
	}
}
