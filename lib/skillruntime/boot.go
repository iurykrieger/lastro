package skillruntime

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/lifecycle"
	"github.com/iurykrieger/lastro/internal/runtime/executor"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// HarnessVersion is the version string recorded in Handle entries by the
// constructed Lifecycle. Hardcoded for B5 sub-PR 1; replace with a
// build-time variable later.
const HarnessVersion = "0.1.0-b5"

// Booted is the bundle of objects every B5 skill script gets from
// BootLifecycle. The script keeps a reference to *Booted for the duration
// of the invocation and calls Cleanup before exit.
type Booted struct {
	Lifecycle   *lifecycle.Lifecycle
	Sensors     *sensor.Store
	Fixtures    *fixture.Store
	UseCases    map[string]*usecase.UseCase
	RuntimeRoot string // <repoRoot>/.harness/runtime — used by skills that need to walk run dirs
	Cleanup     func() error
}

// BootLifecycle loads .harness/{sensors,fixtures,use-cases} from disk and
// returns a configured *lifecycle.Lifecycle ready for RunSensor /
// StartSensor / StopSensor calls. Returns an error if .harness/ is
// missing or any sub-store fails to load.
func BootLifecycle(repoRoot string) (*Booted, error) {
	harnessDir := filepath.Join(repoRoot, ".harness")
	if info, err := os.Stat(harnessDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("skillruntime: .harness/ not found at %s", harnessDir)
	}

	sensorStore, err := loadSensors(filepath.Join(harnessDir, "sensors"))
	if err != nil {
		return nil, fmt.Errorf("skillruntime: load sensors: %w", err)
	}

	fixtureStore, err := loadFixtures(filepath.Join(harnessDir, "fixtures"))
	if err != nil {
		return nil, fmt.Errorf("skillruntime: load fixtures: %w", err)
	}

	useCases, err := loadUseCases(filepath.Join(harnessDir, "use-cases"), fixtureStore)
	if err != nil {
		return nil, fmt.Errorf("skillruntime: load use cases: %w", err)
	}

	entryPoints := collectEntryPoints(useCases)

	resolver := &template.Resolver{
		Fixtures:    fixtureStore,
		EntryPoints: entryPoints,
	}

	exec := executor.New(executor.Options{
		RepoRoot:     repoRoot,
		Resolver:     resolver,
		FixtureStore: fixtureStore,
		UseCaseLookup: func(sensorID string) (*usecase.UseCase, bool) {
			s, ok := sensorStore.LookupSensor(sensorID)
			if !ok {
				return nil, false
			}
			uc, ok := useCases[s.UseCaseID]
			return uc, ok
		},
		// Required for sensors that compose core primitives via uses-steps
		// (e.g. an e2e-test sensor that `uses: e2e-test`).
		SensorLookup: sensorStore.LookupSensor,
	})

	runtimeRoot := filepath.Join(harnessDir, "runtime")
	if err := os.MkdirAll(runtimeRoot, 0o755); err != nil {
		return nil, fmt.Errorf("skillruntime: mkdir runtime: %w", err)
	}

	lc := lifecycle.New(lifecycle.Options{
		Sensors:     lifecycle.WrapSensorStore(sensorStore),
		Executor:    exec,
		RuntimeRoot: runtimeRoot,
		Version:     HarnessVersion,
	})

	return &Booted{
		Lifecycle:   lc,
		Sensors:     sensorStore,
		Fixtures:    fixtureStore,
		UseCases:    useCases,
		RuntimeRoot: runtimeRoot,
		Cleanup:     func() error { return nil },
	}, nil
}

// loadSensors returns an empty store if the directory is empty;
// sensor.LoadDirectory must tolerate that or we wrap.
func loadSensors(dir string) (*sensor.Store, error) {
	if !dirHasYAML(dir) {
		return sensor.NewStore()
	}
	return sensor.LoadDirectory(dir)
}

func loadFixtures(dir string) (*fixture.Store, error) {
	if !dirHasYAML(dir) {
		return fixture.NewStore()
	}
	return fixture.LoadDirectory(dir)
}

func loadUseCases(dir string, fixtures fixture.FixtureStore) (map[string]*usecase.UseCase, error) {
	out := map[string]*usecase.UseCase{}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return out, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		uc, err := usecase.Load(data, fixtures)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", e.Name(), err)
		}
		out[uc.ID] = uc
	}
	return out, nil
}

func collectEntryPoints(useCases map[string]*usecase.UseCase) map[string]entrypoint.EntryPoint {
	out := map[string]entrypoint.EntryPoint{}
	for _, uc := range useCases {
		for _, ep := range uc.EntryPoints {
			out[ep.ID] = ep
		}
	}
	return out
}

func dirHasYAML(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			return true
		}
	}
	// LoadDirectory descends one level into subdirectories, so detection must too:
	// sensors live under sensors/core/ and sensors/<use_case_id>/.
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subEntries, err := os.ReadDir(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, se := range subEntries {
			if !se.IsDir() && filepath.Ext(se.Name()) == ".yaml" {
				return true
			}
		}
	}
	return false
}
