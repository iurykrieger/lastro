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
	bin, err := os.MkdirTemp("", "create-core-sensors-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(bin)

	binary = filepath.Join(bin, "create-core-sensors")
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

// seedHarness writes a stack-manifest with curl and run-dev components to harnessDir.
func seedHarness(t *testing.T, harnessDir string) {
	t.Helper()

	stackYAML := []byte(`schema_version: 1.0.0
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
  - schema_version: 1.0.0
    id: run-dev
    kind: tool
    name: run-dev
    version: "1.0"
    capabilities: [lifecycle]
    detection_evidence: [{file: Makefile, path: .}]
`)
	if err := os.WriteFile(filepath.Join(harnessDir, "stack-manifest.yaml"), stackYAML, 0o644); err != nil {
		t.Fatal(err)
	}
}

// happyCoreSensorYAML is a valid core sensor that passes all checks when
// the seeded harness is present: full e2e-test baseline floor, every
// input referenced, grade-and-emit shape (no curl --fail).
const happyCoreSensorYAML = `schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
depends_on: [run-dev]
inputs:
  base_url:      { required: true,  default: "http://localhost:8080" }
  method:        { required: true,  default: GET }
  path:          { required: true,  default: /health_check/ready }
  query:         { required: false, default: "" }
  headers:       { required: false, default: "" }
  body:          { required: false, default: "" }
  expect_status: { required: true,  default: 2xx }
  timeout:       { required: false, default: "10" }
outputs:
  status: { from: "${{ steps.request.outputs.status }}" }
  body:   { from: "${{ steps.request.outputs.body }}" }
signal_matches:
  - key: expectation-unmet
    pattern: "expectation-unmet expected=(?P<expected>\\S+) got=(?P<got>\\d+)"
    verdict: fail
    heal_hint: { summary: "Response status did not match the declared expectation", rationale: "The endpoint answered outside expect_status; inspect the handler or the expectation binding." }
steps:
  - id: request
    run: |
      hdr_file=$(mktemp)
      printf '%s' "${{ inputs.headers }}" > "$hdr_file"
      if [ -n "${{ inputs.body }}" ]; then set -- --data-binary "@${{ inputs.body }}"; else set --; fi
      status=$(curl -sS -o /tmp/harness-e2e-body -w '%{http_code}' \
        --max-time "${{ inputs.timeout }}" -X "${{ inputs.method }}" \
        -H "@$hdr_file" "$@" \
        "${{ inputs.base_url }}${{ inputs.path }}${{ inputs.query }}")
      body=$(cat /tmp/harness-e2e-body 2>/dev/null || true)
      printf 'status=%s\nbody=%s\n' "$status" "$body"
      printf 'status=%s\nbody=%s\n' "$status" "$body" >> "$HARNESS_OUTPUT"
      expect=$(printf '%s' "${{ inputs.expect_status }}" | tr 'xX' '??')
      case "$status" in
        $expect) ;;
        *) printf 'expectation-unmet expected=%s got=%s\n' "${{ inputs.expect_status }}" "$status"; exit 1 ;;
      esac
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

func TestCreateCoreSensors_HappyPath(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	seedHarness(t, harness)

	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(happyCoreSensorYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, serr, code := runScript(t, "--file", input, "--harness-dir", harness)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, serr, sout)
	}
	if sout != "" {
		t.Fatalf("expected empty stdout on success, got %q", sout)
	}
	if _, err := os.Stat(filepath.Join(harness, "sensors", "core", "e2e-test.yaml")); err != nil {
		t.Fatalf("sensor not written at sensors/core/e2e-test.yaml: %v", err)
	}
}

func TestCreateCoreSensors_Grounding_ExitsTwoWithJSON(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	seedHarness(t, harness)

	// Sensor references unknown-tool which is not in the stack manifest.
	badGroundingSensorYAML := `schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [unknown-tool]
steps:
  - id: probe
    run: echo hi
`
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(badGroundingSensorYAML), 0o644); err != nil {
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

func TestCreateCoreSensors_NotCoreScope_ExitsTwoWithJSON(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	seedHarness(t, harness)

	// Sensor with use-case scope — not valid for create-core-sensors.
	useCaseScopedYAML := `schema_version: 1.0.0
id: s-some-order-e2e
scope: use-case
use_case_id: some-order
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
steps:
  - id: probe
    run: echo hi
`
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(useCaseScopedYAML), 0o644); err != nil {
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
	if !strings.Contains(pe.Message, "scope") {
		t.Fatalf("message %q should mention scope", pe.Message)
	}
}

func TestCreateCoreSensors_IncompleteInputSurface_ExitsTwoWithJSON(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	seedHarness(t, harness)

	// e2e-test-angle core sensor declaring only a slice of the baseline floor.
	narrowYAML := `schema_version: 1.0.0
id: e2e-test
scope: core
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: [curl]
inputs:
  method: { required: true, default: GET }
  path:   { required: true, default: / }
steps:
  - id: request
    run: 'curl -sS -X "${{ inputs.method }}" "http://localhost:8080${{ inputs.path }}"'
`
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(narrowYAML), 0o644); err != nil {
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
	if pe.Kind != persisterror.IncompleteInputSurface {
		t.Fatalf("Kind=%q, want %q", pe.Kind, persisterror.IncompleteInputSurface)
	}
}
