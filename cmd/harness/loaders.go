package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/policy"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/stack"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// HarnessArtifacts bundles every artifact required by `harness validate`.
// Loading is atomic: a missing file fails the whole load.
type HarnessArtifacts struct {
	HarnessDir    string
	RuntimeRoot   string
	StackManifest stack.StackManifest
	Fixtures      *fixture.Store
	Sensors       *sensor.Store
	UseCases      map[string]*usecase.UseCase // keyed by UseCase.ID
	Policy        *policy.EffectivePolicy
}

// LoadHarnessArtifacts reads the .harness/ tree rooted at harnessDir.
// All four directories (use-cases, fixtures, sensors) and both files
// (stack-manifest.yaml, validation-policy.yaml) must exist.
//
// policyPath is resolved by resolvePolicyPath() and may live outside
// harnessDir when --policy or HARNESS_POLICY is set.
func LoadHarnessArtifacts(harnessDir, policyPath string) (*HarnessArtifacts, error) {
	manifestPath := filepath.Join(harnessDir, "stack-manifest.yaml")
	manifest, err := stack.Load(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("load stack manifest: %w", err)
	}

	fixturesDir := filepath.Join(harnessDir, "fixtures")
	fixtureStore, err := fixture.LoadDirectory(fixturesDir)
	if err != nil {
		return nil, fmt.Errorf("load fixtures: %w", err)
	}

	useCases, err := loadUseCases(filepath.Join(harnessDir, "use-cases"), fixtureStore)
	if err != nil {
		return nil, fmt.Errorf("load use cases: %w", err)
	}

	sensorsDir := filepath.Join(harnessDir, "sensors")
	sensorStore, err := sensor.LoadDirectory(sensorsDir)
	if err != nil {
		return nil, fmt.Errorf("load sensors: %w", err)
	}

	pol, err := loadPolicy(policyPath)
	if err != nil {
		return nil, fmt.Errorf("load policy: %w", err)
	}

	return &HarnessArtifacts{
		HarnessDir:    harnessDir,
		RuntimeRoot:   filepath.Join(harnessDir, "runtime"),
		StackManifest: manifest,
		Fixtures:      fixtureStore,
		Sensors:       sensorStore,
		UseCases:      useCases,
		Policy:        pol,
	}, nil
}

func loadUseCases(dir string, fixtures fixture.FixtureStore) (map[string]*usecase.UseCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read use cases dir %s: %w", dir, err)
	}
	out := make(map[string]*usecase.UseCase)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".yaml" && filepath.Ext(name) != ".yml" {
			continue
		}
		full := filepath.Join(dir, name)
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", full, err)
		}
		uc, err := usecase.Load(data, fixtures)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", full, err)
		}
		if _, dup := out[uc.ID]; dup {
			return nil, fmt.Errorf("duplicate use case id %q in %s", uc.ID, full)
		}
		out[uc.ID] = uc
	}
	if len(out) == 0 {
		return nil, errors.New("no use case YAMLs found")
	}
	return out, nil
}

func loadPolicy(path string) (*policy.EffectivePolicy, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open policy: %w", err)
	}
	defer f.Close()
	local, err := policy.Load(f)
	if err != nil {
		return nil, fmt.Errorf("decode policy: %w", err)
	}
	// Two-scope resolution: v1 has no global, so global=nil; local-only
	// flow is exactly what policy.Resolve already supports.
	return policy.Resolve(nil, local), nil
}
