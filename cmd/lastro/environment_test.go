package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectEnvironment_FactsMode(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "package.json"), `{"scripts":{"dev":"next dev"}}`)
	var out, errb bytes.Buffer
	code := detectEnvironment([]string{"detect-environment", "--mode", "facts", "--repo", dir}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "next dev") {
		t.Fatalf("facts output missing script: %s", out.String())
	}
}

func TestDetectEnvironment_PersistValidationError(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "m.yaml")
	facts := filepath.Join(dir, "f.yaml")
	mustWrite(t, model, "schema_version: 1.0.0\napplication:\n  provided_by: {file: package.json, path: scripts.ghost}\n")
	mustWrite(t, facts, "scripts:\n  dev: next dev\n")
	var out, errb bytes.Buffer
	code := detectEnvironment([]string{"detect-environment", "--mode", "persist", "--file", model, "--facts", facts, "--harness-dir", filepath.Join(dir, ".harness")}, &out, &errb)
	if code != 2 {
		t.Fatalf("want exit 2 (validation), got %d (stderr=%s)", code, errb.String())
	}
	if !strings.Contains(out.String(), "environment-model") {
		t.Fatalf("want JSON persisterror on stdout, got %s", out.String())
	}
}
