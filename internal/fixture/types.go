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
