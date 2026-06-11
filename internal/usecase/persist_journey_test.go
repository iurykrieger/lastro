package usecase

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

const inventoryYAML = `schema_version: 1.0.0
source_root: .
files:
  - {path: handlers.go, precision: ast}
branches:
  - {id: br-aaaaaaaaaaaa, file: handlers.go, line: 10, kind: if, condition: "err != nil"}
  - {id: br-bbbbbbbbbbbb, file: handlers.go, line: 14, kind: else, condition: ""}
`

// journeyUC returns ucYAML extended with journey/variation/covers fields.
func journeyUC(covers string) string {
	return ucYAML + `journey: orders
variation: success
covers: [` + covers + `]
`
}

func writeInventory(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "branch-inventory.yaml"), []byte(inventoryYAML), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPersist_JourneyLandsInJourneyFolder(t *testing.T) {
	dir := setupFxDir(t)
	writeInventory(t, dir)

	if err := Persist([]byte(journeyUC("br-aaaaaaaaaaaa")), dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	want := filepath.Join(dir, "use-cases", "orders", "create-order.yaml")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("journeyed use case not at %s: %v", want, err)
	}
	flat := filepath.Join(dir, "use-cases", "create-order.yaml")
	if _, err := os.Stat(flat); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("flat copy must not exist for journeyed use case: %v", err)
	}
}

func TestPersist_UnknownBranchRefRejected(t *testing.T) {
	dir := setupFxDir(t)
	writeInventory(t, dir)

	err := Persist([]byte(journeyUC("br-eeeeeeeeeeee")), dir)
	if err == nil {
		t.Fatal("expected unknown_branch_ref failure, got nil")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.UnknownBranchRef {
		t.Fatalf("Kind = %q, want %q", pe.Kind, persisterror.UnknownBranchRef)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "use-cases", "orders", "create-order.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("use case written despite unknown branch ref: %v", statErr)
	}
}

func TestPersist_CoversWithoutInventoryIsMissingDependency(t *testing.T) {
	dir := setupFxDir(t)

	err := Persist([]byte(journeyUC("br-aaaaaaaaaaaa")), dir)
	if err == nil {
		t.Fatal("expected missing_dependency failure, got nil")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.MissingDependency {
		t.Fatalf("Kind = %q, want %q", pe.Kind, persisterror.MissingDependency)
	}
}

func TestPersist_UnknownVariationRejected(t *testing.T) {
	dir := setupFxDir(t)

	uc := ucYAML + "journey: orders\nvariation: sideways\n"
	err := Persist([]byte(uc), dir)
	if err == nil {
		t.Fatal("expected variation validation failure, got nil")
	}
	var pe *persisterror.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *persisterror.Error, got %T: %v", err, err)
	}
	if pe.Kind != persisterror.UnknownEnumValue {
		t.Fatalf("Kind = %q, want %q (err: %v)", pe.Kind, persisterror.UnknownEnumValue, err)
	}
}

func TestPersist_FlatLayoutStillWorks(t *testing.T) {
	dir := setupFxDir(t)

	if err := Persist([]byte(ucYAML), dir); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "use-cases", "create-order.yaml")); err != nil {
		t.Fatalf("flat use case not written: %v", err)
	}
}
