package sensor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	if err := validateIntrinsic(s); err != nil {
		return Sensor{}, fmt.Errorf("sensor %s: intrinsic validation: %w", path, err)
	}

	return s, nil
}

// LoadDirectory loads every *.yaml / *.yml file directly inside the
// given directory (non-recursive — sensors are flat per the framework's
// convention) and returns a Store containing them.
//
// Per-file load errors abort the walk and bubble up with the offending
// path. Duplicate-id errors name both source files so authors can
// locate the collision quickly.
func LoadDirectory(path string) (*Store, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("sensor dir %s: read: %w", path, err)
	}

	type loaded struct {
		s    Sensor
		from string
	}
	var sensors []loaded
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		full := filepath.Join(path, name)
		sn, err := LoadSensor(full)
		if err != nil {
			return nil, err // already path-decorated by LoadSensor
		}
		sensors = append(sensors, loaded{s: sn, from: full})
	}

	// Build the store. Catch duplicate ids here so the error can name both files.
	seen := make(map[string]string, len(sensors))
	bare := make([]Sensor, 0, len(sensors))
	for _, l := range sensors {
		if prior, dup := seen[l.s.ID]; dup {
			return nil, fmt.Errorf("sensor: duplicate id %q in %s and %s", l.s.ID, prior, l.from)
		}
		seen[l.s.ID] = l.from
		bare = append(bare, l.s)
	}

	return NewStore(bare...)
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
