package sensor

import (
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestValidateCompositionUsesTargetMustBeCore(t *testing.T) {
	prim := Sensor{ID: "e2e-test", Scope: "core", Inputs: map[string]InputSpec{"method": {Required: true, Default: "GET", HasDefault: true}}}
	notCore := Sensor{ID: "decoy", Scope: "use-case", UseCaseID: "uc-x"}
	consumer := Sensor{ID: "c", Scope: "use-case", UseCaseID: "uc-x",
		Steps: []Step{{ID: "s", Uses: "decoy"}}}
	store, _ := NewStore(prim, notCore, consumer)
	if err := ValidateComposition(consumer, store); err == nil {
		t.Fatal("expected error: uses target is not scope=core")
	}
}

func TestValidateCompositionRequiredInputUnbound(t *testing.T) {
	prim := Sensor{ID: "e2e-test", Scope: "core",
		Inputs: map[string]InputSpec{"path": {Required: true}}} // required, no default
	consumer := Sensor{ID: "c", Scope: "use-case", UseCaseID: "uc-x",
		Steps: []Step{{ID: "s", Uses: "e2e-test", With: map[string]string{}}}}
	store, _ := NewStore(prim, consumer)
	if err := ValidateComposition(consumer, store); err == nil {
		t.Fatal("expected input-unbound error")
	}
}

func TestValidateCompositionRequiredInputDefaulted(t *testing.T) {
	prim := Sensor{ID: "e2e-test", Scope: "core",
		Inputs: map[string]InputSpec{"path": {Required: true, Default: "/", HasDefault: true}}}
	consumer := Sensor{ID: "c", Scope: "use-case", UseCaseID: "uc-x",
		Steps: []Step{{ID: "s", Uses: "e2e-test"}}}
	store, _ := NewStore(prim, consumer)
	if err := ValidateComposition(consumer, store); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateCompositionRequiredInputBound(t *testing.T) {
	prim := Sensor{ID: "e2e-test", Scope: "core",
		Inputs: map[string]InputSpec{"path": {Required: true}}}
	consumer := Sensor{ID: "c", Scope: "use-case", UseCaseID: "uc-x",
		Steps: []Step{{ID: "s", Uses: "e2e-test", With: map[string]string{"path": "/v1/charges"}}}}
	store, _ := NewStore(prim, consumer)
	if err := ValidateComposition(consumer, store); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateCompositionUnknownTarget(t *testing.T) {
	consumer := Sensor{ID: "c", Scope: "use-case", UseCaseID: "uc-x",
		Steps: []Step{{ID: "s", Uses: "ghost"}}}
	store, _ := NewStore(consumer)
	if err := ValidateComposition(consumer, store); err == nil {
		t.Fatal("expected error: unknown uses target")
	}
}

func TestValidateCompositionIgnoresRunSteps(t *testing.T) {
	consumer := Sensor{ID: "c", Scope: "use-case", UseCaseID: "uc-x",
		Steps: []Step{{ID: "s", Run: "echo hi"}}}
	store, _ := NewStore(consumer)
	if err := ValidateComposition(consumer, store); err != nil {
		t.Fatalf("run-only sensor should pass: %v", err)
	}
}

func TestValidateWithKeys_UndeclaredKeyFails(t *testing.T) {
	prim := Sensor{
		SchemaVersion: "1.0.0", ID: "e2e-test", Scope: enums.ScopeCore,
		Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Inputs: map[string]InputSpec{"method": {HasDefault: true}},
		Steps:  []Step{{ID: "request", Run: `echo "${{ inputs.method }}"`}},
	}
	consumer := Sensor{
		SchemaVersion: "1.0.0", ID: "s-uc-e2e", Scope: enums.ScopeUseCase,
		UseCaseID: "uc", Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{
			ID: "go", Uses: "e2e-test",
			With: map[string]string{"method": "POST", "idempotency_key": "abc"},
		}},
	}
	store, err := NewStore(prim, consumer)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	werr := ValidateWithKeys(consumer, store)
	if werr == nil {
		t.Fatal("undeclared with key must fail")
	}
	if !strings.Contains(werr.Error(), "idempotency_key") || !strings.Contains(werr.Error(), "e2e-test") {
		t.Fatalf("error should name the key and the primitive, got: %v", werr)
	}
}

func TestValidateWithKeys_DeclaredKeysPass(t *testing.T) {
	prim := Sensor{
		SchemaVersion: "1.0.0", ID: "e2e-test", Scope: enums.ScopeCore,
		Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Inputs: map[string]InputSpec{"method": {HasDefault: true}, "path": {HasDefault: true}},
		Steps:  []Step{{ID: "request", Run: `echo "${{ inputs.method }} ${{ inputs.path }}"`}},
	}
	consumer := Sensor{
		SchemaVersion: "1.0.0", ID: "s-uc-e2e", Scope: enums.ScopeUseCase,
		UseCaseID: "uc", Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{
			ID: "go", Uses: "e2e-test",
			With: map[string]string{"method": "POST", "path": "/v1/x"},
		}},
	}
	store, err := NewStore(prim, consumer)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if werr := ValidateWithKeys(consumer, store); werr != nil {
		t.Fatalf("declared keys must pass: %v", werr)
	}
}

func TestValidateWithKeys_UnknownTargetSkipped(t *testing.T) {
	// An unknown uses-target is ValidateComposition's finding, not this check's.
	consumer := Sensor{
		SchemaVersion: "1.0.0", ID: "s-uc-e2e", Scope: enums.ScopeUseCase,
		UseCaseID: "uc", Angle: enums.AngleE2ETest, Kind: enums.KindAssertion,
		Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Steps: []Step{{ID: "go", Uses: "ghost", With: map[string]string{"x": "y"}}},
	}
	store, err := NewStore(consumer)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if werr := ValidateWithKeys(consumer, store); werr != nil {
		t.Fatalf("unknown target must be skipped here: %v", werr)
	}
}
