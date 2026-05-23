package entrypoint

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedSchemaMatchesCanonicalSource(t *testing.T) {
	canonicalPath := filepath.Join("..", "..", "schemas", "entry-point.yaml")
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical schema %s: %v", canonicalPath, err)
	}
	if !bytes.Equal(canonical, embeddedSchemaYAML) {
		t.Errorf("internal/entrypoint/schema.yaml has drifted from schemas/entry-point.yaml; re-run `cp schemas/entry-point.yaml internal/entrypoint/schema.yaml`")
	}
}
