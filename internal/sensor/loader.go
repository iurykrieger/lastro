package sensor

import (
	"encoding/json"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// LoadSensor reads, schema-validates, deserializes, and intrinsically
// validates a sensor YAML file. Returns a fully-validated Sensor or a
// path-decorated error. No grounding is performed — call
// ValidateAgainstStack / ValidateAgainstFixtures separately.
func LoadSensor(path string) (Sensor, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Sensor{}, fmt.Errorf("sensor %s: read: %w", path, err)
	}

	asJSON, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return Sensor{}, fmt.Errorf("sensor %s: yaml->json: %w", path, err)
	}

	if err := validateAgainstSchema(asJSON); err != nil {
		return Sensor{}, fmt.Errorf("sensor %s: schema validation: %w", path, err)
	}

	var s Sensor
	if err := json.Unmarshal(asJSON, &s); err != nil {
		return Sensor{}, fmt.Errorf("sensor %s: deserialize: %w", path, err)
	}

	return s, nil
}

func validateAgainstSchema(jsonDoc []byte) error {
	sch, err := compiledSchema()
	if err != nil {
		return err
	}
	var instance any
	if err := json.Unmarshal(jsonDoc, &instance); err != nil {
		return fmt.Errorf("decode instance: %w", err)
	}
	if err := sch.Validate(instance); err != nil {
		return err
	}
	return nil
}
