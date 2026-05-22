package fixture

import (
	"path/filepath"
	"testing"
)

func TestLoadFixtureInputExampleStructuralFields(t *testing.T) {
	p := filepath.Join("..", "..", "schemas", "examples", "fixture", "input.yaml")
	fx, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("LoadFixture(%s): %v", p, err)
	}
	if fx.ID != "order-input-fixture" {
		t.Errorf("ID: got %q, want order-input-fixture", fx.ID)
	}
	if fx.UseCaseID != "create-order-use-case" {
		t.Errorf("UseCaseID: got %q, want create-order-use-case", fx.UseCaseID)
	}
	if fx.Role != RoleInput {
		t.Errorf("Role: got %q, want %q", fx.Role, RoleInput)
	}
	if fx.ContentType != "application/json" {
		t.Errorf("ContentType: got %q, want application/json", fx.ContentType)
	}
	if len(fx.Payload) == 0 {
		t.Errorf("Payload: empty; want non-empty bytes")
	}
	if fx.Binding == nil {
		t.Fatal("Binding: nil; want non-nil for input example")
	}
	if fx.Binding.Channel != ChannelHTTP {
		t.Errorf("Binding.Channel: got %q, want http", fx.Binding.Channel)
	}
	if fx.Binding.Selector["method"] != "POST" {
		t.Errorf("Binding.Selector[method]: got %v, want POST", fx.Binding.Selector["method"])
	}
}

func TestLoadFixtureInputExampleParsesJSONPayload(t *testing.T) {
	p := filepath.Join("..", "..", "schemas", "examples", "fixture", "input.yaml")
	fx, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("LoadFixture: %v", err)
	}
	if fx.Parsed == nil {
		t.Fatal("Parsed: nil; want non-nil for application/json payload")
	}
	m, ok := fx.Parsed.(map[string]any)
	if !ok {
		t.Fatalf("Parsed: got %T, want map[string]any", fx.Parsed)
	}
	if m["customer_id"] != "c-001" {
		t.Errorf("Parsed.customer_id: got %v, want c-001", m["customer_id"])
	}
}

func TestLoadFixtureExpectedOutputExample(t *testing.T) {
	p := filepath.Join("..", "..", "schemas", "examples", "fixture", "expected-output.yaml")
	fx, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("LoadFixture: %v", err)
	}
	if fx.Role != RoleExpectedOutput {
		t.Errorf("Role: got %q, want %q", fx.Role, RoleExpectedOutput)
	}
	if fx.Parsed == nil {
		t.Error("Parsed: nil for application/json fixture; want non-nil")
	}
}

func TestLoadFixtureExpectedSideEffectExampleHasRawTextPayloadOnly(t *testing.T) {
	p := filepath.Join("..", "..", "schemas", "examples", "fixture", "expected-side-effect.yaml")
	fx, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("LoadFixture: %v", err)
	}
	if fx.Role != RoleExpectedSideEffect {
		t.Errorf("Role: got %q, want %q", fx.Role, RoleExpectedSideEffect)
	}
	if fx.ContentType != "text/plain" {
		t.Errorf("ContentType: got %q, want text/plain", fx.ContentType)
	}
	if fx.Parsed != nil {
		t.Errorf("Parsed: got %v, want nil for text/plain", fx.Parsed)
	}
	if len(fx.Payload) == 0 {
		t.Error("Payload: empty; want raw bytes preserved")
	}
}

func TestLoadFixtureYAMLContentType(t *testing.T) {
	p := filepath.Join("testdata", "yaml-content-type.yaml")
	fx, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("LoadFixture: %v", err)
	}
	m, ok := fx.Parsed.(map[string]any)
	if !ok {
		t.Fatalf("Parsed: got %T, want map[string]any", fx.Parsed)
	}
	if m["customer_id"] != "c-001" {
		t.Errorf("Parsed.customer_id: got %v, want c-001", m["customer_id"])
	}
}

func TestLoadFixtureXMLContentType(t *testing.T) {
	p := filepath.Join("testdata", "xml-content-type.yaml")
	fx, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("LoadFixture: %v", err)
	}
	if _, ok := fx.Parsed.(map[string]any); !ok {
		t.Fatalf("Parsed: got %T, want map[string]any", fx.Parsed)
	}
}

func TestLoadDirectoryHappyPathExamples(t *testing.T) {
	p := filepath.Join("..", "..", "schemas", "examples", "fixture")
	store, err := LoadDirectory(p)
	if err != nil {
		t.Fatalf("LoadDirectory(%s): %v", p, err)
	}
	all := store.All()
	if len(all) != 3 {
		t.Fatalf("All: got %d fixtures, want 3", len(all))
	}
	for _, fx := range all {
		if fx.UseCaseID != "create-order-use-case" {
			t.Errorf("fixture %q: UseCaseID %q, want create-order-use-case", fx.ID, fx.UseCaseID)
		}
	}
	roles := map[Role]bool{}
	for _, fx := range all {
		roles[fx.Role] = true
	}
	for _, r := range []Role{RoleInput, RoleExpectedOutput, RoleExpectedSideEffect} {
		if !roles[r] {
			t.Errorf("expected role %q not present in loaded fixtures", r)
		}
	}
}

func TestLoadFixtureWithoutBinding(t *testing.T) {
	p := filepath.Join("testdata", "no-binding.yaml")
	fx, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("LoadFixture: %v", err)
	}
	if fx.Binding != nil {
		t.Errorf("Binding: got %+v, want nil", fx.Binding)
	}
	if fx.ID != "no-binding-fixture" {
		t.Errorf("ID: got %q, want no-binding-fixture", fx.ID)
	}
}
