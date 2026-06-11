package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/fixture"
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

func TestLoadUseCases_JourneyFoldersAndFlat(t *testing.T) {
	dir := t.TempDir()
	flatUC := `schema_version: 2.0.0
id: uc-flat
title: Flat
archetype_scope: [library]
entry_points:
  - {id: ep-flat, archetype: library, spec: {symbol: F}}
given: ["g"]
when: ["w"]
then: ["t"]
`
	journeyUC := `schema_version: 2.0.0
id: uc-journeyed
title: Journeyed
archetype_scope: [library]
entry_points:
  - {id: ep-j, archetype: library, spec: {symbol: J}}
given: ["g"]
when: ["w"]
then: ["t"]
journey: orders
variation: success
`
	if err := os.WriteFile(filepath.Join(dir, "uc-flat.yaml"), []byte(flatUC), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "orders"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "orders", "uc-journeyed.yaml"), []byte(journeyUC), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := fixture.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	out, err := loadUseCases(dir, store)
	if err != nil {
		t.Fatalf("loadUseCases: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("loaded %d use cases, want 2 (flat + journeyed): %v", len(out), out)
	}
	if out["uc-journeyed"] == nil || out["uc-journeyed"].Journey != "orders" {
		t.Errorf("journeyed use case not loaded correctly: %+v", out["uc-journeyed"])
	}
	if out["uc-flat"] == nil {
		t.Error("flat use case not loaded")
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
