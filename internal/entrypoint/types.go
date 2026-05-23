// Package entrypoint owns the EntryPoint data type embedded inside UseCase.
// The package provides the YAML loader (which delegates archetype-specific
// validation to the canonical JSON Schema at schemas/entry-point.yaml),
// minimal accessors used by the UseCase template resolver, and golden
// per-archetype examples.
package entrypoint

import "github.com/iurykrieger/lastro/internal/enums"

// EntryPoint is an archetype-typed observable surface. The Spec shape is
// determined by Archetype; see schemas/entry-point.yaml for the per-
// archetype required fields and inline-enum constraints. Spec values are
// the raw types yielded by yaml-to-json normalization (string, float64,
// bool, []any, map[string]any).
type EntryPoint struct {
	ID        string          `json:"id"        yaml:"id"`
	Archetype enums.Archetype `json:"archetype" yaml:"archetype"`
	Spec      map[string]any  `json:"spec"      yaml:"spec"`
}
