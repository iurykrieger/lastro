package fixture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/persisterror"
)

// Persist validates an LLM-emitted fixture YAML, patch-bumps its
// schema_version against any prior on-disk version, and atomically
// writes it to <harnessDir>/fixtures/<id>.yaml. Returns a
// *persisterror.Error on validation failure; nothing is written when an
// error is returned.
//
// We re-marshal from a generic map (not the typed Fixture struct) so the
// payload field survives the round-trip — Fixture.Payload is `json:"-"`,
// so yaml.Marshal on the typed struct would drop it.
func Persist(content []byte, harnessDir string) error {
	// Step 1: validate via the typed loader.
	fx, err := LoadFixtureBytes(content)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "fixture",
			Message:    err.Error(),
		}
	}

	// Step 2: parse the original content into a generic map so we can
	// mutate schema_version and re-marshal without losing fields the
	// typed struct doesn't round-trip (e.g., payload).
	var raw map[string]any
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "fixture",
			EntityID:   fx.ID,
			Message:    fmt.Sprintf("re-parse for bump: %v", err),
		}
	}

	// Step 3: bump schema_version.
	targetPath := filepath.Join(harnessDir, "fixtures", fx.ID+".yaml")
	bumped, err := bumpSchemaVersion(targetPath, fx.SchemaVersion)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "fixture",
			EntityID:   fx.ID,
			Message:    fmt.Sprintf("schema_version bump: %v", err),
		}
	}
	raw["schema_version"] = bumped

	// Step 4: marshal and atomic write.
	out, err := yaml.Marshal(raw)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "fixture",
			EntityID:   fx.ID,
			Message:    fmt.Sprintf("marshal: %v", err),
		}
	}
	if err := atomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "fixture",
			EntityID:   fx.ID,
			Message:    fmt.Sprintf("write %s: %v", targetPath, err),
		}
	}
	return nil
}

// --- helpers inline-copied from internal/stack/persist.go ---
// Per CLAUDE.md rule 3, extract to a shared package when a third caller
// appears (which is Phase 5's usecase.Persist).

// bumpSchemaVersion reads the existing target file (if any), parses its
// schema_version, and returns the patch-incremented value. If the target
// doesn't exist, returns input unchanged (the LLM-supplied initial
// version).
func bumpSchemaVersion(targetPath, input string) (string, error) {
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
	return bumpPatch(head.SchemaVersion)
}

// bumpPatch increments the patch component of a semver string. Major
// and minor are preserved verbatim.
func bumpPatch(v string) (string, error) {
	var maj, min, patch int
	n, err := fmt.Sscanf(v, "%d.%d.%d", &maj, &min, &patch)
	if err != nil || n != 3 {
		return "", fmt.Errorf("not a semver: %q", v)
	}
	return fmt.Sprintf("%d.%d.%d", maj, min, patch+1), nil
}

// atomicWrite writes content to <targetPath>.tmp then renames over the
// target. Ensures any reader sees either the prior content or the new
// content, never a partial file.
func atomicWrite(targetPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, targetPath)
}
