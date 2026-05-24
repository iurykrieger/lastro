package signal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedSchemaMatchesCanonicalSource(t *testing.T) {
	canonicalPath := filepath.Join("..", "..", "schemas", "signal.yaml")
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical schema %s: %v", canonicalPath, err)
	}
	if !bytes.Equal(canonical, embeddedSchemaYAML) {
		t.Errorf("internal/signal/schema.yaml has drifted from schemas/signal.yaml; re-run `cp schemas/signal.yaml internal/signal/schema.yaml`")
	}
}

func TestCompiledSchemaIsAvailable(t *testing.T) {
	s, err := compiledSchema()
	if err != nil {
		t.Fatalf("compiledSchema: %v", err)
	}
	if s == nil {
		t.Fatal("compiledSchema: returned nil schema")
	}
}
