package healloop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileTransactor_RestoresOriginalBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.txt")
	original := []byte("hello\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	tx := &fileTransactor{repoRoot: dir}
	handle, err := tx.Snapshot(context.Background(), []string{"foo.txt"})
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the file.
	if err := os.WriteFile(path, []byte("BAD"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := handle.Revert(); err != nil {
		t.Fatalf("Revert: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("file content = %q, want %q", got, original)
	}
}

func TestFileTransactor_DeletesCreatedFiles_OnRevert(t *testing.T) {
	dir := t.TempDir()
	tx := &fileTransactor{repoRoot: dir}
	handle, err := tx.Snapshot(context.Background(), []string{"new.txt"})
	if err != nil {
		t.Fatal(err)
	}
	// Create the file post-snapshot.
	path := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := handle.Revert(); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file removed, got err=%v", err)
	}
}

func TestFileTransactor_Commit_IsNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(path, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	tx := &fileTransactor{repoRoot: dir}
	handle, err := tx.Snapshot(context.Background(), []string{"foo.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("NEW"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := handle.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "NEW" {
		t.Errorf("file content = %q, want %q (Commit must not revert)", got, "NEW")
	}
}

func TestFileTransactor_Apply_WritesAndDeletes(t *testing.T) {
	dir := t.TempDir()
	keepPath := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(keepPath, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	tx := &fileTransactor{repoRoot: dir}
	handle, err := tx.Snapshot(context.Background(), []string{"keep.txt", "new/deep/file.txt"})
	if err != nil {
		t.Fatal(err)
	}

	plan := EditPlan{Files: []EditFile{
		{Path: "keep.txt", Op: OpDelete},
		{Path: "new/deep/file.txt", Op: OpWrite, Content: "created"},
	}}
	if err := handle.Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if _, err := os.Stat(keepPath); !os.IsNotExist(err) {
		t.Errorf("expected keep.txt deleted, err=%v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "new/deep/file.txt"))
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if string(got) != "created" {
		t.Errorf("new file content = %q, want %q", got, "created")
	}
}

func TestFileTransactor_Apply_RejectsUnknownOp(t *testing.T) {
	dir := t.TempDir()
	tx := &fileTransactor{repoRoot: dir}
	handle, err := tx.Snapshot(context.Background(), []string{"foo.txt"})
	if err != nil {
		t.Fatal(err)
	}
	plan := EditPlan{Files: []EditFile{{Path: "foo.txt", Op: EditOp("bogus")}}}
	if err := handle.Apply(plan); err == nil {
		t.Fatalf("expected error for unknown Op, got nil")
	}
}
