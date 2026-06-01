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
	bin, err := os.MkdirTemp("", "detect-use-cases-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(bin)

	binary = filepath.Join(bin, "detect-use-cases")
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

// validFixtureYAML is a minimal valid fixture for use in tests.
const validFixtureYAML = `schema_version: 1.0.0
id: fx-req
use_case_id: create-order
role: input
content_type: application/json
payload: |
  {}
binding: {channel: http, selector: {method: POST, path: /orders}}
source_refs: [{path: src/x.ts, symbol: "y"}]
`

// validUseCaseYAML is a minimal valid use-case that references fx-req
// in both fixture_ids and the given/when/then templates.
const validUseCaseYAML = `schema_version: 2.0.0
id: create-order
title: Create order
archetype_scope: [http-api]
entry_points:
  - id: ep1
    archetype: http-api
    spec: {method: POST, path: /orders}
given:
  - "Request matching ${{fixtures.fx-req}} is constructed"
when:
  - "Client invokes ${{entry_points.ep1}}"
then:
  - "Endpoint returns success"
fixture_ids: [fx-req]
`

func TestDetectUseCases_Fixture_HappyPath(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(validFixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, serr, code := runScript(t, "--type", "fixture", "--file", input, "--harness-dir", harness)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, serr, sout)
	}
	if sout != "" {
		t.Fatalf("expected empty stdout on success, got %q", sout)
	}
	if _, err := os.Stat(filepath.Join(harness, "fixtures", "fx-req.yaml")); err != nil {
		t.Fatalf("fixture not written: %v", err)
	}
}

func TestDetectUseCases_UseCase_HappyPath_WithFixtureOnDisk(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")

	// Pre-seed the fixture that the use-case references.
	fxDir := filepath.Join(harness, "fixtures")
	if err := os.MkdirAll(fxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fxDir, "fx-req.yaml"), []byte(validFixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(validUseCaseYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, serr, code := runScript(t, "--type", "use-case", "--file", input, "--harness-dir", harness)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, serr, sout)
	}
	if sout != "" {
		t.Fatalf("expected empty stdout on success, got %q", sout)
	}
	if _, err := os.Stat(filepath.Join(harness, "use-cases", "create-order.yaml")); err != nil {
		t.Fatalf("use-case not written: %v", err)
	}
}

func TestMain_MissingFlag_ExitsOne(t *testing.T) {
	_, serr, code := runScript(t)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if !strings.Contains(serr, "--file") {
		t.Fatalf("stderr=%q should mention --file", serr)
	}
}

func TestMain_InvalidType_ExitsOne(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, serr, code := runScript(t, "--type", "bogus", "--file", input)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if !strings.Contains(serr, "--type") {
		t.Fatalf("stderr=%q should mention --type", serr)
	}
}

func TestDetectUseCases_UseCase_RejectsMissingFixture_ExitsTwoWithJSON(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")

	// No fixture on disk — use-case references fx-nonexistent.
	ucYAML := `schema_version: 2.0.0
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
fixture_ids: [fx-nonexistent]
`
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(ucYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, _, code := runScript(t, "--type", "use-case", "--file", input, "--harness-dir", harness)
	if code != 2 {
		t.Fatalf("exit=%d, want 2; stdout=%q", code, sout)
	}
	var pe persisterror.Error
	if err := json.Unmarshal([]byte(sout), &pe); err != nil {
		t.Fatalf("stdout is not a persisterror.Error JSON: %v\nstdout=%q", err, sout)
	}
	if pe.Kind != persisterror.FixtureBinding {
		t.Fatalf("Kind=%q, want %q", pe.Kind, persisterror.FixtureBinding)
	}
}
