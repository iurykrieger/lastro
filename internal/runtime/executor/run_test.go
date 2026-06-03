package executor

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// fakeSensorBin holds the path to the compiled fakesensor binary, built
// once by TestMain.
var fakeSensorBin string

func TestMain(m *testing.M) {
	bin, err := buildFakeSensor()
	if err != nil {
		panic("build fakesensor: " + err.Error())
	}
	fakeSensorBin = bin
	code := m.Run()
	_ = os.Remove(bin)
	os.Exit(code)
}

func buildFakeSensor() (string, error) {
	dir, err := os.MkdirTemp("", "fakesensor-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "fakesensor")
	if isWindows() {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "../../testutil/fakesensor/main.go")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

// isWindows returns true if the current binary was built for Windows.
// We use runtime.GOOS rather than environment variables because WSL
// inherits OS=Windows_NT from the host shell despite running Linux.
func isWindows() bool { return runtime.GOOS == "windows" }

func TestRunAssertion_PassSingleStep(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0",
		ID:            "fake-pass-sensor",
		UseCaseID:     "fake-uc",
		Angle:         enums.AngleBuild,
		Kind:          enums.KindAssertion,
		Nature:        enums.NatureComputational,
		OutputType:    enums.OutputSingleShot,
		Uses:          []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "only", Run: fakeSensorBin + " signal pass"},
		},
	}
	ex := New(Options{
		RepoRoot:     t.TempDir(),
		Resolver:     &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore: emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) {
			if id == s.ID {
				return uc, true
			}
			return nil, false
		},
		Now: fixedExecNow,
	})

	runDir := t.TempDir()
	agg, err := ex.Run(context.Background(), s, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass", agg.Verdict)
	}
	if agg.TerminationReason != enums.TerminationCompleted {
		t.Errorf("termination_reason = %q, want completed", agg.TerminationReason)
	}
	if agg.Rollup.TotalSignals != 1 || agg.Rollup.PassCount != 1 {
		t.Errorf("rollup = %+v, want 1 pass", agg.Rollup)
	}
	// signals.jsonl should contain exactly one decoded line.
	b, _ := os.ReadFile(filepath.Join(runDir, "signals.jsonl"))
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("signals.jsonl line count = %d, want 1", len(lines))
	}
}

