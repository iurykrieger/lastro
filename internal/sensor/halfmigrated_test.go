package sensor

import (
	"os"
	"path/filepath"
	"testing"
)

// validUseCaseSensor returns a schema-valid use-case sensor YAML.
func validUseCaseSensor(id, useCaseID, angle string) string {
	return "schema_version: 1.0.0\n" +
		"id: " + id + "\n" +
		"use_case_id: " + useCaseID + "\n" +
		"angle: " + angle + "\n" +
		"kind: assertion\n" +
		"nature: computational\n" +
		"output_type: single-shot\n" +
		"uses: []\n" +
		"steps:\n" +
		"  - id: run\n" +
		"    run: go test ./...\n"
}

// TestLoadDirectory_HalfMigrated_FlatAndFolderedCoexist guards the
// transition required by spec §11 Q4: while sensors are being migrated from
// the flat layout (sensors/<id>.yaml) into per-use-case folders
// (sensors/<uc>/<id>.yaml), the loader must read both shapes in one pass.
func TestLoadDirectory_HalfMigrated_FlatAndFolderedCoexist(t *testing.T) {
	root := t.TempDir()

	// Flat (not yet migrated).
	if err := os.WriteFile(filepath.Join(root, "flat-build.yaml"),
		[]byte(validUseCaseSensor("flat-build", "uc-x", "build")), 0o644); err != nil {
		t.Fatal(err)
	}
	// Foldered (already migrated).
	sub := filepath.Join(root, "uc-x")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "foldered-unit.yaml"),
		[]byte(validUseCaseSensor("foldered-unit", "uc-x", "unit-test")), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := LoadDirectory(root)
	if err != nil {
		t.Fatalf("LoadDirectory(half-migrated): %v", err)
	}
	got := store.ForUseCase("uc-x")
	if len(got) != 2 {
		t.Fatalf("ForUseCase(uc-x) = %d sensors, want 2 (flat + foldered)", len(got))
	}
	if got[0].ID != "flat-build" || got[1].ID != "foldered-unit" {
		t.Errorf("ids = %v, want [flat-build foldered-unit]", []string{got[0].ID, got[1].ID})
	}
}
