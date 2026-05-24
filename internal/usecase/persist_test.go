package usecase

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/persisterror"
)

func TestPersist_TemplateResolution_RejectsUnresolvableToken(t *testing.T) {
	dir := t.TempDir()
	fxDir := filepath.Join(dir, "fixtures")
	if err := os.MkdirAll(fxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fxYAMLContent := []byte(`schema_version: 1.0.0
id: fx-req
use_case_id: create-order
role: input
content_type: application/json
payload: |
  {}
binding:
  channel: http
  selector: {method: POST, path: /orders}
source_refs: [{path: src/x.ts, symbol: "y"}]
`)
	if err := os.WriteFile(filepath.Join(fxDir, "fx-req.yaml"), fxYAMLContent, 0o644); err != nil {
		t.Fatal(err)
	}

	uc := []byte(`schema_version: 2.0.0
id: create-order
title: Create order
archetype_scope: [http-api]
entry_points:
  - id: create-order-endpoint
    archetype: http-api
    spec: {method: POST, path: /orders}
given:
  - "Request matching {{fixtures.fx-req}} is constructed"
when:
  - "Client invokes {{entry_points.nonexistent}}"
then:
  - "Endpoint returns success"
fixture_ids: [fx-req]
`)
	err := Persist(uc, dir)
	if err == nil {
		t.Fatal("expected TemplateResolution failure, got nil")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.TemplateResolution {
		t.Fatalf("Kind=%q, want %q (full error: %v)", pe.Kind, persisterror.TemplateResolution, err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "use-cases", "create-order.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("use-case file written despite TemplateResolution failure: stat=%v", statErr)
	}
}

// fxYAML is a minimal valid fixture used across Persist tests.
// ID uses kebab-case per ^[a-z][a-z0-9-]*$ entity id pattern.
const fxYAML = `schema_version: 1.0.0
id: fx-create-order-request
use_case_id: create-order
role: input
content_type: application/json
payload: |
  {"item":"book"}
binding:
  channel: http
  selector: {method: POST, path: /orders}
source_refs: [{path: src/handlers.ts, symbol: createOrder}]
`

// ucYAML is a minimal valid use-case that references fxYAML.
// entry-point ID and fixture ID both use kebab-case.
const ucYAML = `schema_version: 2.0.0
id: create-order
title: Create order
archetype_scope: [http-api]
entry_points:
  - id: create-order-endpoint
    archetype: http-api
    spec: {method: POST, path: /orders}
given:
  - "Request matching {{fixtures.fx-create-order-request}} is constructed"
when:
  - "Client invokes {{entry_points.create-order-endpoint}}"
then:
  - "Endpoint returns success"
fixture_ids: [fx-create-order-request]
`

// setupFxDir writes the fixture YAML under <dir>/fixtures/ and returns dir.
func setupFxDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	fxDir := filepath.Join(dir, "fixtures")
	if err := os.MkdirAll(fxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fxDir, "fx-create-order-request.yaml"), []byte(fxYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPersist_NewFile_WithReferencedFixturesOnDisk(t *testing.T) {
	dir := setupFxDir(t)

	if err := Persist([]byte(ucYAML), dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	// Use-case file must exist at the expected path.
	ucPath := filepath.Join(dir, "use-cases", "create-order.yaml")
	if _, err := os.Stat(ucPath); err != nil {
		t.Fatalf("use-case not written: %v", err)
	}

	// Sanity check: on-disk fixture store loads OK.
	store, err := fixture.LoadDirectory(filepath.Join(dir, "fixtures"))
	if err != nil {
		t.Fatalf("LoadDirectory sanity: %v", err)
	}
	if _, ok := store.LookupFixture("fx-create-order-request"); !ok {
		t.Fatal("test setup broken: fixture not in store")
	}
}

func TestPersist_RejectsMissingFixtureReference(t *testing.T) {
	dir := setupFxDir(t)

	// Use-case references a fixture that doesn't exist on disk.
	uc := `schema_version: 2.0.0
id: create-order
title: Create order
archetype_scope: [http-api]
entry_points:
  - id: create-order-endpoint
    archetype: http-api
    spec: {method: POST, path: /orders}
given:
  - "Request matching {{fixtures.fx-missing-fixture}} is constructed"
when:
  - "Client invokes {{entry_points.create-order-endpoint}}"
then:
  - "Endpoint returns success"
fixture_ids: [fx-missing-fixture]
`
	err := Persist([]byte(uc), dir)
	if err == nil {
		t.Fatal("expected error for missing fixture, got nil")
	}

	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.FixtureBinding {
		t.Fatalf("expected Kind=%q, got %q", persisterror.FixtureBinding, pe.Kind)
	}

	// Nothing must have been written.
	ucPath := filepath.Join(dir, "use-cases", "create-order.yaml")
	if _, err := os.Stat(ucPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("use-case must not be written on validation failure, stat: %v", err)
	}
}

func TestPersist_AtomicityOnWriteFailure(t *testing.T) {
	// Create a regular file at the path MkdirAll would need as a parent dir.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// harnessDir is a child of a regular file — MkdirAll(use-cases/) will fail.
	harnessDir := filepath.Join(blocker, "inner")

	// Pre-seed a valid fixture so Persist can pass fixture validation before
	// it attempts the directory write.
	fxDir := filepath.Join(harnessDir, "fixtures")
	_ = os.MkdirAll(fxDir, 0o755) // will fail — blocker is a file; that's OK
	// Since we cannot write the fixture dir, use a use-case with no fixture
	// references so Persist reaches the write step before failing.
	uc := []byte(`schema_version: 2.0.0
id: create-order
title: Create order
archetype_scope: [http-api]
entry_points:
  - id: ep1
    archetype: http-api
    spec: {method: POST, path: /orders}
given: ["g"]
when: ["w"]
then: ["t"]
`)
	err := Persist(uc, harnessDir)
	if err == nil {
		t.Fatal("Persist succeeded against unwritable harnessDir")
	}
	ucPath := filepath.Join(harnessDir, "use-cases", "create-order.yaml")
	_, statErr := os.Stat(ucPath)
	if statErr == nil {
		t.Fatalf("partial state left behind: file exists at %s", ucPath)
	}
}

func TestPersist_BumpsExisting(t *testing.T) {
	dir := setupFxDir(t)

	// Pre-seed use-cases/create-order.yaml with schema_version: 2.0.4.
	ucDir := filepath.Join(dir, "use-cases")
	if err := os.MkdirAll(ucDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `schema_version: 2.0.4
id: create-order
title: Create order
archetype_scope: [http-api]
entry_points:
  - id: create-order-endpoint
    archetype: http-api
    spec: {method: POST, path: /orders}
given:
  - "Request matching {{fixtures.fx-create-order-request}} is constructed"
when:
  - "Client invokes {{entry_points.create-order-endpoint}}"
then:
  - "Endpoint returns success"
fixture_ids: [fx-create-order-request]
`
	ucPath := filepath.Join(ucDir, "create-order.yaml")
	if err := os.WriteFile(ucPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Persist([]byte(ucYAML), dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	// Read back and check schema_version was bumped to 2.0.5.
	data, err := os.ReadFile(ucPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "schema_version: 2.0.5") {
		t.Fatalf("expected schema_version: 2.0.5, got:\n%s", data)
	}
}
