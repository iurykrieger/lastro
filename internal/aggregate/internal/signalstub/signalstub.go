// Package signalstub vendors the minimal Signal and HealHint types that
// internal/aggregate needs while E7 (internal/signal) is being developed
// in parallel. When E7 lands, change every import of this package to
// "internal/signal" and delete this directory.
//
// The shapes here mirror plan.md §4.5 and the canonical schemas/signal.yaml
// JSON Schema. They intentionally have no parser/validator logic — that is
// E7's responsibility.
package signalstub

import (
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
)

// Locus identifies a code location the LLM should consider editing.
type Locus struct {
	Path   string `json:"path"`
	Symbol string `json:"symbol,omitempty"`
}

// HealHint is the LLM-actionable instruction attached to non-pass signals
// and aggregates. Required when Verdict is warn or fail.
type HealHint struct {
	Summary        string  `json:"summary"`
	SuggestedLocus []Locus `json:"suggested_locus,omitempty"`
	Rationale      string  `json:"rationale"`
}

// Signal is one record emitted by a sensor during execution. Single-shot
// sensors emit exactly one Signal followed by one AggregateSignal; stream
// sensors emit many.
type Signal struct {
	SchemaVersion string                `json:"schema_version"`
	SensorID      string                `json:"sensor_id"`
	UseCaseID     string                `json:"use_case_id"`
	Angle         enums.ValidationAngle `json:"angle"`
	EmittedAt     time.Time             `json:"emitted_at"`
	Verdict       enums.Verdict         `json:"verdict"`
	Confidence    float64               `json:"confidence"`
	Evidence      map[string]any        `json:"evidence,omitempty"`
	HealHint      *HealHint             `json:"heal_hint,omitempty"`
}
