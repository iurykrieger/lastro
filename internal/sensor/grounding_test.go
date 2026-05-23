package sensor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/stack"
)

func TestValidateAgainstStack_AllRefsKnown(t *testing.T) {
	manifest := loadHTTPManifest(t)
	// The http-api example manifest contains components nestjs + postgres.
	// A sensor that uses only nestjs is fully grounded.
	s := Sensor{
		ID:        "ok-sensor",
		UseCaseID: "uc",
		Uses:      []string{"nestjs"},
		Steps:     []Step{{ID: "x", Run: "true"}},
	}
	if err := ValidateAgainstStack(s, manifest); err != nil {
		t.Errorf("ValidateAgainstStack: got %v, want nil", err)
	}
}

func TestValidateAgainstStack_TwoUnknownRefs_JoinedError(t *testing.T) {
	manifest := loadHTTPManifest(t)
	s := Sensor{
		ID:        "bad-sensor",
		UseCaseID: "uc",
		Uses:      []string{"nestjs", "ghost-lib", "phantom-runtime"},
		Steps:     []Step{{ID: "x", Run: "true"}},
	}
	err := ValidateAgainstStack(s, manifest)
	if err == nil {
		t.Fatal("expected error for unknown stack ids, got nil")
	}
	for _, want := range []string{"ghost-lib", "phantom-runtime"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error did not mention %q; got: %v", want, err)
		}
	}
	// nestjs is valid — should NOT appear as a problem.
	if strings.Contains(err.Error(), "nestjs") {
		t.Errorf("error wrongly accused nestjs; got: %v", err)
	}
}

func TestValidateAgainstStack_EmptyUses(t *testing.T) {
	manifest := loadHTTPManifest(t)
	s := Sensor{ID: "empty-uses-sensor", Uses: nil}
	if err := ValidateAgainstStack(s, manifest); err != nil {
		t.Errorf("empty Uses should ground trivially; got %v", err)
	}
}

func loadHTTPManifest(t *testing.T) stack.StackManifest {
	t.Helper()
	path := filepath.Join("..", "..", "schemas", "examples", "stack-manifest", "http-api.yaml")
	m, err := stack.Load(path)
	if err != nil {
		t.Fatalf("stack.Load(%s): %v", path, err)
	}
	return m
}
