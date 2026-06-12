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

// A scanner step (npm audit, gosec, grep-based secret scan) signals
// findings via a non-zero exit. As long as a signal_matches matcher fired,
// the run is a graded completion — never an ErrStepCrashed/inconclusive.
func TestRunAssertion_NonZeroExitWithMatchedSignalsGrades(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "scanner-sensor", UseCaseID: "fake-uc",
		Angle: enums.AngleSecurity, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "audit", Run: "echo 'dep-audit critical=1 high=2'; exit 1"},
		},
		SignalMatches: []sensor.SignalMatch{
			{Key: "dep-audit-clean", Pattern: "dep-audit critical=0 high=0", Verdict: enums.VerdictPass},
			{Key: "dep-audit-findings", Pattern: "dep-audit critical=[1-9]|high=[1-9]", Verdict: enums.VerdictFail,
				HealHint: &sensor.MatchHealHint{Summary: "Vulnerable dependencies", Rationale: "The dependency audit reported high/critical advisories."}},
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
	if agg.TerminationReason != enums.TerminationCompleted {
		t.Errorf("termination_reason = %q, want completed (non-zero exit with signals is not a crash)", agg.TerminationReason)
	}
	if agg.Verdict != enums.VerdictFail {
		t.Errorf("verdict = %q, want fail", agg.Verdict)
	}
	if agg.HealHint == nil {
		t.Error("heal_hint is nil; want the fail matcher's hint propagated")
	}
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

func TestRun_EnvFileValueReachesStepProcess(t *testing.T) {
	repo := t.TempDir()
	envFile := filepath.Join(repo, ".env")
	if err := os.WriteFile(envFile, []byte("HARNESS_T8_TOKEN=fromenvfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "env-inject", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{Key: "seen", Pattern: "token=present", Verdict: enums.VerdictPass},
			// Canary: fires when the env_file value did NOT reach the child.
			{Key: "env-absent", Pattern: "token=absent", Verdict: enums.VerdictFail,
				HealHint: &sensor.MatchHealHint{Summary: "env_file value missing", Rationale: "ambient injection failed"}},
		},
		Steps: []sensor.Step{{ID: "only", Run: `[ "$HARNESS_T8_TOKEN" = "fromenvfile" ] && echo "token=present" || echo "token=absent"`}},
	}
	ex := New(Options{
		RepoRoot: repo, EnvFile: envFile,
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})
	agg, err := ex.Run(context.Background(), s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (env_file value did not reach the child)", agg.Verdict)
	}
	if agg.Rollup.TotalSignals == 0 {
		t.Error("zero signals: the pass matcher never fired, test would be vacuous")
	}
}

func TestRun_HostWinsOverEnvFile(t *testing.T) {
	t.Setenv("HARNESS_T8_CLASH", "fromhost")
	repo := t.TempDir()
	envFile := filepath.Join(repo, ".env")
	if err := os.WriteFile(envFile, []byte("HARNESS_T8_CLASH=fromfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "env-clash", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		SignalMatches: []sensor.SignalMatch{
			{Key: "host-won", Pattern: "clash=fromhost", Verdict: enums.VerdictPass},
			// Canary: fires only if the env_file value overrides the host, flipping the verdict to fail.
			{Key: "file-leaked", Pattern: "clash=fromfile", Verdict: enums.VerdictFail,
				HealHint: &sensor.MatchHealHint{Summary: "file value leaked", Rationale: "host must win"}},
		},
		Steps: []sensor.Step{{ID: "only", Run: `echo "clash=$HARNESS_T8_CLASH"`}},
	}
	ex := New(Options{
		RepoRoot: repo, EnvFile: envFile,
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})
	agg, err := ex.Run(context.Background(), s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (host value should win)", agg.Verdict)
	}
}

func TestRun_MissingEnvRefAggregatesInconclusive(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "env-missing-ref", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake-stack"},
		Steps: []sensor.Step{{ID: "only", Run: `echo "t=${{ env.HARNESS_T9_ABSENT }}"`}},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})
	runDir := t.TempDir()
	agg, err := ex.Run(context.Background(), s, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictInconclusive {
		t.Errorf("verdict = %q, want inconclusive", agg.Verdict)
	}
	if agg.HealHint != nil {
		t.Errorf("inconclusive aggregate must not carry a heal hint, got %+v", agg.HealHint)
	}
	b, _ := os.ReadFile(filepath.Join(runDir, "signals.jsonl"))
	if !strings.Contains(string(b), "missing-env") || !strings.Contains(string(b), "HARNESS_T9_ABSENT") {
		t.Errorf("signals.jsonl missing the typed missing-env record: %s", b)
	}
}

