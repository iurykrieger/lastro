package lifecycle

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
