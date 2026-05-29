package sensor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
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
			// be absent for core-scoped sensors.
			isCore := s.Scope == enums.ScopeCore
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

func TestLoadDirectory_FolderedAndFlat(t *testing.T) {
	dir := t.TempDir()
	// legacy flat use-case sensor
	writeSensor(t, filepath.Join(dir, "oc-build.yaml"), "oc-build", "use-case", "order-create", "build")
	// foldered use-case sensor
	if err := os.MkdirAll(filepath.Join(dir, "order-create"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSensor(t, filepath.Join(dir, "order-create", "oc-e2e.yaml"), "oc-e2e", "use-case", "order-create", "e2e-test")
	// foldered core sensor
	if err := os.MkdirAll(filepath.Join(dir, "core"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSensor(t, filepath.Join(dir, "core", "run-dev.yaml"), "run-dev", "core", "", "environment")

	st, err := LoadDirectory(dir)
	if err != nil {
		t.Fatalf("load dir: %v", err)
	}
	if len(st.All()) != 3 {
		t.Fatalf("want 3 sensors, got %d", len(st.All()))
	}
	if len(st.ForScope(enums.ScopeCore)) != 1 {
		t.Fatalf("want 1 core sensor")
	}
}

// writeSensor writes a minimal valid sensor YAML to path.
func writeSensor(t *testing.T, path, id, scope, useCase, angle string) {
	t.Helper()
	var ucLine string
	if useCase != "" {
		ucLine = "use_case_id: " + useCase + "\n"
	}
	body := "schema_version: 1.0.0\nid: " + id + "\nscope: " + scope + "\n" + ucLine +
		"angle: " + angle + "\nkind: assertion\nnature: computational\noutput_type: single-shot\nuses: []\nsteps:\n  - id: s\n    run: \"echo ok\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSensor_DefaultsScopeToUseCase(t *testing.T) {
	yaml := []byte(`schema_version: 1.0.0
id: order-create-build
use_case_id: order-create
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: compile
    run: "go build ./..."
`)
	s, err := LoadSensorBytes(yaml)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Scope != enums.ScopeUseCase {
		t.Errorf("scope: got %q, want %q (default)", s.Scope, enums.ScopeUseCase)
	}
}

func TestLoadSensor_CoreScopeNoUseCase(t *testing.T) {
	yaml := []byte(`schema_version: 1.0.0
id: run-dev
scope: core
angle: environment
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: boot
    run: "make dev"
`)
	s, err := LoadSensorBytes(yaml)
	if err != nil {
		t.Fatalf("load core: %v", err)
	}
	if s.Scope != enums.ScopeCore {
		t.Errorf("scope: got %q, want core", s.Scope)
	}
	if s.UseCaseID != "" {
		t.Errorf("use_case_id: got %q, want empty for core", s.UseCaseID)
	}
}
