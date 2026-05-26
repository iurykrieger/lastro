package skillio

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindRepoRoot walks up from cwd looking for the nearest ancestor that
// contains a .harness/ directory; if none is found, falls back to the
// nearest ancestor containing a go.mod file. Returns the absolute path
// or an error if neither marker is found before reaching the filesystem
// root.
func FindRepoRoot(cwd string) (string, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("skillio: abs(%q): %w", cwd, err)
	}

	// Pass 1: look for .harness/
	for dir := abs; ; {
		if info, err := os.Stat(filepath.Join(dir, ".harness")); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Pass 2: look for go.mod
	for dir := abs; ; {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("skillio: no .harness/ or go.mod found in %q or any parent", cwd)
}

// HarnessDir returns the absolute path to the .harness/ directory inside
// the given repo root.
func HarnessDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".harness")
}
