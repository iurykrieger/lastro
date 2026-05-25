package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
)

// ErrMaxHealIterationsOutOfRange is returned by Load when a source
// validation-policy declares max_heal_iterations outside [0, 20].
var ErrMaxHealIterationsOutOfRange = errors.New("policy: max_heal_iterations out of range [0, 20]")

// Load parses a single ValidationPolicy from a YAML stream. The pipeline
// is read → YAML→JSON normalize → json.Unmarshal → JSON Schema validate →
// semantic validation. Both schema and semantic checks always run after a
// successful unmarshal so callers receive typed errors (e.g.
// ErrMaxHealIterationsOutOfRange) even when the JSON Schema also catches the
// violation.
func Load(r io.Reader) (*ValidationPolicy, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("policy: read: %w", err)
	}
	asJSON, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("policy: yaml-to-json: %w", err)
	}
	var p ValidationPolicy
	if err := json.Unmarshal(asJSON, &p); err != nil {
		return nil, fmt.Errorf("policy: deserialize: %w", err)
	}
	if p.SchemaVersion != SupportedSchemaVersion {
		return nil, fmt.Errorf("policy: schema_version %q not supported (want %q)", p.SchemaVersion, SupportedSchemaVersion)
	}
	var validationErrs []error
	if schemaErr := validateAgainstSchema(asJSON); schemaErr != nil {
		validationErrs = append(validationErrs, fmt.Errorf("schema validation: %w", schemaErr))
	}
	if semanticErr := validateSemantics(&p); semanticErr != nil {
		validationErrs = append(validationErrs, semanticErr)
	}
	if joined := errors.Join(validationErrs...); joined != nil {
		return nil, fmt.Errorf("policy: %w", joined)
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

// validateSemantics enforces loader rules 5 (applicable-angle matrix),
// 6 (disjoint lists), 7 (no duplicates within a list), and the
// MaxHealIterations range check (0..20).
func validateSemantics(p *ValidationPolicy) error {
	var errs []error
	for arch, block := range p.PerArchetype {
		errs = append(errs, checkApplicableAngles(arch, block)...)
		errs = append(errs, checkDuplicates(arch, block)...)
		errs = append(errs, checkDisjoint(arch, block)...)
	}
	if p.MaxHealIterations != nil {
		v := *p.MaxHealIterations
		if v < 0 || v > 20 {
			errs = append(errs, fmt.Errorf("%w: got %d", ErrMaxHealIterationsOutOfRange, v))
		}
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

func checkDuplicates(arch enums.Archetype, block ArchetypeBlock) []error {
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
		seen := make(map[enums.ValidationAngle]struct{}, len(l.angles))
		for _, a := range l.angles {
			if _, dup := seen[a]; dup {
				errs = append(errs, fmt.Errorf("archetype %q list %s: duplicate angle %q", arch, l.name, a))
				continue
			}
			seen[a] = struct{}{}
		}
	}
	return errs
}

func checkDisjoint(arch enums.Archetype, block ArchetypeBlock) []error {
	var errs []error
	pairs := []struct {
		aName, bName string
		a, b         []enums.ValidationAngle
	}{
		{"obligatory_angles", "optional_angles", block.Obligatory, block.Optional},
		{"obligatory_angles", "disabled_angles", block.Obligatory, block.Disabled},
		{"optional_angles", "disabled_angles", block.Optional, block.Disabled},
	}
	for _, p := range pairs {
		in := make(map[enums.ValidationAngle]struct{}, len(p.a))
		for _, a := range p.a {
			in[a] = struct{}{}
		}
		for _, b := range p.b {
			if _, ok := in[b]; ok {
				errs = append(errs, fmt.Errorf("archetype %q lists %s and %s overlap on angle %q", arch, p.aName, p.bName, b))
			}
		}
	}
	return errs
}
