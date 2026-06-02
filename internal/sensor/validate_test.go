package sensor

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestLoadSensor_DuplicateStepID(t *testing.T) {
	path := filepath.Join("testdata", "invalid", "duplicate-step-id.yaml")
	_, err := LoadSensor(path)
	if err == nil {
		t.Fatal("expected error for duplicate step ids, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate step id") {
		t.Errorf("error did not mention duplicate step id; got: %v", err)
	}
	if !strings.Contains(err.Error(), "probe") {
		t.Errorf("error did not name the duplicated id 'probe'; got: %v", err)
	}
}

func TestValidateIntrinsic_OnTypedSensorOK(t *testing.T) {
	s := Sensor{
		ID:        "foo",
		UseCaseID: "uc",
		Uses:      []string{"node"},
		Steps:     []Step{{ID: "s1", Run: "echo"}, {ID: "s2", Run: "echo"}},
	}
	if err := validateIntrinsic(s); err != nil {
		t.Errorf("validateIntrinsic on clean sensor: %v", err)
	}
}

func TestLoadSensor_DuplicateTopLevelUses(t *testing.T) {
	path := filepath.Join("testdata", "invalid", "duplicate-uses.yaml")
	_, err := LoadSensor(path)
	if err == nil {
		t.Fatal("expected error for duplicate top-level uses, got nil")
	}
	if !errorsJoinContains(err, "duplicate uses id") {
		t.Errorf("error did not mention duplicate uses id; got: %v", err)
	}
	if !errorsJoinContains(err, "node") {
		t.Errorf("error did not name the duplicated id 'node'; got: %v", err)
	}
}

// TestLoadSensor_DuplicateStepUses is removed: step-level uses is now a
// scalar primitive id, so "duplicate step uses" is not a representable state.
// Task 8 introduces checkStepShape which validates run-xor-uses exclusivity.

func TestLoadSensor_SelfDependency(t *testing.T) {
	path := filepath.Join("testdata", "invalid", "self-dep.yaml")
	_, err := LoadSensor(path)
	if err == nil {
		t.Fatal("expected error for self-dependency, got nil")
	}
	if !errorsJoinContains(err, "self-dependency") {
		t.Errorf("error did not mention self-dependency; got: %v", err)
	}
}

func TestLoadSensor_MultiViolation(t *testing.T) {
	// File contains duplicate step ids AND self-dependency AND
	// duplicate top-level uses — three rules fire, joined error
	// must mention all three.
	path := filepath.Join("testdata", "invalid", "multi-violation.yaml")
	_, err := LoadSensor(path)
	if err == nil {
		t.Fatal("expected error for multi-violation file, got nil")
	}
	for _, want := range []string{"duplicate step id", "duplicate uses id", "self-dependency"} {
		if !errorsJoinContains(err, want) {
			t.Errorf("joined error missing %q; got: %v", want, err)
		}
	}
}

func TestValidateIntrinsic_CoreScopeForbidsUseCaseID(t *testing.T) {
	s := Sensor{
		ID: "run-dev", Scope: enums.ScopeCore, UseCaseID: "order-create",
		Angle: enums.AngleEnvironment, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{ID: "boot", Run: "make dev"}},
	}
	err := validateIntrinsic(s)
	if err == nil || !strings.Contains(err.Error(), "core") {
		t.Fatalf("expected core-scope use_case_id error, got %v", err)
	}
}

func TestValidateIntrinsic_UseCaseScopeRequiresUseCaseID(t *testing.T) {
	s := Sensor{
		ID: "x-build", Scope: enums.ScopeUseCase, UseCaseID: "",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{ID: "c", Run: "go build ./..."}},
	}
	err := validateIntrinsic(s)
	if err == nil || !strings.Contains(err.Error(), "use_case_id") {
		t.Fatalf("expected missing use_case_id error, got %v", err)
	}
}

func TestValidateStepShapeRunXorUses(t *testing.T) {
	bad := Sensor{ID: "x", Steps: []Step{{ID: "s"}}} // neither run nor uses
	if err := validateIntrinsic(bad); err == nil {
		t.Fatal("expected error for step with neither run nor uses")
	}
	both := Sensor{ID: "x", Steps: []Step{{ID: "s", Run: "echo", Uses: "p"}}}
	if err := validateIntrinsic(both); err == nil {
		t.Fatal("expected error for step with both run and uses")
	}
}

func TestValidateWithOnlyOnUsesStep(t *testing.T) {
	bad := Sensor{ID: "x", Steps: []Step{{ID: "s", Run: "echo", With: map[string]string{"a": "b"}}}}
	if err := validateIntrinsic(bad); err == nil {
		t.Fatal("expected error: with on a run-step")
	}
}

func TestCheckStepShapeDirect(t *testing.T) {
	if checkStepShape(Sensor{Steps: []Step{{ID: "ok", Run: "echo"}}}) != nil {
		t.Error("valid run-step should pass")
	}
	if checkStepShape(Sensor{Steps: []Step{{ID: "ok", Uses: "p"}}}) != nil {
		t.Error("valid uses-step should pass")
	}
	if checkStepShape(Sensor{Steps: []Step{{ID: "u", Uses: "p", With: map[string]string{"k": "v"}}}}) != nil {
		t.Error("uses-step with `with` should pass")
	}
	if checkStepShape(Sensor{Steps: []Step{{ID: "bad"}}}) == nil {
		t.Error("neither run nor uses should fail")
	}
	if checkStepShape(Sensor{Steps: []Step{{ID: "bad", Run: "e", Uses: "p"}}}) == nil {
		t.Error("both run and uses should fail")
	}
	if checkStepShape(Sensor{Steps: []Step{{ID: "bad", Run: "e", With: map[string]string{"k": "v"}}}}) == nil {
		t.Error("with on run-step should fail")
	}
}

// errorsJoinContains walks errors.Join trees, returning true if any
// wrapped error's message contains substr. Used so tests can assert
// on a single rule's message inside a joined multi-rule error.
func errorsJoinContains(err error, substr string) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), substr) {
		return true
	}
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range u.Unwrap() {
			if errorsJoinContains(e, substr) {
				return true
			}
		}
	}
	if u := errors.Unwrap(err); u != nil {
		return errorsJoinContains(u, substr)
	}
	return false
}
