package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func loadExample(t *testing.T, name string) *ValidationPolicy {
	t.Helper()
	path := filepath.Join("..", "..", "schemas", "examples", "validation-policy", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	p, err := Load(f)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	return p
}

func TestLoad_GlobalExample(t *testing.T) {
	p := loadExample(t, "global.yaml")
	if p.SchemaVersion != "1.0.0" {
		t.Errorf("SchemaVersion = %q, want 1.0.0", p.SchemaVersion)
	}
	if p.Scope != ScopeGlobal {
		t.Errorf("Scope = %q, want global", p.Scope)
	}
	block, ok := p.PerArchetype[enums.ArchetypeHTTPAPI]
	if !ok {
		t.Fatal("PerArchetype[http-api] missing")
	}
	if len(block.Obligatory) != 5 {
		t.Errorf("http-api obligatory count = %d, want 5", len(block.Obligatory))
	}
}

func TestLoad_LocalExample(t *testing.T) {
	p := loadExample(t, "local.yaml")
	if p.Scope != ScopeLocal {
		t.Errorf("Scope = %q, want local", p.Scope)
	}
}
