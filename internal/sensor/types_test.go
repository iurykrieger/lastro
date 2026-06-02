package sensor

import (
	"encoding/json"
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

func TestStepHoldsIDAndRun(t *testing.T) {
	step := Step{
		ID:  "probe",
		Run: "curl -sS http://localhost:3000/orders",
	}
	if step.ID != "probe" || step.Run == "" || step.Uses != "" {
		t.Errorf("Step field round-trip failed: %+v", step)
	}
}

func TestStepHoldsIDUsesAndWith(t *testing.T) {
	step := Step{
		ID:   "create",
		Uses: "e2e-test",
		With: map[string]string{"method": "POST"},
	}
	if step.ID != "create" || step.Uses != "e2e-test" || step.With["method"] != "POST" || step.Run != "" {
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

func TestStepUnmarshalUsesScalarAndWith(t *testing.T) {
	var s Step
	if err := json.Unmarshal([]byte(`{"id":"create","uses":"e2e-test","with":{"method":"POST"}}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Uses != "e2e-test" || s.With["method"] != "POST" || s.Run != "" {
		t.Errorf("got %#v", s)
	}
}

func TestSensorInputsOutputsParse(t *testing.T) {
	raw := []byte(`{"schema_version":"1.0.0","id":"e2e-test","scope":"core","angle":"e2e-test",` +
		`"kind":"assertion","nature":"computational","output_type":"single-shot","uses":[],` +
		`"inputs":{"method":{"required":true,"default":"GET"}},` +
		`"outputs":{"body":{"from":"${{ steps.request.outputs.body }}"}},` +
		`"steps":[{"id":"request","run":"echo hi"}]}`)
	s, err := LoadSensorBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Inputs["method"].Required || s.Inputs["method"].Default != "GET" {
		t.Errorf("inputs = %#v", s.Inputs)
	}
	if !s.Inputs["method"].HasDefault {
		t.Errorf("expected HasDefault true for method")
	}
	if s.Outputs["body"].From == "" {
		t.Errorf("outputs = %#v", s.Outputs)
	}
}
