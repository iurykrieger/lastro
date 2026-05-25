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
	bin, err := os.MkdirTemp("", "detect-stack-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(bin)

	binary = filepath.Join(bin, "detect-stack")
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

func TestDetectStack_HappyPath(t *testing.T) {
	dir := t.TempDir()
	harness := filepath.Join(dir, ".harness")
	input := filepath.Join(dir, "in.yaml")
	if err := os.WriteFile(input, []byte(`schema_version: 1.0.0
archetype: http-api
components:
  - schema_version: 1.0.0
    id: express
    kind: library
    name: express
    version: 4.18.0
    capabilities: [http-routing]
    detection_evidence: [{file: package.json, path: .dependencies.express}]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, serr, code := runScript(t, "--file", input, "--harness-dir", harness)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, serr)
	}
	if sout != "" {
		t.Fatalf("expected empty stdout on success, got %q", sout)
	}
	if _, err := os.Stat(filepath.Join(harness, "stack-manifest.yaml")); err != nil {
		t.Fatalf("stack-manifest not written: %v", err)
	}
}

func TestDetectStack_ValidationFailure_ExitsTwoWithJSON(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.yaml")
	// Use bad archetype so Persist fails at the schema/enum level.
	if err := os.WriteFile(input, []byte(`schema_version: 1.0.0
archetype: not-a-real-archetype
components:
  - schema_version: 1.0.0
    id: x
    kind: library
    name: x
    version: 1.0.0
    capabilities: [c]
    detection_evidence: [{file: f, path: p}]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sout, _, code := runScript(t, "--file", input, "--harness-dir", filepath.Join(dir, ".harness"))
	if code != 2 {
		t.Fatalf("exit=%d, want 2", code)
	}
	var pe persisterror.Error
	if err := json.Unmarshal([]byte(sout), &pe); err != nil {
		t.Fatalf("stdout is not a persisterror.Error JSON: %v\nstdout=%q", err, sout)
	}
	if pe.EntityType != "stack-manifest" {
		t.Fatalf("EntityType=%q, want stack-manifest", pe.EntityType)
	}
}

func TestDetectStack_MissingFlag_ExitsOne(t *testing.T) {
	_, serr, code := runScript(t)
	if code != 1 {
		t.Fatalf("exit=%d, want 1", code)
	}
	if !strings.Contains(serr, "--file") {
		t.Fatalf("stderr=%q should mention --file", serr)
	}
}
