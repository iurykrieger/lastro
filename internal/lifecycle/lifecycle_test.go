package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	rxexec "github.com/iurykrieger/lastro/internal/runtime/executor"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

var fakeSensorBin string

func TestMain(m *testing.M) {
	// Skip the build when we are the re-exec'd child; the binary path is
	// passed via HARNESS_TEST_FAKE_SENSOR instead.
	if os.Getenv("HARNESS_TEST_CHILD") != "1" {
		bin, err := buildFakeSensor()
		if err != nil {
			panic("build fakesensor: " + err.Error())
		}
		fakeSensorBin = bin
		defer os.Remove(bin)
	}
	os.Exit(m.Run())
}

func buildFakeSensor() (string, error) {
	dir, err := os.MkdirTemp("", "fakesensor-lc-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "fakesensor")
	if isWindows() {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "../testutil/fakesensor/main.go")
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

func TestRunSensor_ErrSensorNotFound(t *testing.T) {
	lc := newTestLifecycle(t, nil)
	_, err := lc.RunSensor(context.Background(), "no-such-sensor", nil)
	if !errors.Is(err, ErrSensorNotFound) {
		t.Errorf("err = %v, want ErrSensorNotFound", err)
	}
}

func TestRunSensor_AssertionPass(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "lifecycle-assertion-pass", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake"},
		Steps: []sensor.Step{{ID: "only", Run: fakeSensorBin + " signal pass"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	agg, err := lc.RunSensor(context.Background(), s.ID, nil)
	if err != nil {
		t.Fatalf("RunSensor: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass", agg.Verdict)
	}

	matches, _ := filepath.Glob(filepath.Join(lc.opts.RuntimeRoot, s.ID, "*", "aggregate.json"))
	if len(matches) != 1 {
		t.Errorf("aggregate.json count = %d, want 1", len(matches))
	}

	entries, _ := lc.registry.List()
	if len(entries) != 0 {
		t.Errorf("registry entries after Run = %d, want 0; entries: %+v", len(entries), entries)
	}
}

// newTestLifecycle constructs a Lifecycle with a stub sensor store and
// an executor wired with empty stores. Uses deterministic NewRunID.
func newTestLifecycle(t *testing.T, sensors []sensor.Sensor) *Lifecycle {
	t.Helper()
	store := &stubSensorStore{by: map[string]sensor.Sensor{}}
	for _, s := range sensors {
		store.by[s.ID] = s
	}
	uc := &usecase.UseCase{ID: "lifecycle-uc"}
	ex := rxexec.New(rxexec.Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
	})
	root := t.TempDir()
	counter := 0
	return New(Options{
		Sensors:     store,
		Executor:    ex,
		RuntimeRoot: root,
		NewRunID: func() string {
			counter++
			return strings.Repeat("0", 25) + string(rune('A'+counter-1))
		},
		Version: "test-0.0.0",
	})
}

// stubSensorStore satisfies the SensorStore interface used by Lifecycle.
type stubSensorStore struct{ by map[string]sensor.Sensor }

func (s *stubSensorStore) Lookup(id string) (sensor.Sensor, bool) {
	v, ok := s.by[id]
	return v, ok
}

// emptyStore satisfies fixture.FixtureStore with no-op implementations.
type emptyStore struct{}

func (emptyStore) LookupFixture(id string) (fixture.Fixture, bool) { return fixture.Fixture{}, false }
func (emptyStore) FixturesForUseCase(uc string) []fixture.Fixture  { return nil }
func (emptyStore) All() []fixture.Fixture                          { return nil }

func TestStartSensor_ObservationalEmitsAndAppearsInRegistry(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "obs-pass", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses: []string{"fake"},
		Steps: []sensor.Step{
			{ID: "watch", Run: fakeSensorBin + " watch --emit order-received --emit order-validated --emit order-persisted --interval 30ms"},
		},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	h, err := lc.StartSensor(context.Background(), s.ID, []string{"order-received", "order-validated", "order-persisted"})
	if err != nil {
		t.Fatalf("StartSensor: %v", err)
	}
	if h == nil || h.RunID == "" {
		t.Fatalf("nil handle / empty RunID")
	}

	// Registry should now show one entry.
	entries, _ := lc.registry.List()
	if len(entries) != 1 || entries[0].SensorID != s.ID {
		t.Errorf("registry entries = %+v, want 1 for %q", entries, s.ID)
	}

	t.Cleanup(func() {
		_, _ = lc.StopSensor(context.Background(), h)
	})

	// Wait briefly for signals.jsonl to accumulate.
	signalsPath := filepath.Join(h.RunDir, "signals.jsonl")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(signalsPath); err == nil && bytes.Count(b, []byte{'\n'}) >= 3 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("signals.jsonl did not accumulate 3 lines within deadline")
}

func TestStartSensor_ErrAssertionSensor(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "assertion-only", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake"},
		Steps: []sensor.Step{{ID: "only", Run: fakeSensorBin + " signal pass"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	_, err := lc.StartSensor(context.Background(), s.ID, nil)
	if !errors.Is(err, ErrAssertionSensor) {
		t.Errorf("err = %v, want ErrAssertionSensor", err)
	}
}

func TestStopSensor_InProcessFastPath(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "obs-stop", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses:  []string{"fake"},
		Steps: []sensor.Step{{ID: "watch", Run: fakeSensorBin + " watch --emit k1 --interval 20ms"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	h, err := lc.StartSensor(context.Background(), s.ID, []string{"k1"})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(80 * time.Millisecond) // let it emit the one observation

	agg, err := lc.StopSensor(context.Background(), h)
	if err != nil {
		t.Fatalf("StopSensor: %v", err)
	}
	if agg.TerminationReason != enums.TerminationStopped {
		t.Errorf("termination_reason = %q, want stopped", agg.TerminationReason)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (observation arrived)", agg.Verdict)
	}

	entries, _ := lc.ListRunning()
	if len(entries) != 0 {
		t.Errorf("ListRunning after Stop = %d, want 0", len(entries))
	}
}

func TestStopSensor_FailWhenObservationMissing(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "obs-missing", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses:  []string{"fake"},
		Steps: []sensor.Step{{ID: "watch", Run: fakeSensorBin + " watch --emit k1 --interval 20ms"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	h, err := lc.StartSensor(context.Background(), s.ID, []string{"k1", "k2-never-arrives"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)

	agg, err := lc.StopSensor(context.Background(), h)
	if err != nil {
		t.Fatalf("StopSensor: %v", err)
	}
	if agg.Verdict != enums.VerdictFail {
		t.Errorf("verdict = %q, want fail (missing observation)", agg.Verdict)
	}
	if agg.HealHint == nil {
		t.Errorf("heal_hint is nil; want observational-missing hint")
	}
}

func TestStopFromOtherProcess(t *testing.T) {
	// Only run as the "parent" half here; the child half exits directly.
	if os.Getenv("HARNESS_TEST_CHILD") == "1" {
		t.Skip("invoked as child; will be re-entered via the child test")
	}

	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "obs-cross", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses:  []string{"fake"},
		Steps: []sensor.Step{{ID: "watch", Run: fakeSensorBin + " watch --emit k1 --interval 30ms"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	h, err := lc.StartSensor(context.Background(), s.ID, []string{"k1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = lc.StopSensor(context.Background(), h) })

	time.Sleep(80 * time.Millisecond) // let one observation arrive

	// Re-exec ourselves as the "child" run. The child looks the run up
	// via the registry and calls StopSensor.
	cmd := exec.Command(os.Args[0],
		"-test.run", "TestStopFromOtherProcess_Child",
		"-test.v",
	)
	cmd.Env = append(os.Environ(),
		"HARNESS_TEST_CHILD=1",
		"HARNESS_TEST_RUNTIME_ROOT="+lc.opts.RuntimeRoot,
		"HARNESS_TEST_SENSOR_ID="+s.ID,
		"HARNESS_TEST_RUN_ID="+h.RunID,
		"HARNESS_TEST_FAKE_SENSOR="+fakeSensorBin,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child invocation failed: %v\noutput:\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("verdict=pass")) {
		t.Errorf("child did not report verdict=pass; output:\n%s", out)
	}
}

// TestStopFromOtherProcess_Child is the re-entrant half. It only runs
// when HARNESS_TEST_CHILD=1, which the parent sets.
func TestStopFromOtherProcess_Child(t *testing.T) {
	if os.Getenv("HARNESS_TEST_CHILD") != "1" {
		t.Skip("not invoked as child")
	}
	root := os.Getenv("HARNESS_TEST_RUNTIME_ROOT")
	sensorID := os.Getenv("HARNESS_TEST_SENSOR_ID")
	runID := os.Getenv("HARNESS_TEST_RUN_ID")
	fake := os.Getenv("HARNESS_TEST_FAKE_SENSOR")

	// Re-register the sensor so synthesizeOrphanAggregate can resolve.
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: sensorID, UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses:  []string{"fake"},
		Steps: []sensor.Step{{ID: "watch", Run: fake + " watch --emit k1"}},
	}
	store := &stubSensorStore{by: map[string]sensor.Sensor{s.ID: s}}
	uc := &usecase.UseCase{ID: "lifecycle-uc"}
	ex := rxexec.New(rxexec.Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
	})
	lc := New(Options{
		Sensors: store, Executor: ex, RuntimeRoot: root,
		NewRunID: func() string { return "child-not-used" },
		Version:  "test-child",
	})
	h, err := lc.LoadHandle(sensorID, runID)
	if err != nil {
		t.Fatalf("LoadHandle: %v", err)
	}
	agg, err := lc.StopSensor(context.Background(), h)
	if err != nil {
		t.Fatalf("StopSensor: %v", err)
	}
	// The parent greps for this exact substring in our stdout.
	fmt.Printf("verdict=%s\n", agg.Verdict)
}

func TestRunWatcher_RegistersRunsAndDeregisters(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "obs-watcher", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses:  []string{"fake"},
		Steps: []sensor.Step{{ID: "watch", Run: fakeSensorBin + " watch --emit k1 --emit k2 --interval 20ms"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})
	runID := lc.GenerateRunID()
	runDir := lc.RunDirFor(s.ID, runID)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- lc.RunWatcher(ctx, s.ID, runID, runDir, []string{"k1", "k2"}) }()

	// While running: the watcher must register itself with this process's
	// PID, and signals.jsonl must accumulate.
	signalsPath := filepath.Join(runDir, "signals.jsonl")
	deadline := time.Now().Add(2 * time.Second)
	registered := false
	for time.Now().Before(deadline) {
		if e, ok, _ := lc.FindRunning(s.ID, runID); ok && e.PID == os.Getpid() {
			registered = true
			if b, err := os.ReadFile(signalsPath); err == nil && bytes.Count(b, []byte{'\n'}) >= 2 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !registered {
		cancel()
		<-done
		t.Fatalf("watcher never registered in running_sensors.json")
	}

	cancel() // graceful stop, like a cross-process SIGTERM
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("RunWatcher did not return after ctx cancel")
	}

	if _, ok := readAggregateJSON(filepath.Join(runDir, "aggregate.json")); !ok {
		t.Errorf("aggregate.json not written after RunWatcher returned")
	}
	entries, _ := lc.registry.List()
	if len(entries) != 0 {
		t.Errorf("registry entries after RunWatcher = %d, want 0", len(entries))
	}
}

func TestStartSensor_RejectsSecondLiveInstanceOfSameSensor(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "run-dev", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses: []string{"fake"},
		Steps: []sensor.Step{
			{ID: "boot", Run: fakeSensorBin + " watch --emit ready --interval 30ms"},
		},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})
	ctx := context.Background()

	h1, err := lc.StartSensor(ctx, "run-dev", []string{"ready"})
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	t.Cleanup(func() { _, _ = lc.StopSensor(ctx, h1) })

	_, err = lc.StartSensor(ctx, "run-dev", []string{"ready"})
	if !errors.Is(err, ErrServiceAlreadyRunning) {
		t.Fatalf("second start err = %v, want ErrServiceAlreadyRunning", err)
	}
}

func TestRunWatcher_ErrAssertionSensor(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "watcher-assertion", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses:  []string{"fake"},
		Steps: []sensor.Step{{ID: "only", Run: fakeSensorBin + " signal pass"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})
	err := lc.RunWatcher(context.Background(), s.ID, lc.GenerateRunID(), t.TempDir(), nil)
	if !errors.Is(err, ErrAssertionSensor) {
		t.Errorf("err = %v, want ErrAssertionSensor", err)
	}
}

func TestRunWatcher_ErrSensorNotFound(t *testing.T) {
	lc := newTestLifecycle(t, nil)
	err := lc.RunWatcher(context.Background(), "no-such-sensor", lc.GenerateRunID(), t.TempDir(), nil)
	if !errors.Is(err, ErrSensorNotFound) {
		t.Errorf("err = %v, want ErrSensorNotFound", err)
	}
}
