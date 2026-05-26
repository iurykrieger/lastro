package stack

import (
	"fmt"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/persisthelp"
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
	bumped, err := persisthelp.BumpSchemaVersion(targetPath, validated.SchemaVersion)
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

	if err := persisthelp.AtomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "stack-manifest",
			Message:    fmt.Sprintf("write %s: %v", targetPath, err),
		}
	}
	return nil
}
