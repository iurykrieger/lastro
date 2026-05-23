package sensor

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
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
