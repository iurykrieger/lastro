package fixture

import (
	"fmt"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/persisthelp"
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
	bumped, err := persisthelp.BumpSchemaVersion(targetPath, fx.SchemaVersion)
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
	if err := persisthelp.AtomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "fixture",
			EntityID:   fx.ID,
			Message:    fmt.Sprintf("write %s: %v", targetPath, err),
		}
	}
	return nil
}

