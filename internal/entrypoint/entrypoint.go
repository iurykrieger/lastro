// Package entrypoint defines the EntryPoint type embedded in UseCase.
// This file is the E4-driven minimal surface; E3 will replace
// Spec map[string]any with discriminated unions and per-archetype typed
// accessors.
package entrypoint

import "github.com/iurykrieger/lastro/internal/enums"

// EntryPoint is an archetype-typed observable surface. The Spec shape is
// determined by Archetype (see schemas/entry-point.yaml).
type EntryPoint struct {
	ID        string          `yaml:"id"          json:"id"`
	Archetype enums.Archetype `yaml:"archetype"   json:"archetype"`
	Spec      map[string]any  `yaml:"spec"        json:"spec"`
}

// SpecField reads a single field from Spec. Returns (zero, false) if the
// field is absent. E3 will replace this with archetype-aware typed
// accessors.
func (e EntryPoint) SpecField(name string) (any, bool) {
	v, ok := e.Spec[name]
	return v, ok
}

// Label returns the short identifier "<archetype>:<id>" used when a bare
// {{entry_points.<id>}} ref is rendered as a string.
func (e EntryPoint) Label() string {
	return string(e.Archetype) + ":" + e.ID
}
