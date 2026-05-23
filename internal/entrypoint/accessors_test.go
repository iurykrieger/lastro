package entrypoint

import (
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestSpecField_HitAndMiss(t *testing.T) {
	ep, err := LoadFromExample(filepath.Join("..", "..", "schemas", "examples", "entry-point", "http-api.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got, ok := ep.SpecField("method"); !ok || got != "POST" {
		t.Errorf("SpecField(method) = (%v, %v), want (POST, true)", got, ok)
	}
	if got, ok := ep.SpecField("nonexistent"); ok || got != nil {
		t.Errorf("SpecField(nonexistent) = (%v, %v), want (nil, false)", got, ok)
	}
}

func TestLabel_HappyPath(t *testing.T) {
	ep := EntryPoint{ID: "create-order-endpoint", Archetype: enums.ArchetypeHTTPAPI}
	if got, want := ep.Label(), "http-api:create-order-endpoint"; got != want {
		t.Errorf("Label = %q, want %q", got, want)
	}
}

func TestLabel_ZeroValue(t *testing.T) {
	if got := (EntryPoint{}).Label(); got != ":" {
		t.Errorf("zero-value Label = %q, want %q", got, ":")
	}
}

func TestValidate_LoadedFixturePasses(t *testing.T) {
	ep, err := LoadFromExample(filepath.Join("..", "..", "schemas", "examples", "entry-point", "cli.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := ep.Validate(); err != nil {
		t.Errorf("Validate on loaded EntryPoint: %v", err)
	}
}

func TestValidate_ConstructedInCodePasses(t *testing.T) {
	ep := EntryPoint{
		ID:        "test-cli",
		Archetype: enums.ArchetypeCLI,
		Spec:      map[string]any{"command": "harness"},
	}
	if err := ep.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidate_RejectsUnknownMethod(t *testing.T) {
	// PROPFIND is a WebDAV verb, not in the closed http-methods enum.
	bad := EntryPoint{
		ID:        "broken",
		Archetype: enums.ArchetypeHTTPAPI,
		Spec:      map[string]any{"method": "PROPFIND", "path": "/x"},
	}
	if err := bad.Validate(); err == nil {
		t.Errorf("Validate expected error for PROPFIND, got nil")
	}
}
