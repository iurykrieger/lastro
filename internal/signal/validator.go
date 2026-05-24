package signal

import (
	"encoding/json"
	"fmt"
)

// Validate checks a Signal against the canonical JSON Schema. Use this
// when a Signal was constructed in Go (e.g., test fixtures, E8 rollup
// inputs) rather than parsed from JSONL — the parser already validates
// each line it yields.
func Validate(sig Signal) error {
	raw, err := json.Marshal(sig)
	if err != nil {
		return fmt.Errorf("signal: marshal for validation: %w", err)
	}
	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		return fmt.Errorf("signal: re-decode for validation: %w", err)
	}
	s, err := compiledSchema()
	if err != nil {
		return err
	}
	if err := s.Validate(instance); err != nil {
		return fmt.Errorf("signal: schema: %w", err)
	}
	return nil
}
