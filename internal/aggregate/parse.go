package aggregate

import (
	"encoding/json"
	"fmt"
	"io"
)

// ParseAggregate reads a single JSON-encoded AggregateSignal from r,
// validates it against both the embedded JSON Schema and the hand-written
// Go-level rules, and returns the typed record on success.
//
// The reader is expected to contain exactly one JSON record (the terminal
// JSON Lines record of a sensor's stdout). Splitting a multi-line stream
// into Signal records plus the terminal AggregateSignal is the
// responsibility of the sensor runtime (Phase B), not this package.
func ParseAggregate(r io.Reader) (AggregateSignal, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return AggregateSignal{}, fmt.Errorf("aggregate: read input: %w", err)
	}

	var a AggregateSignal
	if err := json.Unmarshal(raw, &a); err != nil {
		return AggregateSignal{}, fmt.Errorf("aggregate: decode JSON: %w", err)
	}

	if a.Type != TypeAggregate {
		return AggregateSignal{}, fmt.Errorf("aggregate: type must be %q, got %q", TypeAggregate, a.Type)
	}

	if err := validateAgainstSchema(raw); err != nil {
		return AggregateSignal{}, fmt.Errorf("aggregate: schema validation: %w", err)
	}

	if err := Validate(a); err != nil {
		return AggregateSignal{}, fmt.Errorf("aggregate: %w", err)
	}

	return a, nil
}

func validateAgainstSchema(jsonDoc []byte) error {
	s, err := compiledSchema()
	if err != nil {
		return err
	}
	var instance any
	if err := json.Unmarshal(jsonDoc, &instance); err != nil {
		return fmt.Errorf("decode instance: %w", err)
	}
	if err := s.Validate(instance); err != nil {
		return err
	}
	return nil
}
