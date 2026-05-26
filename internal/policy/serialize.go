package policy

import (
	"fmt"
	"sort"

	"sigs.k8s.io/yaml"
)

// MarshalYAML renders the EffectivePolicy as audit-friendly YAML. Output
// is deterministic: archetypes sorted alphabetically, angles sorted
// alphabetically within each list, empty lists emitted as []. No scope:
// field; resolved_from: is the discriminator that distinguishes a
// resolved view from a source policy.
//
// The output intentionally cannot be loaded back through Load. Source
// policies are human-authored; effective dumps are derived artifacts.
func (p *EffectivePolicy) MarshalYAML() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("policy: marshal effective: nil receiver")
	}
	type blockOut struct {
		Obligatory []string `json:"obligatory_angles" yaml:"obligatory_angles"`
		Optional   []string `json:"optional_angles"   yaml:"optional_angles"`
		Disabled   []string `json:"disabled_angles"   yaml:"disabled_angles"`
	}
	type docOut struct {
		SchemaVersion     string              `json:"schema_version"     yaml:"schema_version"`
		ResolvedFrom      []string            `json:"resolved_from"      yaml:"resolved_from"`
		InferentialFloor  float64             `json:"inferential_floor"  yaml:"inferential_floor"`
		MaxHealIterations int                 `json:"max_heal_iterations" yaml:"max_heal_iterations"`
		PerArchetype      map[string]blockOut `json:"per_archetype"      yaml:"per_archetype"`
	}

	doc := docOut{
		SchemaVersion:     p.SchemaVersion,
		ResolvedFrom:      append([]string{}, p.ResolvedFrom...),
		InferentialFloor:  p.InferentialFloor,
		MaxHealIterations: p.MaxHealIterations,
		PerArchetype:      map[string]blockOut{},
	}

	for arch, block := range p.PerArchetype {
		out := blockOut{
			Obligatory: []string{},
			Optional:   []string{},
			Disabled:   []string{},
		}
		for angle, status := range block {
			switch status {
			case StatusObligatory:
				out.Obligatory = append(out.Obligatory, string(angle))
			case StatusOptional:
				out.Optional = append(out.Optional, string(angle))
			case StatusDisabled:
				out.Disabled = append(out.Disabled, string(angle))
			}
		}
		sort.Strings(out.Obligatory)
		sort.Strings(out.Optional)
		sort.Strings(out.Disabled)
		doc.PerArchetype[string(arch)] = out
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("policy: marshal effective: %w", err)
	}
	return raw, nil
}
