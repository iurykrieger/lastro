package entrypoint

import (
	"encoding/json"
	"fmt"
)

// SpecField looks up a single key in the spec map. Returns the raw value and
// true if present; nil and false otherwise. E4's template resolver uses this
// for {{entry_points.<id>.spec.<key>}} resolution. The returned `any` is
// whatever JSON yielded from yaml-to-json normalization (string, float64,
// bool, []any, map[string]any).
func (e EntryPoint) SpecField(name string) (any, bool) {
	v, ok := e.Spec[name]
	return v, ok
}

// Label renders the EntryPoint as the compact "<archetype>:<id>" form used
// in human-facing log lines and as the fallback rendering for a bare
// {{entry_points.<id>}} reference (no spec field). On a zero-value
// EntryPoint the result is the literal ":" — callers must not assume the
// label parses as a valid archetype:id pair.
func (e EntryPoint) Label() string {
	return string(e.Archetype) + ":" + e.ID
}

// Validate runs JSON Schema validation against the receiver. The loader
// already validates during LoadEntryPoint; this method is for EntryPoints
// constructed in code (e.g., by tests or by E4's template resolver).
func (e EntryPoint) Validate() error {
	asJSON, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("entrypoint: marshal for validation: %w", err)
	}
	return validateAgainstSchema(asJSON)
}
