package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

const evoStackManifest = `schema_version: 1.0.0
archetype: http-api
applicable_angles: [security, build, code-structure, unit-test, e2e-test, contracts, logs, metrics, database, performance]
components:
  - schema_version: 1.0.0
    id: curl
    kind: tool
    name: curl
    version: "8.0"
    capabilities: [http-client]
    detection_evidence: [{file: Dockerfile, path: .}]
`

// evoCorePrimitive satisfies the e2e-test baseline floor (echo-based run
// keeps step resolvability trivial). extraInputs/extraRun let the
// evolution step add a new input and reference it.
func evoCorePrimitive(extraInputs, extraRun string) string {
	return `schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
inputs:
  base_url:      { required: true,  default: "http://localhost:8080" }
  method:        { required: true,  default: GET }
  path:          { required: true,  default: / }
  query:         { required: false, default: "" }
  headers:       { required: false, default: "" }
  body:          { required: false, default: "" }
  expect_status: { required: true,  default: 2xx }
  timeout:       { required: false, default: "10" }
` + extraInputs + `steps:
  - id: request
    run: |
      echo "${{ inputs.base_url }} ${{ inputs.method }} ${{ inputs.path }}"
      echo "${{ inputs.query }} ${{ inputs.headers }} ${{ inputs.body }}"
      echo "${{ inputs.expect_status }} ${{ inputs.timeout }}"
` + extraRun
}

const evoUseCaseSensor = `schema_version: 1.0.0
id: s-uc-checkout-e2e-test
scope: use-case
use_case_id: uc-checkout
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: reject
    uses: e2e-test
    with:
      method: POST
      path: /v1/charges
      expect_status: "422"
      idempotency_key: abc123
`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPersistCreateSensors_UnknownWithKey_ThenEvolutionFlow(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	writeFile(t, filepath.Join(harness, "stack-manifest.yaml"), evoStackManifest)
	writeFile(t, filepath.Join(harness, "use-cases", "uc-checkout.yaml"), "id: uc-checkout\n")

	// Seed the core primitive (without idempotency_key) via create-core-sensors.
	coreIn := filepath.Join(dir, "core.yaml")
	writeFile(t, coreIn, evoCorePrimitive("", ""))
	var out, errOut bytes.Buffer
	code := persistCreateCoreSensors(
		[]string{"create-core-sensors", "--file", coreIn, "--harness-dir", harness}, &out, &errOut)
	if code != 0 {
		t.Fatalf("seed core: exit=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}

	// 1. Binding an undeclared input fails with unknown_with_key.
	ucIn := filepath.Join(dir, "uc-sensor.yaml")
	writeFile(t, ucIn, evoUseCaseSensor)
	out.Reset()
	errOut.Reset()
	code = persistCreateSensors(
		[]string{"create-sensors", "--file", ucIn, "--harness-dir", harness}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit=%d, want 2; stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	var pe persisterror.Error
	if err := json.Unmarshal(out.Bytes(), &pe); err != nil {
		t.Fatalf("stdout is not persisterror JSON: %v\nstdout=%q", err, out.String())
	}
	if pe.Kind != persisterror.UnknownWithKey {
		t.Fatalf("Kind=%q, want %q", pe.Kind, persisterror.UnknownWithKey)
	}
	if pe.EntityType != "sensor" || pe.EntityID != "s-uc-checkout-e2e-test" {
		t.Fatalf("error should name the failing sensor: %+v", pe)
	}

	// 2. Evolve the core: add the input (with default) AND reference it.
	writeFile(t, coreIn, evoCorePrimitive(
		"  idempotency_key: { required: false, default: \"\" }\n",
		"      echo \"${{ inputs.idempotency_key }}\"\n"))
	out.Reset()
	errOut.Reset()
	code = persistCreateCoreSensors(
		[]string{"create-core-sensors", "--file", coreIn, "--harness-dir", harness}, &out, &errOut)
	if code != 0 {
		t.Fatalf("evolve core: exit=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}

	// 3. Retry the use-case sensor — now persists.
	out.Reset()
	errOut.Reset()
	code = persistCreateSensors(
		[]string{"create-sensors", "--file", ucIn, "--harness-dir", harness}, &out, &errOut)
	if code != 0 {
		t.Fatalf("retry: exit=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if _, err := os.Stat(filepath.Join(harness, "sensors", "uc-checkout", "s-uc-checkout-e2e-test.yaml")); err != nil {
		t.Fatalf("use-case sensor not written: %v", err)
	}
}
