package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// passingTreeRel is the relative path to the hand-authored passing
// .harness/ fixture tree. Tests that need it use t.Skip when it does
// not exist on disk so the CI run does not depend on it. The tree is
// expected to be added under cmd/harness/testdata/harness/passing/.harness/
// in a follow-up commit; until then these tests act as documentation
// of the contract LoadHarnessArtifacts must satisfy.
const passingTreeRel = "testdata/harness/passing/.harness"

func TestLoadHarnessArtifacts_HappyPath(t *testing.T) {
	harnessDir, err := filepath.Abs(passingTreeRel)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(harnessDir); err != nil {
		t.Skipf("fixture tree missing (%v); hand-author cmd/harness/testdata/harness/passing/.harness/ to enable", err)
	}

	policyPath := filepath.Join(harnessDir, "validation-policy.yaml")
	arts, err := LoadHarnessArtifacts(harnessDir, policyPath)
	if err != nil {
		t.Fatalf("LoadHarnessArtifacts: %v", err)
	}
	if arts.Sensors == nil || arts.Fixtures == nil {
		t.Fatalf("missing stores: %+v", arts)
	}
	if len(arts.UseCases) == 0 {
		t.Fatalf("no use cases loaded")
	}
	if arts.RuntimeRoot != filepath.Join(harnessDir, "runtime") {
		t.Errorf("RuntimeRoot = %q, want %q", arts.RuntimeRoot, filepath.Join(harnessDir, "runtime"))
	}
}

func TestLoadHarnessArtifacts_MissingStackManifest(t *testing.T) {
	tmp := t.TempDir()
	// Create empty .harness/ — no files inside.
	if err := os.MkdirAll(filepath.Join(tmp, "fixtures"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := LoadHarnessArtifacts(tmp, filepath.Join(tmp, "validation-policy.yaml"))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "stack manifest") {
		t.Errorf("error = %v, want substring 'stack manifest'", err)
	}
}