func TestRunAssertion_GoldenAggregate(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "fake-pass-sensor", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake-stack"},
		Steps: []sensor.Step{{ID: "only", Run: fakeSensorBin + " signal pass --angle build"}},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})

	agg, err := ex.Run(context.Background(), s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := normalizeForGolden(agg)
	want := readGolden(t, "testdata/golden/assertion_pass.json")
	if got != want {
		t.Errorf("aggregate mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func normalizeForGolden(a aggregate.AggregateSignal) string {
	a.StartedAt = goldenTime
	a.EndedAt = goldenTime
	b, _ := json.MarshalIndent(a, "", "  ")
	return string(b)
}

func readGolden(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	return strings.TrimRight(string(b), "\n")
}

var goldenTime, _ = time.Parse(time.RFC3339Nano, "2026-05-24T10:00:00Z")
var fixedExecNow = func() time.Time { return goldenTime }

type emptyStore struct{}

func (emptyStore) LookupFixture(id string) (fixture.Fixture, bool) { return fixture.Fixture{}, false }
func (emptyStore) FixturesForUseCase(uc string) []fixture.Fixture  { return nil }
func (emptyStore) All() []fixture.Fixture                          { return nil }

func TestRunAssertion_ContextCancellationKillsChild(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "slow-sensor", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake-stack"},
		Steps: []sensor.Step{{ID: "slow", Run: fakeSensorBin + " sleep 5s"}},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	agg, err := ex.Run(ctx, s, t.TempDir(), nil, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run returned error (should still complete with verdict): %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run took %v; should have aborted quickly after cancel", elapsed)
	}
	if agg.TerminationReason != enums.TerminationStopped {
		t.Errorf("termination_reason = %q, want stopped", agg.TerminationReason)
	}
}

func TestRunAssertion_TimeoutReports(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "slow-sensor", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake-stack"},
		Steps: []sensor.Step{{ID: "slow", Run: fakeSensorBin + " sleep 5s"}},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	agg, err := ex.Run(ctx, s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.TerminationReason != enums.TerminationTimeout {
		t.Errorf("termination_reason = %q, want timeout", agg.TerminationReason)
	}
}

// TestRun_SignalMatchesCompleteness verifies the full signal_matches path
// through Run: a sensor whose step prints a line matching an expected-pass
// matcher produces a complete aggregate (no missing observations), while a
// sensor whose step prints a non-matching line is incomplete and receives a
// fail verdict.
func TestRun_SignalMatchesCompleteness(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}

	newSensor := func(id, echoText string) sensor.Sensor {
		return sensor.Sensor{
			SchemaVersion: "1.0.0",
			ID:            id,
			UseCaseID:     "fake-uc",
			Angle:         enums.AngleBuild,
			Kind:          enums.KindAssertion,
			Nature:        enums.NatureComputational,
			OutputType:    enums.OutputSingleShot,
			Uses:          []string{"fake-stack"},
			Steps: []sensor.Step{
				{ID: "s", Run: "echo '" + echoText + "'"},
			},
			SignalMatches: []sensor.SignalMatch{
				{Key: "ready", Pattern: "api ready on", Expected: true},
			},
		}
	}

	newExecutor := func() *Executor {
		return New(Options{
			RepoRoot:      t.TempDir(),
			Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
			FixtureStore:  emptyStore{},
			UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
			Now:           fixedExecNow,
		})
	}

	// Sensor A: step prints matching text → completeness must be non-nil with
	// zero missing observations.
	t.Run("matching_line_complete", func(t *testing.T) {
		s := newSensor("sensor-a", "api ready on :3030")
		agg, err := newExecutor().Run(context.Background(), s, t.TempDir(), nil, nil)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if agg.Completeness == nil {
			t.Fatal("completeness is nil; want non-nil for sensor with expected signal_matches")
		}
		if len(agg.Completeness.MissingObservations) != 0 {
			t.Errorf("MissingObservations = %v; want empty", agg.Completeness.MissingObservations)
		}
		if agg.Verdict != enums.VerdictPass {
			t.Errorf("verdict = %q; want pass", agg.Verdict)
		}
	})

	// Sensor B: step prints non-matching text → "ready" key is missing, verdict
	// must be fail.
	t.Run("non_matching_line_incomplete", func(t *testing.T) {
		s := newSensor("sensor-b", "nope")
		agg, err := newExecutor().Run(context.Background(), s, t.TempDir(), nil, nil)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if agg.Completeness == nil {
			t.Fatal("completeness is nil; want non-nil for sensor with expected signal_matches")
		}
		if len(agg.Completeness.MissingObservations) != 1 {
			t.Errorf("MissingObservations = %v; want [\"ready\"]", agg.Completeness.MissingObservations)
		}
		if agg.Verdict != enums.VerdictFail {
			t.Errorf("verdict = %q; want fail", agg.Verdict)
		}
	})
}

func TestRunAssertion_CrashedStepSynthesizesHint(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "crash-sensor", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "boom", Run: fakeSensorBin + ` crash --exit-code 2 --stderr "could not connect to redis"`},
		},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})

	agg, err := ex.Run(context.Background(), s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.TerminationReason != enums.TerminationError {
		t.Errorf("termination_reason = %q, want error", agg.TerminationReason)
	}
	if agg.HealHint == nil {
		t.Fatalf("heal_hint is nil; want synthesized hint")
	}
	if !strings.Contains(agg.HealHint.Rationale, "could not connect to redis") {
		t.Errorf("heal_hint.rationale missing stderr: %q", agg.HealHint.Rationale)
	}
}
