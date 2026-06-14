package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectEnvironment_FactsMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"next dev"}}`), 0o644)
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
	os.WriteFile(model, []byte("schema_version: 1.0.0\napplication:\n  provided_by: {file: package.json, path: scripts.ghost}\n"), 0o644)
	os.WriteFile(facts, []byte("scripts:\n  dev: next dev\n"), 0o644)
	var out, errb bytes.Buffer
	code := detectEnvironment([]string{"detect-environment", "--mode", "persist", "--file", model, "--facts", facts, "--harness-dir", filepath.Join(dir, ".harness")}, &out, &errb)
	if code != 2 {
		t.Fatalf("want exit 2 (validation), got %d (stderr=%s)", code, errb.String())
	}
	if !strings.Contains(out.String(), "environment-model") {
		t.Fatalf("want JSON persisterror on stdout, got %s", out.String())
	}
}
