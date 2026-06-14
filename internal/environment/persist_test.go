// internal/environment/persist_test.go
package environment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

const validModelYAML = `schema_version: 1.0.0
application:
  provided_by: {file: package.json, path: scripts.dev}
  depends_on: [postgres, migrate]
dependencies:
  postgres:
    type: datastore
    provided_by: {file: docker-compose.yml, path: services.postgres}
setup:
  - id: migrate
    type: setup
    provided_by: {file: package.json, path: scripts.db:migrate}
    depends_on: [postgres]
`

const factsYAML = `scripts:
  dev: next dev
  db:migrate: drizzle-kit migrate
compose_services:
  postgres:
    image: postgres:16-alpine
compose_file: docker-compose.yml
`

func TestPersist_OK(t *testing.T) {
	dir := t.TempDir()
	if err := Persist([]byte(validModelYAML), []byte(factsYAML), dir); err != nil {
		t.Fatalf("persist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "environment-model.yaml")); err != nil {
		t.Fatalf("model not written: %v", err)
	}
}

func TestPersist_UngroundedProvidedBy(t *testing.T) {
	dir := t.TempDir()
	bad := `schema_version: 1.0.0
application:
  provided_by: {file: package.json, path: scripts.ghost}
`
	err := Persist([]byte(bad), []byte(factsYAML), dir)
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("want *persisterror.Error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "environment-model.yaml")); statErr == nil {
		t.Fatal("ungrounded model must NOT be written")
	}
}