func TestRun_SensorDeclaredRequiredEnvBlocksAllSteps(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	marker := filepath.Join(t.TempDir(), "ran")
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "env-declared", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake-stack"},
		Env:   map[string]sensor.EnvSpec{"HARNESS_T9_REQ": {Description: "needed"}},
		Steps: []sensor.Step{{ID: "only", Run: "touch " + marker}},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})
	runDir := t.TempDir()
	agg, err := ex.Run(context.Background(), s, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictInconclusive {
		t.Errorf("verdict = %q, want inconclusive", agg.Verdict)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("step ran despite missing required env (must be pre-spawn)")
	}
	b, _ := os.ReadFile(filepath.Join(runDir, "signals.jsonl"))
	if !strings.Contains(string(b), "missing-env") || !strings.Contains(string(b), "HARNESS_T9_REQ") {
		t.Errorf("signals.jsonl missing the typed missing-env record: %s", b)
	}
}

// TestRun_StepEnvRefDerivedValueIsRedacted proves that the step.go
// ref-derived registration site (runStep's own env: block) is the sole
// redaction source when the secret comes from the HOST environment. No
// EnvFile is declared, so the ambient registration loop in executor.go Run
// never fires. The ${{ env.HARNESS_T10_HOSTSECRET }} ref in the step's
// env: map resolves from the host and the resolved value is registered via
// a.Redactor.Add — the only masking path exercised by this test.
//
// Mutation evidence: commenting out the step.go `a.Redactor.Add(val)` block
// (lines ~116-120) causes this test to fail because raw.log contains the
// unredacted literal "host-secret-value".
func TestRun_StepEnvRefDerivedValueIsRedacted(t *testing.T) {
	t.Setenv("HARNESS_T10_HOSTSECRET", "host-secret-value")

	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "step-env-redact", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{
				ID: "leak",
				// Inject the host var via the step's env: map using a ref.
				// The step then leaks the injected value so redaction is
				// exercised regardless of whether it matches a signal pattern.
				Env: map[string]string{"INJECTED": "${{ env.HARNESS_T10_HOSTSECRET }}"},
				Run: `echo "leak=$INJECTED"`,
			},
		},
	}
	// No EnvFile — host vars are deliberately NOT ambient-registered.
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})

	runDir := t.TempDir()
	_, err := ex.Run(context.Background(), s, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// raw.log must contain the leak line with the value masked, and must
	// NOT contain the literal secret. This assertion fails when the
	// step.go ref-derived registration site is disabled.
	rawBytes, _ := os.ReadFile(filepath.Join(runDir, "raw.log"))
	rawStr := string(rawBytes)
	if !strings.Contains(rawStr, "leak=***") {
		t.Errorf("raw.log does not contain redacted leak line (leak=***): %s", rawStr)
	}
	if strings.Contains(rawStr, "host-secret-value") {
		t.Errorf("raw.log contains unredacted secret: %s", rawStr)
	}
}

func TestRun_UnparseableEnvFileAggregatesInconclusive(t *testing.T) {
	repo := t.TempDir()
	envFile := filepath.Join(repo, ".env")
	if err := os.WriteFile(envFile, []byte("KEY=\"unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "env-bad-file", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake-stack"},
		Steps: []sensor.Step{{ID: "only", Run: "echo never"}},
	}
	ex := New(Options{
		RepoRoot: repo, EnvFile: envFile,
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})
	runDir := t.TempDir()
	agg, err := ex.Run(context.Background(), s, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run must not error (signal instead): %v", err)
	}
	if agg.Verdict != enums.VerdictInconclusive {
		t.Errorf("verdict = %q, want inconclusive", agg.Verdict)
	}
	if agg.HealHint != nil {
		t.Errorf("inconclusive aggregate must not carry a heal hint, got %+v", agg.HealHint)
	}
	b, _ := os.ReadFile(filepath.Join(runDir, "signals.jsonl"))
	if !strings.Contains(string(b), "env-file-invalid") {
		t.Errorf("signals.jsonl missing env-file-invalid: %s", b)
	}
}
