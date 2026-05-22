package fixture

import (
	"encoding/json"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// LoadFixture loads, validates, and (partially) parses a fixture YAML file.
// Payload parsing is integrated in Task 14.
//
// Errors are wrapped with the file path and the failing phase for easy
// diagnosis.
func LoadFixture(path string) (Fixture, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, fmt.Errorf("fixture %s: read: %w", path, err)
	}

	// Phase 1: YAML → JSON normalization.
	asJSON, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return Fixture{}, fmt.Errorf("fixture %s: yaml-to-json: %w", path, err)
	}

	// Phase 2: JSON Schema validation.
	if err := validateAgainstSchema(asJSON); err != nil {
		return Fixture{}, fmt.Errorf("fixture %s: schema validation: %w", path, err)
	}

	// Phase 3: deserialize into typed Fixture.
	var fx Fixture
	if err := json.Unmarshal(asJSON, &fx); err != nil {
		return Fixture{}, fmt.Errorf("fixture %s: deserialize: %w", path, err)
	}

	// Phase 3b: Payload field is a string in YAML — extract it raw.
	// The struct's Payload field has json:"-", so json.Unmarshal skipped it;
	// pull the string out of the JSON document and re-encode as bytes.
	var rawPayload struct {
		Payload string `json:"payload"`
	}
	if err := json.Unmarshal(asJSON, &rawPayload); err != nil {
		return Fixture{}, fmt.Errorf("fixture %s: extract payload: %w", path, err)
	}
	fx.Payload = []byte(rawPayload.Payload)

	// Phase 4 (payload parsing): integrated in Task 14.

	return fx, nil
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
