package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// HarnessDirName is the conventional subdirectory containing detected
// use cases, fixtures, sensors, and runtime artifacts.
const HarnessDirName = ".harness"

// ErrNoHarnessDir is returned when neither the configured RepoRoot
// nor any ancestor contains a .harness/ directory.
var ErrNoHarnessDir = errors.New("harness: no .harness/ directory found")

// resolveRepoRoot returns the effective repo root in priority order:
//   1. cfg.RepoRoot (explicit flag)
//   2. HARNESS_REPO_ROOT env var
//   3. current working directory
//
// The returned path is absolute and cleaned.
func resolveRepoRoot(cfg *Config) (string, error) {
	candidate := cfg.RepoRoot
	if candidate == "" {
		candidate = os.Getenv("HARNESS_REPO_ROOT")
	}
	if candidate == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve repo root: %w", err)
		}
		candidate = cwd
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	return filepath.Clean(abs), nil
}

// resolveHarnessDir returns <repoRoot>/.harness. Errors if the path
// does not exist; callers can then exit 64 (EX_USAGE).
func resolveHarnessDir(repoRoot string) (string, error) {
	dir := filepath.Join(repoRoot, HarnessDirName)
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w at %s", ErrNoHarnessDir, dir)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s exists but is not a directory", dir)
	}
	return dir, nil
}

// resolvePolicyPath returns the validation-policy.yaml path:
//   1. cfg.Policy if set
//   2. HARNESS_POLICY env var
//   3. <harnessDir>/validation-policy.yaml
func resolvePolicyPath(cfg *Config, harnessDir string) string {
	if cfg.Policy != "" {
		return cfg.Policy
	}
	if env := os.Getenv("HARNESS_POLICY"); env != "" {
		return env
	}
	return filepath.Join(harnessDir, "validation-policy.yaml")
}
