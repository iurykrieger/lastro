package skillio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepoRoot_FindsHarnessDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".harness"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := FindRepoRoot(nested)
	if err != nil {
		t.Fatalf("FindRepoRoot: %v", err)
	}
	// resolve symlinks because t.TempDir returns /tmp paths that may differ
	wantAbs, _ := filepath.EvalSymlinks(root)
	gotAbs, _ := filepath.EvalSymlinks(got)
	if gotAbs != wantAbs {
		t.Errorf("FindRepoRoot = %q, want %q", gotAbs, wantAbs)
	}
}

func TestFindRepoRoot_FallsBackToGoMod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := FindRepoRoot(nested)
	if err != nil {
		t.Fatalf("FindRepoRoot: %v", err)
	}
	wantAbs, _ := filepath.EvalSymlinks(root)
	gotAbs, _ := filepath.EvalSymlinks(got)
	if gotAbs != wantAbs {
		t.Errorf("FindRepoRoot = %q, want %q", gotAbs, wantAbs)
	}
}

func TestFindRepoRoot_NoMarker(t *testing.T) {
	root := t.TempDir()
	_, err := FindRepoRoot(root)
	if err == nil {
		t.Errorf("expected error when no .harness/ or go.mod present")
	}
}

func TestHarnessDir(t *testing.T) {
	got := HarnessDir("/repo")
	want := filepath.Join("/repo", ".harness")
	if got != want {
		t.Errorf("HarnessDir = %q, want %q", got, want)
	}
}
