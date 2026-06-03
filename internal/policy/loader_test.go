package policy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func loadExample(t *testing.T, name string) *ValidationPolicy {
	t.Helper()
	path := filepath.Join("..", "..", "schemas", "examples", "validation-policy", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	p, err := Load(f)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	return p
}

func TestLoad_GlobalExample(t *testing.T) {
	p := loadExample(t, "global.yaml")
	if p.SchemaVersion != "1.0.0" {
		t.Errorf("SchemaVersion = %q, want 1.0.0", p.SchemaVersion)
	}
	if p.Scope != ScopeGlobal {
		t.Errorf("Scope = %q, want global", p.Scope)
	}
	block, ok := p.PerArchetype[enums.ArchetypeHTTPAPI]
	if !ok {
		t.Fatal("PerArchetype[http-api] missing")
	}
	if len(block.Obligatory) != 5 {
		t.Errorf("http-api obligatory count = %d, want 5", len(block.Obligatory))
	}
}

func TestLoad_LocalExample(t *testing.T) {
	p := loadExample(t, "local.yaml")
	if p.Scope != ScopeLocal {
		t.Errorf("Scope = %q, want local", p.Scope)
	}
}

func loadTestdata(t *testing.T, name string) error {
	t.Helper()
	_, err := loadValid(t, name)
	return err
}

func TestLoad_RejectsInapplicableAngle(t *testing.T) {
	err := loadTestdata(t, "inapplicable-angle.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := strings.ToLower(err.Error())
	for _, want := range []string{"library", "e2e-test", "not applicable"} {
		if !strings.Contains(msg, strings.ToLower(want)) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestLoad_RejectsOverlappingLists(t *testing.T) {
	err := loadTestdata(t, "overlapping-lists.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := strings.ToLower(err.Error())
	for _, want := range []string{"http-api", "logs", "overlap"} {
		if !strings.Contains(msg, strings.ToLower(want)) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestLoad_RejectsDuplicateInList(t *testing.T) {
	err := loadTestdata(t, "duplicate-in-list.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := strings.ToLower(err.Error())
	for _, want := range []string{"http-api", "build", "duplicate"} {
		if !strings.Contains(msg, strings.ToLower(want)) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func loadValid(t *testing.T, name string) (*ValidationPolicy, error) {
	t.Helper()
	path := filepath.Join("testdata", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	return Load(f)
}

func TestLoad_AcceptsExplicitFloor(t *testing.T) {
	p, err := loadValid(t, "floor-explicit.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.InferentialFloor == nil {
		t.Fatal("InferentialFloor should be non-nil when YAML sets it")
	}
	if *p.InferentialFloor != 0.85 {
		t.Errorf("*InferentialFloor = %v, want 0.85", *p.InferentialFloor)
	}
}

func TestLoad_OmittedFloorRemainsNil(t *testing.T) {
	p := loadExample(t, "global.yaml")
	if p.InferentialFloor != nil {
		t.Errorf("global.yaml has no inferential_floor; InferentialFloor = %v, want nil", *p.InferentialFloor)
	}
}

func TestLoad_RejectsOutOfRangeFloor(t *testing.T) {
	err := loadTestdata(t, "floor-out-of-range.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := strings.ToLower(err.Error())
	for _, want := range []string{"inferential_floor", "maximum"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestLoad_RejectsOutOfRangeMaxHealIterations(t *testing.T) {
	err := loadTestdata(t, "max-heal-iterations-out-of-range.yaml")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrMaxHealIterationsOutOfRange) {
		t.Errorf("error = %v, want errors.Is(_, ErrMaxHealIterationsOutOfRange)", err)
	}
}

func TestLoad_AcceptsValidMaxHealIterations(t *testing.T) {
	p, err := loadValid(t, "max-heal-iterations-valid.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.MaxHealIterations == nil || *p.MaxHealIterations != 5 {
		t.Errorf("MaxHealIterations = %v, want pointer to 5", p.MaxHealIterations)
	}
}

func TestLoad_RejectsSchemaViolations(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		wantSub string
	}{
		{"missing schema_version", "missing-schema-version.yaml", "schema_version"},
		{"unsupported schema_version", "unsupported-schema-version.yaml", "schema_version"},
		{"unknown scope", "unknown-scope.yaml", "scope"},
		{"unknown archetype", "unknown-archetype.yaml", "frobnicator"},
		{"unknown angle", "unknown-angle.yaml", "obligatory_angles"},
		{"unknown top-level field", "unknown-top-field.yaml", "inherits_from"},
		{"unknown block field", "unknown-block-field.yaml", "obligatorY_angles"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := loadTestdata(t, tc.file)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantSub)) {
				t.Errorf("error %q missing %q", err.Error(), tc.wantSub)
			}
		})
	}
}
