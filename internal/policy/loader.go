package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
)

// Load parses a single ValidationPolicy from a YAML stream. The pipeline
// is read → YAML→JSON normalize → JSON Schema validate → json.Unmarshal →
// semantic validation. Semantic checks (applicable-angle matrix, disjoint
// lists) live in validateSemantics.
func Load(r io.Reader) (*ValidationPolicy, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("policy: read: %w", err)
	}
	asJSON, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("policy: yaml-to-json: %w", err)
	}
	if err := validateAgainstSchema(asJSON); err != nil {
		return nil, fmt.Errorf("policy: schema validation: %w", err)
	}
	var p ValidationPolicy
	if err := json.Unmarshal(asJSON, &p); err != nil {
		return nil, fmt.Errorf("policy: deserialize: %w", err)
	}
	if p.SchemaVersion != SupportedSchemaVersion {
		return nil, fmt.Errorf("policy: schema_version %q not supported (want %q)", p.SchemaVersion, SupportedSchemaVersion)
	}
	if err := validateSemantics(&p); err != nil {
		return nil, fmt.Errorf("policy: semantic validation: %w", err)
	}
	return &p, nil
}

func validateAgainstSchema(jsonDoc []byte) error {
	s, err := compiledSchema()
	if err != nil {
		return err
	}
	var instance any
	if err := json.Unmarshal(jsonDoc, &instance); err != nil {
		return fmt.Errorf("decode instance: %w", err)
	}
	return s.Validate(instance)
}

// validateSemantics enforces loader rules 5 (applicable-angle matrix) and
// 6 (disjoint lists). Filled in by Tasks 6 and 7.
func validateSemantics(p *ValidationPolicy) error {
	var errs []error
	for arch, block := range p.PerArchetype {
		errs = append(errs, checkApplicableAngles(arch, block)...)
	}
	return errors.Join(errs...)
}

func checkApplicableAngles(arch enums.Archetype, block ArchetypeBlock) []error {
	var errs []error
	lists := []struct {
		name   string
		angles []enums.ValidationAngle
	}{
		{"obligatory_angles", block.Obligatory},
		{"optional_angles", block.Optional},
		{"disabled_angles", block.Disabled},
	}
	for _, l := range lists {
		for _, a := range l.angles {
			if !enums.Applies(arch, a) {
				errs = append(errs, fmt.Errorf("archetype %q list %s: angle %q is not applicable", arch, l.name, a))
			}
		}
	}
	return errs
}
