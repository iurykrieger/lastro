// Package fixture loads, validates, and serves concrete I/O fixtures that
// prove a use case's behavior. A Fixture is a typed payload (request body,
// CLI args, event message, expected response, etc.) belonging to exactly
// one UseCase but referenceable from many sensors across many angles.
//
// The canonical schema is schemas/fixture.yaml at the repo root; an
// embedded copy at schema.yaml is kept byte-equal by drift_test.go.
package fixture

// Role is the canonical fixture role. Mirrors the closed enum in
// schemas/enums/fixture-roles.yaml.
type Role string

const (
	RoleInput              Role = "input"
	RoleExpectedOutput     Role = "expected-output"
	RoleExpectedSideEffect Role = "expected-side-effect"
)

// Channel is the binding channel for a fixture — how the payload reaches
// the application's surface. Mirrors the closed enum in schemas/fixture.yaml.
type Channel string

const (
	ChannelHTTP    Channel = "http"
	ChannelCLIArgs Channel = "cli-args"
	ChannelEvent   Channel = "event"
	ChannelStdout  Channel = "stdout"
	ChannelLogLine Channel = "log-line"
	ChannelDBRow   Channel = "db-row"
)

// SourceRef is provenance metadata — a pointer into the source code
// from which a fixture was reverse-engineered. Pointers only; no
// embedded code or runtime semantics.
type SourceRef struct {
	Path   string `json:"path"`
	Symbol string `json:"symbol,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Binding describes how a fixture's payload connects to the application's
// observable surface. Optional in the schema; nil when omitted.
type Binding struct {
	Channel  Channel        `json:"channel"`
	Selector map[string]any `json:"selector,omitempty"`
}

// Fixture is a single loaded, parsed, schema-validated fixture.
//
// Payload is the raw bytes from the YAML payload field (UTF-8). Parsed is
// the eager structured-payload parse: non-nil for application/json,
// application/yaml, application/xml (and suffix variants), nil otherwise.
// Consumers needing typed access drill into Parsed; consumers shipping
// the payload over the wire use Payload directly.
type Fixture struct {
	SchemaVersion string      `json:"schema_version"`
	ID            string      `json:"id"`
	UseCaseID     string      `json:"use_case_id"`
	Role          Role        `json:"role"`
	ContentType   string      `json:"content_type"`
	Payload       []byte      `json:"-"` // populated from the YAML string field, not via JSON unmarshal
	Parsed        any         `json:"-"` // eager parse result; nil when unstructured
	Binding       *Binding    `json:"binding,omitempty"`
	SourceRefs    []SourceRef `json:"source_refs,omitempty"`
}

// FixtureStore is the seam E4 (template resolver) and E6 (sensor grounding
// validator) bind against. The concrete *Store in this package satisfies it.
type FixtureStore interface {
	LookupFixture(id string) (Fixture, bool)
	FixturesForUseCase(useCaseID string) []Fixture
	All() []Fixture
}
