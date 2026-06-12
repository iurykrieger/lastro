// Package sensor loads, validates, and orders the dynamically-generated
// Sensor units that validate (UseCase × ValidationAngle) pairs. A Sensor
// composes a top-level toolbox subset (StackComponent ids it draws from)
// and an ordered list of Steps. Each step is a discriminated union: either
// a run-step (shell command) or a uses-step (composed primitive id), with
// fixtures referenced via ${{ fixtures.<id> }} interpolation inside run
// commands and with values.
//
// The canonical schema lives at schemas/sensor.yaml and is consumed via
// the embedded schemas.FS. This package never edits sensors — generation
// is a Phase B concern (the /create-sensors skill).
package sensor

import "github.com/iurykrieger/lastro/internal/enums"

// Sensor is the in-memory representation of a loaded sensor YAML.
type Sensor struct {
	SchemaVersion string                 `json:"schema_version"`
	ID            string                 `json:"id"`
	Scope         enums.SensorScope      `json:"scope,omitempty"`
	UseCaseID     string                 `json:"use_case_id,omitempty"`
	Angle         enums.ValidationAngle  `json:"angle"`
	Kind          enums.SensorKind       `json:"kind"`
	Nature        enums.SensorNature     `json:"nature"`
	OutputType    enums.SignalOutputType `json:"output_type"`
	// ObserveWindow optionally bounds how long an attaching observational
	// sensor watches a shared service's signal stream before rolling up on
	// completeness. A Go duration string ("45s"); empty means runtime default.
	ObserveWindow string                 `json:"observe_window,omitempty"`
	Uses          []string               `json:"uses"`                 // StackComponent ids (grounding invariant 1)
	DependsOn     []string               `json:"depends_on,omitempty"` // Sensor ids (optional)
	Inputs        map[string]InputSpec   `json:"inputs,omitempty"`
	Outputs       map[string]OutputSpec  `json:"outputs,omitempty"`
	// Env declares ambient environment variables the sensor's recipes read
	// (secrets, connection strings). The runtime enforces required entries
	// pre-spawn; on a composed primitive a consumer step's env: injection
	// also satisfies the requirement.
	Env map[string]EnvSpec `json:"env,omitempty"`
	// SignalMatches declares regex matchers applied to every stdout/stderr line.
	// Each match synthesizes a Signal (see internal/runtime/executor). Valid on
	// both assertion and observational sensors.
	SignalMatches []SignalMatch `json:"signal_matches,omitempty"`
	Steps         []Step        `json:"steps"`
}

// SignalMatch maps a regex over a sensor's output lines to a synthesized Signal.
// Verdict defaults to pass and Confidence to 1 when unset (Confidence is a
// pointer to distinguish unset from 0; the default of 1 is applied by the
// consumer when the pointer is nil — the loader does not materialize it).
// Expected is only valid on pass matchers (enforced at load).
// HealHint is required-by-effect for fail/warn.
type SignalMatch struct {
	Key        string         `json:"key"`
	Pattern    string         `json:"pattern"`
	Verdict    enums.Verdict  `json:"verdict,omitempty"`
	Confidence *float64       `json:"confidence,omitempty"`
	Expected   bool           `json:"expected,omitempty"`
	HealHint   *MatchHealHint `json:"heal_hint,omitempty"`
}

// MatchHealHint is the heal-hint a fail/warn matcher attaches to its signal.
// Local type to avoid a sensor->signal package dependency.
type MatchHealHint struct {
	Summary   string `json:"summary"`
	Rationale string `json:"rationale"`
}

// Step is one step of a Sensor. Exactly one of Run / Uses is set
// (enforced by the schema oneOf and by validateIntrinsic).
//   - Run-step:  Run is the shell command; With is empty.
//   - Uses-step: Uses is a core primitive's id; With binds its inputs.
//
// Fixtures are referenced via ${{ fixtures.<id> }} interpolation inside
// Run and With values (grounding invariant 2).
type Step struct {
	ID   string            `json:"id"`
	Run  string            `json:"run,omitempty"`
	Uses string            `json:"uses,omitempty"`
	With map[string]string `json:"with,omitempty"`
	// Env is the per-step environment injection map (issue #49). Values
	// support ${{ }} interpolation and are resolved by the executor at
	// spawn time, never inlined into script text.
	Env map[string]string `json:"env,omitempty"`
}

// InputSpec declares a composable input on a primitive sensor.
type InputSpec struct {
	Required    bool   `json:"required,omitempty"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
	HasDefault  bool   `json:"-"` // set by loader: distinguishes "" default from absent
}

// OutputSpec declares a re-exported output on a primitive sensor.
type OutputSpec struct {
	From        string `json:"from"`
	Description string `json:"description,omitempty"`
}

// EnvSpec declares one ambient environment variable a sensor's recipes
// read. Required is a pointer so the zero value distinguishes "unset"
// (defaults to true) from an explicit false.
type EnvSpec struct {
	Required    *bool  `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

// IsRequired reports whether the variable must be present and non-empty
// in the merged host+env_file view before any step spawns.
// Zero-value caveat: indexing a nil or empty map with a missing key yields
// the zero EnvSpec (Required == nil), so IsRequired returns true. Callers
// must check map presence before indexing by name to avoid a false positive.
func (e EnvSpec) IsRequired() bool { return e.Required == nil || *e.Required }

// UseCaseFixtureOwnership is the seam between this package and the
// authoritative source of "which fixtures does use case X own."
// Production code wires it over *usecase.UseCase.FixtureIDs; the
// heal loop (which may not have a full UseCase in scope) can wire
// it over fixture.Store.FixturesForUseCase. A nil return is treated
// as an empty owned-set by ValidateAgainstFixtures.
type UseCaseFixtureOwnership interface {
	OwnedFixtureIDs(useCaseID string) []string
}
