package sensor

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestSensorZeroValueIsUsable(t *testing.T) {
	var s Sensor
	if s.ID != "" || s.UseCaseID != "" || s.Angle != "" || len(s.Uses) != 0 || len(s.Steps) != 0 {
		t.Errorf("zero-value Sensor should have empty fields; got %+v", s)
	}
}

func TestSensorHoldsTypedEnumValues(t *testing.T) {
	s := Sensor{
		SchemaVersion: "1.0.0",
		ID:            "build-create-order-sensor",
		UseCaseID:     "create-order-use-case",
		Angle:         enums.AngleBuild,
		Kind:          enums.KindAssertion,
		Nature:        enums.NatureComputational,
		OutputType:    enums.OutputSingleShot,
		Uses:          []string{"node"},
	}
	if s.Angle != enums.AngleBuild {
		t.Errorf("Angle: got %q, want %q", s.Angle, enums.AngleBuild)
	}
	if s.Kind != enums.KindAssertion {
		t.Errorf("Kind: got %q, want %q", s.Kind, enums.KindAssertion)
	}
}

func TestStepHoldsIDRunAndUses(t *testing.T) {
	step := Step{
		ID:   "probe",
		Run:  "curl -sS http://localhost:3000/orders",
		Uses: []string{"order-input-fixture"},
	}
	if step.ID != "probe" || step.Run == "" || len(step.Uses) != 1 {
		t.Errorf("Step field round-trip failed: %+v", step)
	}
}

// Compile-time interface assertion — proves the seam exists and is
// stable. Implementing this fake forces breaking changes to the
// interface to surface as build errors here.
var _ UseCaseFixtureOwnership = (*compileTimeStubOwnership)(nil)

type compileTimeStubOwnership struct{}

func (*compileTimeStubOwnership) OwnedFixtureIDs(useCaseID string) []string {
	return nil
}

func TestOwnedFixtureIDsContractAllowsNilReturn(t *testing.T) {
	var owner UseCaseFixtureOwnership = &compileTimeStubOwnership{}
	got := owner.OwnedFixtureIDs("anything")
	if got != nil {
		t.Errorf("stub: expected nil, got %v", got)
	}
}
