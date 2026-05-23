// Package signal implements the typed Signal record, a streaming JSON
// Lines parser, validation, and an encoder. Signals are emitted by
// sensors (Phase B) and consumed by the per-sensor signal collector and
// downstream aggregator (E8); this package owns only the contract, not
// the producers or consumers.
package signal

import (
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
)

// Signal is one record emitted by a sensor during execution — a verdict,
// evidence, and (on failure) a heal_hint. Signals travel as JSON Lines
// on sensor stdout. The canonical schema is schemas/signal.yaml;
// internal/signal/schema.yaml mirrors it.
type Signal struct {
	SchemaVersion string                `json:"schema_version"`
	SensorID      string                `json:"sensor_id"`
	UseCaseID     string                `json:"use_case_id"`
	Angle         enums.ValidationAngle `json:"angle"`
	EmittedAt     time.Time             `json:"emitted_at"`
	Verdict       enums.Verdict         `json:"verdict"`
	Confidence    float64               `json:"confidence"`
	Evidence      Evidence              `json:"evidence"`
	HealHint      *HealHint             `json:"heal_hint,omitempty"`
}

// Evidence is the open-shape evidence map. The schema documents three
// well-known keys (expected, actual, fixture_id) and permits any other
// key for sensor-specific data.
type Evidence map[string]any

// Expected returns the value of the "expected" key. ok is true iff the
// key is present in the map (regardless of its concrete type).
func (e Evidence) Expected() (any, bool) {
	v, ok := e["expected"]
	return v, ok
}

// Actual returns the value of the "actual" key. ok is true iff the
// key is present in the map (regardless of its concrete type).
func (e Evidence) Actual() (any, bool) {
	v, ok := e["actual"]
	return v, ok
}

// FixtureID returns the value of the "fixture_id" key as a string.
// ok is true iff the key is present AND its value is a string.
func (e Evidence) FixtureID() (string, bool) {
	v, ok := e["fixture_id"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

// HealHint compresses a failing signal into LLM-ingestible action. The
// schema requires it whenever verdict == fail; the JSON omitempty tag on
// Signal.HealHint preserves that semantic when the field is nil for
// passing or inconclusive signals.
type HealHint struct {
	Summary        string  `json:"summary"`
	SuggestedLocus []Locus `json:"suggested_locus,omitempty"`
	Rationale      string  `json:"rationale"`
}

// Locus is one pointer into the application source that the LLM may
// want to edit.
type Locus struct {
	Path   string `json:"path"`
	Symbol string `json:"symbol,omitempty"`
}
