package entrypoint

import (
	"encoding/json"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// LoadEntryPoint parses a single EntryPoint from YAML bytes. The pipeline is
// YAML→JSON normalize → JSON Schema validate → json.Unmarshal. Errors are
// wrapped with the failing phase.
func LoadEntryPoint(raw []byte) (EntryPoint, error) {
	asJSON, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return EntryPoint{}, fmt.Errorf("entrypoint: yaml-to-json: %w", err)
	}
	if err := validateAgainstSchema(asJSON); err != nil {
		return EntryPoint{}, fmt.Errorf("entrypoint: schema validation: %w", err)
	}
	var ep EntryPoint
	if err := json.Unmarshal(asJSON, &ep); err != nil {
		return EntryPoint{}, fmt.Errorf("entrypoint: deserialize: %w", err)
	}
	return ep, nil
}

// LoadFromExample reads a YAML file (typically one of the canonical examples
// under schemas/examples/entry-point/) and loads it as an EntryPoint. Test
// convenience; not for runtime use.
func LoadFromExample(path string) (EntryPoint, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return EntryPoint{}, fmt.Errorf("entrypoint: read %s: %w", path, err)
	}
	return LoadEntryPoint(raw)
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
	return s.Validate(instance)
}
