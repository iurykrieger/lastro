package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRepoRoot_FlagWins(t *testing.T) {
	cfg := &Config{RepoRoot: "/tmp/explicit"}
	t.Setenv("HARNESS_REPO_ROOT", "/tmp/envvar")

	got, err := resolveRepoRoot(cfg)
	if err != nil {
		t.Fatalf("resolveRepoRoot: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean("/tmp/explicit") {
		t.Errorf("got %q, want /tmp/explicit", got)
	}
}

func TestResolveRepoRoot_EnvFallback(t *testing.T) {
	t.Setenv("HARNESS_REPO_ROOT", "/tmp/envvar")
	cfg := &Config{}

	got, err := resolveRepoRoot(cfg)
	if err != nil {
		t.Fatalf("resolveRepoRoot: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean("/tmp/envvar") {
		t.Errorf("got %q, want /tmp/envvar", got)
	}
}

func TestResolveHarnessDir_Missing(t *testing.T) {
	tmp := t.TempDir()
	_, err := resolveHarnessDir(tmp)
	if err == nil {
		t.Fatalf("expected ErrNoHarnessDir, got nil")
	}
}

func TestResolveHarnessDir_Found(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, HarnessDirName), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := resolveHarnessDir(tmp)
	if err != nil {
		t.Fatalf("resolveHarnessDir: %v", err)
	}
	want := filepath.Join(tmp, HarnessDirName)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolvePolicyPath(t *testing.T) {
	t.Run("flag wins", func(t *testing.T) {
		cfg := &Config{Policy: "/opt/policy.yaml"}
		got := resolvePolicyPath(cfg, "/tmp/.harness")
		if got != "/opt/policy.yaml" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("env fallback", func(t *testing.T) {
		t.Setenv("HARNESS_POLICY", "/opt/env.yaml")
		got := resolvePolicyPath(&Config{}, "/tmp/.harness")
		if got != "/opt/env.yaml" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("default", func(t *testing.T) {
		// Clear any env from the test parent.
		t.Setenv("HARNESS_POLICY", "")
		got := resolvePolicyPath(&Config{}, "/tmp/.harness")
		want := filepath.Join("/tmp/.harness", "validation-policy.yaml")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
