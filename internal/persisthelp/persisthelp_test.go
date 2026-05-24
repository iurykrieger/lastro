package persisthelp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBumpPatch(t *testing.T) {
	cases := map[string]string{"1.0.0": "1.0.1", "2.3.4": "2.3.5"}
	for in, want := range cases {
		got, err := BumpPatch(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s -> %s, want %s", in, got, want)
		}
	}
	if _, err := BumpPatch("not-semver"); err == nil {
		t.Fatal("BumpPatch accepted non-semver")
	}
}

func TestBumpSchemaVersion_FileMissingReturnsInput(t *testing.T) {
	got, err := BumpSchemaVersion(filepath.Join(t.TempDir(), "missing.yaml"), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.0.0" {
		t.Fatalf("got %q, want 1.0.0", got)
	}
}

func TestBumpSchemaVersion_FilePresentBumpsItsVersion(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.yaml")
	if err := os.WriteFile(target, []byte("schema_version: 1.0.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := BumpSchemaVersion(target, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.0.5" {
		t.Fatalf("got %q, want 1.0.5", got)
	}
}

func TestAtomicWrite_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "c.yaml")
	if err := AtomicWrite(target, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}
