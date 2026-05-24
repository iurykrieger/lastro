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

// fakeOwner is the test-local UseCaseFixtureOwnership: a one-line
// map keyed by use case id. Production wiring lives elsewhere (an
// adapter over *usecase.UseCase.FixtureIDs).
type fakeOwner map[string][]string

func (f fakeOwner) OwnedFixtureIDs(useCaseID string) []string {
	return f[useCaseID]
}

func TestValidateAgainstFixtures_AllRefsOwned(t *testing.T) {
	owner := fakeOwner{
		"uc-1": []string{"order-input-fixture", "order-output-fixture"},
	}
	s := Sensor{
		ID:        "ok-sensor",
		UseCaseID: "uc-1",
		Steps: []Step{
			{ID: "send", Run: "x", Uses: []string{"order-input-fixture"}},
			{ID: "check", Run: "y", Uses: []string{"order-output-fixture"}},
		},
	}
	if err := ValidateAgainstFixtures(s, owner); err != nil {
		t.Errorf("ValidateAgainstFixtures: got %v, want nil", err)
	}
}

func TestValidateAgainstFixtures_StepsWithoutUses_PassTrivially(t *testing.T) {
	owner := fakeOwner{"uc-1": []string{"x"}}
	s := Sensor{
		ID:        "build-sensor",
		UseCaseID: "uc-1",
		Steps:     []Step{{ID: "compile", Run: "tsc"}},
	}
	if err := ValidateAgainstFixtures(s, owner); err != nil {
		t.Errorf("step without Uses should pass trivially; got %v", err)
	}
}

func TestValidateAgainstFixtures_TwoStepsWithUnknownFixtures(t *testing.T) {
	owner := fakeOwner{"uc-1": []string{"known-fixture"}}
	s := Sensor{
		ID:        "bad-sensor",
		UseCaseID: "uc-1",
		Steps: []Step{
			{ID: "first", Uses: []string{"ghost-fixture"}},
			{ID: "second", Uses: []string{"known-fixture", "phantom-fixture"}},
		},
	}
	err := ValidateAgainstFixtures(s, owner)
	if err == nil {
		t.Fatal("expected error for unowned fixtures, got nil")
	}
	for _, want := range []string{"first", "ghost-fixture", "second", "phantom-fixture"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error did not mention %q; got: %v", want, err)
		}
	}
	// known-fixture is valid — must NOT appear as a problem.
	if strings.Contains(err.Error(), "unknown fixture \"known-fixture\"") {
		t.Errorf("error wrongly accused known-fixture; got: %v", err)
	}
}

func TestValidateAgainstFixtures_UnknownUseCase_FailsEveryStepWithUses(t *testing.T) {
	owner := fakeOwner{} // owner returns nil for any use case id
	s := Sensor{
		ID:        "orphan-sensor",
		UseCaseID: "nonexistent-uc",
		Steps: []Step{
			{ID: "send", Uses: []string{"x-fixture"}},
		},
	}
	err := ValidateAgainstFixtures(s, owner)
	if err == nil {
		t.Fatal("expected error when owner returns nil owned-set, got nil")
	}
	if !strings.Contains(err.Error(), "x-fixture") {
		t.Errorf("error did not mention x-fixture; got: %v", err)
	}
}
