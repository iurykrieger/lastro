package sensor

import "testing"

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
