// Package policy loads, validates, resolves, and serializes ValidationPolicy
// YAML files. It is a pure library: no filesystem awareness beyond the
// io.Reader passed to Load, no environment lookups, no network. Multi-tier
// upstream composition (org → division → framework default) is delegated
// to callers, who pre-merge a chain into a single *ValidationPolicy before
// calling Resolve.
package policy

import "github.com/iurykrieger/lastro/internal/enums"

// SupportedSchemaVersion pins the schema_version that EffectivePolicy
// values declare. Source files whose schema_version differs are accepted
// only if they match (loader rule 1).
const SupportedSchemaVersion = "1.0.0"

// DefaultInferentialFloor is applied by Resolve when no scope sets
// inferential_floor. Plan §10.3 default.
const DefaultInferentialFloor = 0.7

// Scope is the closed two-value enum for a source ValidationPolicy.
// EffectivePolicy has no Scope — it is the merged result of one or both.
type Scope string

const (
	ScopeGlobal Scope = "global"
	ScopeLocal  Scope = "local"
)

// ValidationPolicy mirrors the on-disk YAML form. Human-authored. The
// canonical schema is schemas/validation-policy.yaml.
type ValidationPolicy struct {
	SchemaVersion string                             `json:"schema_version" yaml:"schema_version"`
	Scope         Scope                              `json:"scope"          yaml:"scope"`
	PerArchetype  map[enums.Archetype]ArchetypeBlock `json:"per_archetype"  yaml:"per_archetype"`
	// InferentialFloor is the minimum confidence below which an inferential
	// sensor's verdict is treated as inconclusive by the use-case aggregator.
	// Nullable so callers can distinguish "field omitted" (nil) from "explicitly
	// 0.0" (non-nil pointer to 0.0). Resolve fills in DefaultInferentialFloor
	// only when every source scope leaves it nil.
	InferentialFloor *float64 `json:"inferential_floor,omitempty" yaml:"inferential_floor,omitempty"`
}

// ArchetypeBlock is a per-archetype declaration of which angles are
// obligatory, optional, or disabled. Within a single block the three
// lists are pairwise disjoint (loader rule 6).
type ArchetypeBlock struct {
	Obligatory []enums.ValidationAngle `json:"obligatory_angles" yaml:"obligatory_angles"`
	Optional   []enums.ValidationAngle `json:"optional_angles"   yaml:"optional_angles"`
	Disabled   []enums.ValidationAngle `json:"disabled_angles"   yaml:"disabled_angles"`
}

// EffectivePolicy is the resolved view of one or both source scopes.
// Derived; not human-authored. Per-(archetype, angle) map form lets
// Resolve express overrides without restating the archetype block.
type EffectivePolicy struct {
	SchemaVersion string                                                    `json:"schema_version" yaml:"schema_version"`
	ResolvedFrom  []string                                                  `json:"resolved_from"  yaml:"resolved_from"`
	PerArchetype  map[enums.Archetype]map[enums.ValidationAngle]AngleStatus `json:"-"              yaml:"-"`
	// InferentialFloor is the resolved floor — always populated post-Resolve.
	InferentialFloor float64 `json:"inferential_floor" yaml:"inferential_floor"`
}

// AngleStatus is one of obligatory / optional / disabled. The zero value
// ("") represents "unset / no opinion" — used internally by Resolve and
// returned by Status when a (archetype, angle) pair has no policy coverage.
type AngleStatus string

const (
	StatusObligatory AngleStatus = "obligatory"
	StatusOptional   AngleStatus = "optional"
	StatusDisabled   AngleStatus = "disabled"
)
