package stack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/persisterror"
)

const manifestFilename = "stack-manifest.yaml"

// Persist validates an LLM-emitted stack-manifest YAML, enriches it with
// applicable_angles derived from the archetype, patch-bumps its
// schema_version against any prior on-disk version, and atomically writes
// it to <harnessDir>/stack-manifest.yaml. Returns a *persisterror.Error
// on validation failure; nothing is written when an error is returned.
func Persist(content []byte, harnessDir string) error {
	// First parse + schema-validate. Validate is intentionally run twice
	// (once here without enrichment, once after enrichment in the
	// canonical loader) because enrichment overwrites a missing or wrong
	// LLM-supplied applicable_angles with the canonical list. We allow
	// the LLM to omit applicable_angles; if it supplies a value, it must
	// match what we'd inject anyway. Strategy: unmarshal first, derive
	// applicable_angles from archetype, then run the full validator.
	var m StackManifest
	if err := yaml.Unmarshal(content, &m); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "stack-manifest",
			Message:    fmt.Sprintf("unmarshal: %v", err),
		}
	}

	// Enrichment: stamp ApplicableAngles from the archetype's canonical
	// list. Required before Validate so the applicable_angles-match check
	// passes regardless of what the LLM emitted in that field.
	m.ApplicableAngles = enums.ApplicableAngles[m.Archetype]

	// Re-marshal then validate via the full LoadBytes pipeline so we get
	// schema + invariant checks consistently.
	enriched, err := yaml.Marshal(m)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "stack-manifest",
			Message:    fmt.Sprintf("marshal enriched: %v", err),
		}
	}
	validated, err := LoadBytes(enriched)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "stack-manifest",
			Message:    err.Error(),
		}
	}

	// Schema-version bump.
	targetPath := filepath.Join(harnessDir, manifestFilename)
	bumped, err := bumpSchemaVersion(targetPath, validated.SchemaVersion)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "stack-manifest",
			Message:    fmt.Sprintf("schema_version bump: %v", err),
		}
	}
	validated.SchemaVersion = bumped

	out, err := yaml.Marshal(validated)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "stack-manifest",
			Message:    fmt.Sprintf("marshal: %v", err),
		}
	}

	if err := atomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "stack-manifest",
			Message:    fmt.Sprintf("write %s: %v", targetPath, err),
		}
	}
	return nil
}

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
