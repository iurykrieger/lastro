package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

func isWindows() bool {
	return strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") ||
		strings.HasSuffix(strings.ToLower(os.Getenv("ComSpec")), "cmd.exe")
}

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
		Uses: []string{"fake"},
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
		Uses: []string{"fake"},
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
		Uses: []string{"fake"},
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
