package stack

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/persisterror"
)

func TestPersist_NewFile_WritesEnrichedManifest(t *testing.T) {
	dir := t.TempDir()
	in := []byte(`schema_version: 1.0.0
archetype: http-api
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence:
      - file: package.json
        path: .dependencies.express
`)
	if err := Persist(in, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "stack-manifest.yaml"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	var m StackManifest
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal written: %v", err)
	}
	if m.Archetype != enums.ArchetypeHTTPAPI {
		t.Fatalf("Archetype=%q", m.Archetype)
	}
	want := enums.ApplicableAngles[enums.ArchetypeHTTPAPI]
	if !angleSetEqual(m.ApplicableAngles, want) {
		t.Fatalf("ApplicableAngles=%v, want %v", m.ApplicableAngles, want)
	}
	if m.SchemaVersion != "1.0.0" {
		t.Fatalf("SchemaVersion=%q, want 1.0.0 (no prior file to bump from)", m.SchemaVersion)
	}
}

func TestPersist_ExistingFile_BumpsSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	prior := []byte(`schema_version: 1.0.7
archetype: http-api
applicable_angles: [security, build, code-structure, unit-test, e2e-test, contracts, logs, metrics, database, performance]
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence:
      - file: package.json
        path: .dependencies.express
`)
	if err := os.WriteFile(filepath.Join(dir, "stack-manifest.yaml"), prior, 0o644); err != nil {
		t.Fatal(err)
	}
	in := []byte(`schema_version: 1.0.0
archetype: http-api
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence:
      - file: package.json
        path: .dependencies.express
`)
	if err := Persist(in, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "stack-manifest.yaml"))
	var m StackManifest
	_ = yaml.Unmarshal(out, &m)
	if m.SchemaVersion != "1.0.8" {
		t.Fatalf("SchemaVersion=%q, want 1.0.8 (prior 1.0.7 + 1)", m.SchemaVersion)
	}
}

func TestPersist_RejectsBadArchetype(t *testing.T) {
	dir := t.TempDir()
	in := []byte(`schema_version: 1.0.0
archetype: not-a-real-archetype
components:
  - schema_version: 1.0.0
    id: x
    kind: library
    name: x
    version: 1.0.0
    capabilities: [c]
    detection_evidence: [{file: f, path: p}]
`)
	err := Persist(in, dir)
	if err == nil {
		t.Fatal("Persist accepted unknown archetype")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("error is not *persisterror.Error: %T", err)
	}
	// The exact Kind depends on whether the schema or programmatic check
	// fires first; both are acceptable signals of a bad archetype.
	if pe.Kind != persisterror.SchemaViolation && pe.Kind != persisterror.UnknownEnumValue {
		t.Fatalf("Kind=%q, want SchemaViolation or UnknownEnumValue", pe.Kind)
	}
	if _, err := os.Stat(filepath.Join(dir, "stack-manifest.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".harness/ written to despite validation failure: stat=%v", err)
	}
}
