package sensor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
	"sigs.k8s.io/yaml"
)

func TestLoadSensor_BuildExample(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "examples", "sensor", "assertion-computational-single.yaml")
	s, err := LoadSensor(path)
	if err != nil {
		t.Fatalf("LoadSensor: %v", err)
	}
	if s.ID != "build-create-order-sensor" {
		t.Errorf("ID: got %q, want %q", s.ID, "build-create-order-sensor")
	}
	if s.UseCaseID != "create-order-use-case" {
		t.Errorf("UseCaseID: got %q, want %q", s.UseCaseID, "create-order-use-case")
	}
	if s.Angle != enums.AngleBuild {
		t.Errorf("Angle: got %q, want %q", s.Angle, enums.AngleBuild)
	}
	if s.Kind != enums.KindAssertion {
		t.Errorf("Kind: got %q, want %q", s.Kind, enums.KindAssertion)
	}
	if s.Nature != enums.NatureComputational {
		t.Errorf("Nature: got %q, want %q", s.Nature, enums.NatureComputational)
	}
	if s.OutputType != enums.OutputSingleShot {
		t.Errorf("OutputType: got %q, want %q", s.OutputType, enums.OutputSingleShot)
	}
	if got, want := s.Uses, []string{"node", "express"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Uses: got %v, want %v", got, want)
	}
	if len(s.Steps) != 1 {
		t.Fatalf("Steps: got %d, want 1", len(s.Steps))
	}
	if s.Steps[0].ID != "compile" || s.Steps[0].Run != "tsc --noEmit" {
		t.Errorf("Step[0]: got %+v", s.Steps[0])
	}
}

func TestLoadSensor_MissingFile(t *testing.T) {
	_, err := LoadSensor("does/not/exist.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadSensor_SchemaRejected(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		wantSubstr string
	}{
		{"missing top-level uses", "missing-uses.yaml", "uses"},
		{"empty steps array", "empty-steps.yaml", "steps"},
		{"invalid angle value", "invalid-angle.yaml", "angle"},
		{"malformed YAML", "malformed.yaml", "yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", "invalid", tc.file)
			_, err := LoadSensor(path)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.file)
			}
			// Substring match is intentionally loose — we want the
			// error to be informative, not exact. Each case names the
			// failing aspect (field name or "yaml") in lowercase.
			lower := strings.ToLower(err.Error())
			if !strings.Contains(lower, tc.wantSubstr) {
				t.Errorf("error did not mention %q; got: %v", tc.wantSubstr, err)
			}
		})
	}
}

// minimalScopeProbe is used only by TestLoadSensor_AllGoldenExamples to
// detect a sensor's scope before the full Sensor struct gains a Scope field
// (Task 6). Once Sensor.Scope is added, callers can use s.Scope instead.
type minimalScopeProbe struct {
	Scope string `json:"scope"`
}

func TestLoadSensor_AllGoldenExamples(t *testing.T) {
	dir := filepath.Join("..", "..", "schemas", "examples", "sensor")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}
	var loaded int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			s, err := LoadSensor(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("LoadSensor(%s): %v", name, err)
			}
			if s.ID == "" {
				t.Errorf("loaded sensor has empty ID")
			}
			// use_case_id is required for use-case-scoped sensors but must
			// be absent for core-scoped sensors. Probe the raw YAML for scope
			// so this check remains correct before Sensor.Scope is added
			// (Task 6 will let callers use s.Scope directly).
			raw, readErr := os.ReadFile(filepath.Join(dir, name))
			if readErr != nil {
				t.Fatalf("re-read %s: %v", name, readErr)
			}
			asJSON, convErr := yaml.YAMLToJSON(raw)
			if convErr != nil {
				t.Fatalf("yaml->json %s: %v", name, convErr)
			}
			var probe minimalScopeProbe
			if jsonErr := json.Unmarshal(asJSON, &probe); jsonErr != nil {
				t.Fatalf("probe scope %s: %v", name, jsonErr)
			}
			isCore := probe.Scope == "core"
			if !isCore && s.UseCaseID == "" {
				t.Errorf("use-case-scoped sensor has empty UseCaseID")
			}
			if isCore && s.UseCaseID != "" {
				t.Errorf("core-scoped sensor must not have UseCaseID, got %q", s.UseCaseID)
			}
			if len(s.Steps) == 0 {
				t.Errorf("loaded sensor has no steps")
			}
		})
		loaded++
	}
	if loaded < 6 {
		t.Errorf("expected at least 6 sensor example files, found %d — schemas/examples/sensor/ may have shrunk", loaded)
	}
}
