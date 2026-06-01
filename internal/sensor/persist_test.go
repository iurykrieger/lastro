package sensor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
    run: curl -X POST localhost:8080/orders -d ${{ fixtures.fx-req }}
`)

func TestPersist_NewFile_AllInvariantsSatisfied(t *testing.T) {
	dir := seedHappyPath(t, t.TempDir())

	if err := Persist(happySensorYAML, dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sensors", "create-order", "s-create-order-e2e.yaml")); err != nil {
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
    run: echo ${{ fixtures.fx-other }}
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

	// Pre-seed an existing sensor with schema_version 1.0.4 at the per-use-case path.
	sensorDir := filepath.Join(dir, "sensors", "create-order")
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
    run: curl -X POST localhost:8080/orders -d ${{ fixtures.fx-req }}
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
	got := string(written)
	if !strings.Contains(got, "1.0.5") {
		t.Fatalf("expected schema_version 1.0.5 in written file, got:\n%s", written)
	}
}

func TestPersist_CoreSensorWritesToCoreFolder(t *testing.T) {
	// seedHappyPath provides a stack-manifest with build in applicable_angles
	// but NOT environment — the core sensor must be accepted regardless
	// because checks (b)(c)(d) are skipped for scope: core.
	dir := seedHappyPath(t, t.TempDir())
	content := []byte(`schema_version: 1.0.0
id: run-dev
scope: core
angle: environment
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: boot
    run: "make dev"
`)
	if err := Persist(content, dir); err != nil {
		t.Fatalf("persist core: %v", err)
	}
	got := filepath.Join(dir, "sensors", "core", "run-dev.yaml")
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("expected core sensor at %s: %v", got, err)
	}
}

func TestPersist_UseCaseSensorWritesToUseCaseFolder(t *testing.T) {
	// seedHappyPath provides a stack-manifest with build in applicable_angles
	// and a use-case "create-order". We persist a use-case sensor with no
	// step uses to avoid fixture-binding complexity.
	dir := seedHappyPath(t, t.TempDir())
	content := []byte(`schema_version: 1.0.0
id: create-order-build
scope: use-case
use_case_id: create-order
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: compile
    run: "go build ./..."
`)
	if err := Persist(content, dir); err != nil {
		t.Fatalf("persist use-case: %v", err)
	}
	got := filepath.Join(dir, "sensors", "create-order", "create-order-build.yaml")
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("expected use-case sensor at %s: %v", got, err)
	}
}

func TestPersist_MissingDependency_MissingStackManifest(t *testing.T) {
	dir := t.TempDir() // empty — no stack-manifest.yaml

	s := []byte(`schema_version: 1.0.0
id: s-x
use_case_id: create-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    run: noop
`)
	err := Persist(s, dir)
	if err == nil {
		t.Fatal("expected MissingDependency, got nil")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.MissingDependency {
		t.Fatalf("Kind=%q, want %q", pe.Kind, persisterror.MissingDependency)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "sensors", "s-x.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("sensor file written despite missing stack manifest: stat=%v", statErr)
	}
}
