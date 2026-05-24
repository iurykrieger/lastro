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
	_ = errors.New         // keep errors import valid until Task 3.4 adds errors.As
	_ = persisterror.Error{} // keep persisterror import valid until Task 3.4
}
