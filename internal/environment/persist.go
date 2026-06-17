// internal/environment/persist.go
package environment

import (
	"fmt"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/persisthelp"
)

const modelFilename = "environment-model.yaml"

// ValidateGrounding asserts every provided_by pointer resolves to a fact the
// parser actually extracted — the anti-hallucination guard.
func ValidateGrounding(m EnvironmentModel, f RawFacts) error {
	check := func(node string, p ProvidedBy) error {
		if _, ok := f.Resolve(p); !ok {
			return fmt.Errorf("node %q: provided_by {file:%q, path:%q} does not resolve to any parsed fact", node, p.File, p.Path)
		}
		return nil
	}
	if err := check("application", m.Application.ProvidedBy); err != nil {
		return err
	}
	for name, d := range m.Dependencies {
		if err := check(name, d.ProvidedBy); err != nil {
			return err
		}
	}
	for _, s := range m.Setup {
		if err := check(s.ID, s.ProvidedBy); err != nil {
			return err
		}
	}
	return nil
}

// Persist validates an LLM-emitted environment model (schema + edges/cycle +
// grounding against the parser's RawFacts), patch-bumps schema_version, and
// atomically writes it to <harnessDir>/environment-model.yaml. Returns a
// *persisterror.Error on any validation failure; nothing is written on error.
func Persist(modelContent, factsContent []byte, harnessDir string) error {
	model, err := LoadBytes(modelContent)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: err.Error()}
	}

	var facts RawFacts
	if err := yaml.Unmarshal(factsContent, &facts); err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: fmt.Sprintf("unmarshal facts: %v", err)}
	}
	if err := ValidateGrounding(model, facts); err != nil {
		return &persisterror.Error{Kind: persisterror.Grounding, EntityType: "environment-model", Message: err.Error()}
	}

	targetPath := filepath.Join(harnessDir, modelFilename)
	bumped, err := persisthelp.BumpSchemaVersion(targetPath, model.SchemaVersion)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: fmt.Sprintf("schema_version bump: %v", err)}
	}
	model.SchemaVersion = bumped

	out, err := yaml.Marshal(model)
	if err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: fmt.Sprintf("marshal: %v", err)}
	}
	if err := persisthelp.AtomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "environment-model", Message: fmt.Sprintf("write %s: %v", targetPath, err)}
	}
	return nil
}
