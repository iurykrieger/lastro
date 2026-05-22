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
