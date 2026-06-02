package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

// binary holds the path to the built executable; set by TestMain.
var binary string

// TestMain compiles the package once into a temp binary, then runs all
// tests against that binary. This avoids `go run` exit-code wrapping
// (go run normalises all non-zero exits to 1).
func TestMain(m *testing.M) {
	bin, err := os.MkdirTemp("", "create-sensors-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(bin)

	binary = filepath.Join(bin, "create-sensors")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("build failed: " + err.Error())
	}
	os.Exit(m.Run())
}

// runScript invokes the compiled binary and returns stdout, stderr, exit code.
func runScript(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var sout, serr strings.Builder
	cmd.Stdout = &sout
	cmd.Stderr = &serr
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("exec: %v", err)
	}
	return sout.String(), serr.String(), code
}

// seedHarness writes a complete, consistent set of entities to harnessDir:
//   - stack-manifest.yaml (http-api, includes express)
//   - fixtures/fx-req.yaml (owned by create-order)
//   - use-cases/create-order.yaml
func seedHarness(t *testing.T, harnessDir string) {
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
	if err := os.WriteFile(filepath.Join(harnessDir, "stack-manifest.yaml"), stackYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	fxDir := filepath.Join(harnessDir, "fixtures")
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

	ucDir := filepath.Join(harnessDir, "use-cases")
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
}

// happySensorYAML is a valid sensor for create-order that passes all
// cross-entity checks when the seeded harness is present.
// Steps use the new composition model: fixtures are referenced via
// ${{ fixtures.<id> }} interpolation inside `run`, not via a step-level
// uses array.
const happySensorYAML = `schema_version: 1.0.0
id: s-create-order-e2e
use_case_id: create-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    run: curl -X POST localhost:8080/orders -d '${{ fixtures.fx-req }}'
`

func TestMain_MissingFlag_ExitsOne(t *testing.T) {
	_, serr, code := runScript(t)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if !strings.Contains(serr, "--file") {
		t.Fatalf("stderr=%q should mention --file", serr)
	}
}

func TestCreateSensors_HappyPath(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	seedHarness(t, harness)

	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(happySensorYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, serr, code := runScript(t, "--file", input, "--harness-dir", harness)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, serr, sout)
	}
	if sout != "" {
		t.Fatalf("expected empty stdout on success, got %q", sout)
	}
	if _, err := os.Stat(filepath.Join(harness, "sensors", "create-order", "s-create-order-e2e.yaml")); err != nil {
		t.Fatalf("sensor not written: %v", err)
	}
}

func TestCreateSensors_Grounding_ExitsTwoWithJSON(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	seedHarness(t, harness)

	// Sensor references unknown-lib which is not in the stack manifest.
	badSensorYAML := `schema_version: 1.0.0
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
`
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(badSensorYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, _, code := runScript(t, "--file", input, "--harness-dir", harness)
	if code != 2 {
		t.Fatalf("exit=%d, want 2; stdout=%q", code, sout)
	}
	var pe persisterror.Error
	if err := json.Unmarshal([]byte(sout), &pe); err != nil {
		t.Fatalf("stdout is not a persisterror.Error JSON: %v\nstdout=%q", err, sout)
	}
	if pe.Kind != persisterror.Grounding {
		t.Fatalf("Kind=%q, want %q", pe.Kind, persisterror.Grounding)
	}
}

// e2eCorePrimitiveYAML is a core primitive with a required input `path`
// (no default). Used by composition validation tests.
const e2eCorePrimitiveYAML = `schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
inputs:
  path: { required: true }
steps:
  - id: request
    run: curl -sS http://localhost:8080${{ inputs.path }}
`

// seedCoreDir writes e2eCorePrimitiveYAML into <harnessDir>/sensors/core/.
func seedCoreDir(t *testing.T, harnessDir string) {
	t.Helper()
	coreDir := filepath.Join(harnessDir, "sensors", "core")
	if err := os.MkdirAll(coreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coreDir, "e2e-test.yaml"), []byte(e2eCorePrimitiveYAML), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCreateSensors_Composition_BoundRequired passes when a consumer uses-step
// supplies every required input of the core primitive.
func TestCreateSensors_Composition_BoundRequired(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	seedHarness(t, harness)
	seedCoreDir(t, harness)

	// Consumer uses the core primitive and binds the required `path` input.
	consumerYAML := `schema_version: 1.0.0
id: s-create-order-e2e-composed
use_case_id: create-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    uses: e2e-test
    with:
      path: /orders
`
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(consumerYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, serr, code := runScript(t, "--file", input, "--harness-dir", harness)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, serr, sout)
	}
}

// TestCreateSensors_Composition_UnboundRequired fails (exit 2, schema_violation)
// when a consumer uses-step leaves a required input of the core primitive unbound.
func TestCreateSensors_Composition_UnboundRequired(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	seedHarness(t, harness)
	seedCoreDir(t, harness)

	// Consumer uses the core primitive but omits the required `path` input.
	consumerYAML := `schema_version: 1.0.0
id: s-create-order-e2e-unbound
use_case_id: create-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [express]
steps:
  - id: probe
    uses: e2e-test
    with: {}
`
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(consumerYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, _, code := runScript(t, "--file", input, "--harness-dir", harness)
	if code != 2 {
		t.Fatalf("exit=%d, want 2; stdout=%q", code, sout)
	}
	var pe persisterror.Error
	if err := json.Unmarshal([]byte(sout), &pe); err != nil {
		t.Fatalf("stdout is not a persisterror.Error JSON: %v\nstdout=%q", err, sout)
	}
	if pe.Kind != persisterror.SchemaViolation {
		t.Fatalf("Kind=%q, want %q", pe.Kind, persisterror.SchemaViolation)
	}
	if !strings.Contains(pe.Message, "path") {
		t.Fatalf("message %q should mention the unbound input \"path\"", pe.Message)
	}
}
