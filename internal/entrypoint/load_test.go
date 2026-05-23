package entrypoint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestLoadEntryPoint_HTTPAPIExample(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "examples", "entry-point", "http-api.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	ep, err := LoadEntryPoint(raw)
	if err != nil {
		t.Fatalf("LoadEntryPoint: %v", err)
	}
	if ep.ID != "create-order-endpoint" {
		t.Errorf("ID = %q, want create-order-endpoint", ep.ID)
	}
	if ep.Archetype != enums.ArchetypeHTTPAPI {
		t.Errorf("Archetype = %q, want %q", ep.Archetype, enums.ArchetypeHTTPAPI)
	}
	if got := ep.Spec["method"]; got != "POST" {
		t.Errorf("Spec[method] = %v, want POST", got)
	}
	if got := ep.Spec["path"]; got != "/orders" {
		t.Errorf("Spec[path] = %v, want /orders", got)
	}
}
