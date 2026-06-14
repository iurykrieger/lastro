// internal/environment/dogfood_test.go
package environment

import (
	"path/filepath"
	"testing"
)

func TestDogfood_NoComposeInHarnessRepo(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := Parse(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.ComposeServices) != 0 {
		t.Fatalf("harness repo should have no compose services, got %v", facts.ComposeServices)
	}
	// A model with only an application node + no deps validates (no-op).
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"Makefile", "build"}},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("no-op model invalid: %v", err)
	}
}
