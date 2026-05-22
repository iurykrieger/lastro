package fixture

import (
	"path/filepath"
	"reflect"
	"testing"
)

// One fixture is loaded once, then resolved from two distinct
// "consumer" call sites. Both observe the same parsed tree — proving
// eager parse happens exactly once at load time and the parsed value
// is shared across consumers (the acceptance criterion from
// docs/harness-framework/E5-fixture.md §"Deliverable acceptance").
func TestFixtureReuseAcrossConsumers(t *testing.T) {
	p := filepath.Join("..", "..", "schemas", "examples", "fixture", "input.yaml")
	fx, err := LoadFixture(p)
	if err != nil {
		t.Fatalf("LoadFixture: %v", err)
	}
	store, err := NewStore(fx)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Consumer A: e2e-test sensor needs the raw payload to send over HTTP.
	a, ok := store.LookupFixture(fx.ID)
	if !ok {
		t.Fatalf("consumer A: LookupFixture: not found")
	}

	// Consumer B: unit-test sensor needs the parsed tree to drive a function call.
	b, ok := store.LookupFixture(fx.ID)
	if !ok {
		t.Fatalf("consumer B: LookupFixture: not found")
	}

	if !reflect.DeepEqual(a.Parsed, b.Parsed) {
		t.Errorf("Parsed differs between consumers; eager parse should be shared")
	}

	// Both consumers should observe the same underlying map (pointer-equal
	// when the parsed value is a map — Go maps are reference types).
	aMap, aOK := a.Parsed.(map[string]any)
	bMap, bOK := b.Parsed.(map[string]any)
	if !aOK || !bOK {
		t.Fatalf("Parsed: A=%T, B=%T; both must be map[string]any", a.Parsed, b.Parsed)
	}
	// Mutating via one consumer is observable by the other — proof of shared map.
	aMap["__test_marker__"] = "shared"
	if bMap["__test_marker__"] != "shared" {
		t.Errorf("Parsed maps are not shared between consumers")
	}
	delete(aMap, "__test_marker__")
}
