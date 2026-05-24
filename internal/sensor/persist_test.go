package sensor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

// seedHappyPath writes a complete, consistent set of fixtures to dir:
//   - stack-manifest.yaml (http-api, includes express)
//   - fixtures/fx-req.yaml (owned by create-order)
//   - use-cases/create-order.yaml
//
// It returns dir unchanged for fluent use.
func seedHappyPath(t *testing.T, dir string) string {
	t.Helper()

	stackYAML := []byte(`schema_version: 1.0.0
archetype: http-api
applicable_angles: [security, build, code-structure, unit-test, e2e-test, contracts, logs, metrics, database, performance]
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence: [{file: package.json, path: .dependencies.express}]
`)
	if err := os.WriteFile(filepath.Join(dir, "stack-manifest.yaml"), stackYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	fxDir := filepath.Join(dir, "fixtures")
	if err := os.MkdirAll(fxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fxDir, "fx-req.yaml"), []byte(`schema_version: 1.0.0
id: fx-req
use_case_id: create-order
role: input
content_type: application/json
payload: |
  {}
binding: {channel: http, selector: {method: POST, path: /orders}}
source_refs: [{path: src/x.ts, symbol: "y"}]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	ucDir := filepath.Join(dir, "use-cases")
	if err := os.MkdirAll(ucDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ucDir, "create-order.yaml"), []byte(`schema_version: 2.0.0
id: create-order
title: Create order
archetype_scope: [http-api]
entry_points: [{id: ep1, archetype: http-api, spec: {method: POST, path: /orders}}]
given: ["g"]
when: ["w"]
then: ["t"]
fixture_ids: [fx-req]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// happySensorYAML is a valid sensor for create-order that passes all
// cross-entity checks when the happy-path seed is present.
var happySensorYAML = []byte(`schema_version: 1.0.0
id: s-create-order-e2e
use_case_id: create-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    run: curl -X POST localhost:8080/orders
    uses: [fx-req]
`)

func TestPersist_NewFile_AllInvariantsSatisfied(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())

	if err := Persist(happySensorYAML, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sensors", "s-create-order-e2e.yaml")); err != nil {
		t.Fatalf("sensor not written: %v", err)
	}
}

func TestPersist_Grounding_RejectsUnknownStackComponent(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())

	s := []byte(`schema_version: 1.0.0
id: s-bad-grounding
use_case_id: create-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [unknown-lib]
steps:
  - id: probe
    run: echo hi
`)
	err := Persist(s, dir)
	if err == nil {
		t.Fatal("expected error for unknown stack component, got nil")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.Grounding {
		t.Fatalf("expected Kind %q, got %q", persisterror.Grounding, pe.Kind)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "sensors", "s-bad-grounding.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("sensor file must not be written on grounding failure")
	}
}

func TestPersist_AngleNotApplicable_RejectsAngleNotInManifest(t *testing.T) {
	dir := t.TempDir()

	// cli archetype does NOT include the "database" angle.
	stackYAML := []byte(`schema_version: 1.0.0
archetype: cli
applicable_angles: [security, build, code-structure, unit-test, contracts, logs]
components:
  - schema_version: 1.0.0
    id: cobra
    kind: library
    name: cobra
    version: 1.7.0
    capabilities: [cli-parsing]
    detection_evidence: [{file: go.mod, path: .require.github.com/spf13/cobra}]
`)
	if err := os.WriteFile(filepath.Join(dir, "stack-manifest.yaml"), stackYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	ucDir := filepath.Join(dir, "use-cases")
	if err := os.MkdirAll(ucDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ucDir, "run-cli.yaml"), []byte(`schema_version: 2.0.0
id: run-cli
title: Run CLI command
archetype_scope: [cli]
entry_points: [{id: ep1, archetype: cli, spec: {command: harness}}]
given: ["g"]
when: ["w"]
then: ["t"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := []byte(`schema_version: 1.0.0
id: s-cli-db
use_case_id: run-cli
angle: database
kind: assertion
nature: computational
output_type: single-shot
uses: [cobra]
steps:
  - id: probe
    run: echo hi
`)
	err := Persist(s, dir)
	if err == nil {
		t.Fatal("expected AngleNotApplicable error, got nil")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.AngleNotApplicable {
		t.Fatalf("expected Kind %q, got %q", persisterror.AngleNotApplicable, pe.Kind)
	}
}

func TestPersist_MissingDependency_RejectsUnknownUseCase(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())

	s := []byte(`schema_version: 1.0.0
id: s-no-uc
use_case_id: nonexistent
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    run: echo hi
`)
	err := Persist(s, dir)
	if err == nil {
		t.Fatal("expected MissingDependency error, got nil")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.MissingDependency {
		t.Fatalf("expected Kind %q, got %q", persisterror.MissingDependency, pe.Kind)
	}
}

func TestPersist_FixtureBinding_RejectsStepUsingUnownedFixture(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())

	// Add a fixture owned by a *different* use case.
	if err := os.WriteFile(filepath.Join(dir, "fixtures", "fx-other.yaml"), []byte(`schema_version: 1.0.0
id: fx-other
use_case_id: other-uc
role: input
content_type: application/json
payload: |
  {}
binding: {channel: http, selector: {method: GET, path: /other}}
source_refs: [{path: src/z.ts, symbol: z}]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := []byte(`schema_version: 1.0.0
id: s-bad-fixture
use_case_id: create-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    run: echo hi
    uses: [fx-other]
`)
	err := Persist(s, dir)
	if err == nil {
		t.Fatal("expected FixtureBinding error, got nil")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.FixtureBinding {
		t.Fatalf("expected Kind %q, got %q", persisterror.FixtureBinding, pe.Kind)
	}
}

func TestPersist_BumpsExisting(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())

	// Pre-seed an existing sensor with schema_version 1.0.4.
	sensorDir := filepath.Join(dir, "sensors")
	if err := os.MkdirAll(sensorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := []byte(`schema_version: 1.0.4
id: s-create-order-e2e
use_case_id: create-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    run: curl -X POST localhost:8080/orders
    uses: [fx-req]
`)
	if err := os.WriteFile(filepath.Join(sensorDir, "s-create-order-e2e.yaml"), existing, 0o644); err != nil {
		t.Fatal(err)
	}

	// Persist new content with schema_version 1.0.0 — should bump to 1.0.5.
	if err := Persist(happySensorYAML, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(sensorDir, "s-create-order-e2e.yaml"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}

	// Quick check: the written file must contain "1.0.5".
	import_check := string(written)
	if !contains(import_check, "1.0.5") {
		t.Fatalf("expected schema_version 1.0.5 in written file, got:\n%s", written)
	}
}

// contains is a minimal helper to avoid importing strings in this file.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
