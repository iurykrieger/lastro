// Package aggregate owns the AggregateSignal entity — the terminal record
// emitted at the end of every sensor execution. It also owns the
// deterministic Rollup function that turns a slice of Signals plus
// execution metadata into the AggregateSignal.
//
// See docs/superpowers/specs/2026-05-23-e8-aggregate-signal-design.md
// and docs/harness-framework/E8-aggregate-signal.md for the design rationale.
package aggregate

import (
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

// TypeAggregate is the on-the-wire discriminator value distinguishing an
// AggregateSignal from a Signal in a shared JSON Lines stream.
const TypeAggregate = "aggregate"

// HealHint is re-exported from signalstub so callers of internal/aggregate
// can construct hints without importing the stub directly. When E7 lands,
// this alias points at internal/signal.HealHint.
type HealHint = signalstub.HealHint

// Locus is re-exported alongside HealHint.
type Locus = signalstub.Locus

// Signal is re-exported from signalstub so consumers outside this
// package can construct RollupInput.Signals without importing the
// stub package directly (Go internal-package rules forbid it).
type Signal = signalstub.Signal

// Evidence is re-exported alongside Signal.
type Evidence = signalstub.Evidence

// AggregateSignal is the terminal record emitted by every sensor run.
type AggregateSignal struct {
	SchemaVersion     string                  `json:"schema_version"`
	Type              string                  `json:"type"`
	SensorID          string                  `json:"sensor_id"`
	UseCaseID         string                  `json:"use_case_id"`
	Angle             enums.ValidationAngle   `json:"angle"`
	StartedAt         time.Time               `json:"started_at"`
	EndedAt           time.Time               `json:"ended_at"`
	TerminationReason enums.TerminationReason `json:"termination_reason"`
	Verdict           enums.Verdict           `json:"verdict"`
	Confidence        float64                 `json:"confidence"`
	Rollup            RollupCounts            `json:"rollup"`
	Completeness      *Completeness           `json:"completeness,omitempty"`
	HealHint          *HealHint               `json:"heal_hint,omitempty"`
}

// RollupCounts is the per-verdict tally for the sensor's signals.
type RollupCounts struct {
	TotalSignals      int `json:"total_signals"`
	PassCount         int `json:"pass_count"`
	WarnCount         int `json:"warn_count"`
	FailCount         int `json:"fail_count"`
	InconclusiveCount int `json:"inconclusive_count"`
}

// Completeness reports observation coverage for observational sensors.
// Always omitted (nil pointer) for non-observational kinds.
type Completeness struct {
	ExpectedObservations []string `json:"expected_observations"`
	MissingObservations  []string `json:"missing_observations"`
}

// RollupInput is the full set of inputs Rollup needs to produce an
// AggregateSignal. The runtime owns where these inputs come from; Rollup
// itself is pure.
type RollupInput struct {
	Signals              []signalstub.Signal
	SensorID             string
	UseCaseID            string
	Angle                enums.ValidationAngle
	Kind                 enums.SensorKind
	OutputType           enums.SignalOutputType
	StartedAt            time.Time
	EndedAt              time.Time
	TerminationReason    enums.TerminationReason
	ExpectedObservations []string // observational only; may be nil
	ObservedKeys         []string // observational only; keys actually seen
}
