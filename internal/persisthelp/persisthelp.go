// Package persisthelp holds the small file-write primitives every entity
// Persist function uses: semver patch-bump against an on-disk target,
// and atomic write via temp-file + rename.
package persisthelp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// BumpSchemaVersion reads targetPath's schema_version (if the file
// exists), patch-increments it, and returns the new value. If the file
// doesn't exist, returns input unchanged.
func BumpSchemaVersion(targetPath, input string) (string, error) {
	existing, err := os.ReadFile(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		return input, nil
	}
	if err != nil {
		return "", err
	}
	var head struct {
		SchemaVersion string `json:"schema_version" yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(existing, &head); err != nil {
		return "", fmt.Errorf("parse existing schema_version: %w", err)
	}
	if head.SchemaVersion == "" {
		return input, nil
	}
	return BumpPatch(head.SchemaVersion)
}

// BumpPatch increments the patch component of a semver string. Major
// and minor are preserved verbatim.
func BumpPatch(v string) (string, error) {
	var maj, min, patch int
	n, err := fmt.Sscanf(v, "%d.%d.%d", &maj, &min, &patch)
	if err != nil || n != 3 {
		return "", fmt.Errorf("not a semver: %q", v)
	}
	return fmt.Sprintf("%d.%d.%d", maj, min, patch+1), nil
}

// AtomicWrite writes content to <targetPath>.tmp then renames over the
// target. Ensures any reader sees either the prior content or the new
// content, never a partial file. Creates parent directories as needed.
func AtomicWrite(targetPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, targetPath); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup; ignore error
		return err
	}
	return nil
}
