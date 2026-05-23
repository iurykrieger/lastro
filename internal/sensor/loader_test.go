package sensor

import (
	"path/filepath"
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
