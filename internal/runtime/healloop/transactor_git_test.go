package healloop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH; skipping git-transactor test")
	}
}

func mustExec(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func gitTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustExec(t, dir, "git", "init", "-q", "-b", "main")
	mustExec(t, dir, "git", "config", "user.email", "test@example.invalid")
	mustExec(t, dir, "git", "config", "user.name", "Test")
	return dir
}

func TestGitTransactor_RestoresViaStashApply(t *testing.T) {
	requireGit(t)
	dir := gitTempRepo(t)
	path := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(path, []byte("orig\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustExec(t, dir, "git", "add", "foo.txt")
	mustExec(t, dir, "git", "commit", "-q", "-m", "init")

	// Dirty the target file before snapshot to exercise stash creation.
	if err := os.WriteFile(path, []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tx := &gitTransactor{repoRoot: dir}
	handle, err := tx.Snapshot(context.Background(), []string{"foo.txt"})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Apply an edit on top of the snapshot.
	if err := os.WriteFile(path, []byte("healed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := handle.Revert(); err != nil {
		t.Fatalf("Revert: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "dirty\n" {
		t.Errorf("file content = %q, want %q (pre-snapshot dirty state restored)", got, "dirty\n")
	}
}

func TestGitTransactor_PreservesUnrelatedDirtyState(t *testing.T) {
	requireGit(t)
	dir := gitTempRepo(t)
	target := filepath.Join(dir, "target.txt")
	unrelated := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(target, []byte("orig\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte("orig\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustExec(t, dir, "git", "add", ".")
	mustExec(t, dir, "git", "commit", "-q", "-m", "init")

	// Dirty the unrelated file.
	if err := os.WriteFile(unrelated, []byte("UNRELATED-DIRTY\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tx := &gitTransactor{repoRoot: dir}
	handle, err := tx.Snapshot(context.Background(), []string{"target.txt"})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Touch target.
	if err := os.WriteFile(target, []byte("healed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := handle.Revert(); err != nil {
		t.Fatalf("Revert: %v", err)
	}

	got, err := os.ReadFile(unrelated)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "UNRELATED-DIRTY\n" {
		t.Errorf("unrelated content = %q, want %q (unrelated dirty state must survive)", got, "UNRELATED-DIRTY\n")
	}
	gotTarget, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotTarget) != "orig\n" {
		t.Errorf("target content = %q, want %q", gotTarget, "orig\n")
	}
}

func TestGitTransactor_Commit_DropsStashKeepsEdits(t *testing.T) {
	requireGit(t)
	dir := gitTempRepo(t)
	path := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(path, []byte("orig\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustExec(t, dir, "git", "add", "foo.txt")
	mustExec(t, dir, "git", "commit", "-q", "-m", "init")

	// Make the file dirty so Snapshot has something to stash.
	if err := os.WriteFile(path, []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tx := &gitTransactor{repoRoot: dir}
	handle, err := tx.Snapshot(context.Background(), []string{"foo.txt"})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	if err := os.WriteFile(path, []byte("healed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := handle.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "healed\n" {
		t.Errorf("file content = %q, want %q", got, "healed\n")
	}

	cmd := exec.Command("git", "-C", dir, "stash", "list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "harness-heal-") {
		t.Errorf("stash list still contains harness-heal entry:\n%s", out)
	}
}
