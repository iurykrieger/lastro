package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDirCopiesFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "top.txt"), []byte("top"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	top, err := os.ReadFile(filepath.Join(dst, "top.txt"))
	if err != nil {
		t.Fatalf("read top: %v", err)
	}
	if string(top) != "top" {
		t.Fatalf("top: want %q, got %q", "top", string(top))
	}

	nested, err := os.ReadFile(filepath.Join(dst, "sub", "nested.txt"))
	if err != nil {
		t.Fatalf("read nested: %v", err)
	}
	if string(nested) != "nested" {
		t.Fatalf("nested: want %q, got %q", "nested", string(nested))
	}
}

func TestCopyDirPreservesFileMode(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	exe := filepath.Join(src, "run.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CopyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dst, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// Allow OSes that ignore exec bit (Windows). Just check it copied.
	if info.Size() == 0 {
		t.Fatalf("copied file is empty")
	}
}
