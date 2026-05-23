// Package fixturestub is a test-only helper for the E4 (usecase) package.
// It builds a fixture.FixtureStore from a compact map[string]string of
// id → JSON payload, eagerly parsing each payload so the resolver's
// jsonpath drilling works the same as it would for fixtures loaded from
// disk. ContentType is hard-coded to "application/json".
package fixturestub

import (
	"encoding/json"

	"github.com/iurykrieger/lastro/internal/fixture"
)

// New builds a FixtureStore from id → JSON-payload strings. Each entry
// is parsed and stored as a fixture.Fixture with both raw Payload and
// the parsed structured value (the same shape the production loader
// would produce for application/json content).
//
// Panics on invalid JSON or duplicate ids — this is test-only code; loud
// failure is the right behavior.
func New(payloads map[string]string) fixture.FixtureStore {
	fixtures := make([]fixture.Fixture, 0, len(payloads))
	for id, raw := range payloads {
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			panic("fixturestub: id " + id + " has invalid JSON: " + err.Error())
		}
		fixtures = append(fixtures, fixture.Fixture{
			SchemaVersion: "1.0.0",
			ID:            id,
			Role:          fixture.RoleInput,
			ContentType:   "application/json",
			Payload:       []byte(raw),
			Parsed:        parsed,
		})
	}
	store, err := fixture.NewStore(fixtures...)
	if err != nil {
		panic("fixturestub: " + err.Error())
	}
	return store
}
