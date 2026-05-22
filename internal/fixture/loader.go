package fixture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// Phase 4: eager payload parse (dispatched on content_type).
	parsed, err := parsePayload(fx.ContentType, fx.Payload)
	if err != nil {
		return Fixture{}, fmt.Errorf("fixture %s: parse payload: %w", path, err)
	}
	fx.Parsed = parsed

	return fx, nil
}

// LoadDirectory loads every *.yaml / *.yml file directly inside the given
// directory (non-recursive — fixtures are flat per the framework's
// convention) and returns a Store containing them.
//
// Returns the first per-file load error encountered, or a duplicate-id
// error decorated with both source file paths.
func LoadDirectory(path string) (*Store, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("fixture dir %s: read: %w", path, err)
	}

	type loaded struct {
		fx   Fixture
		from string
	}
	var fixtures []loaded
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		full := filepath.Join(path, name)
		fx, err := LoadFixture(full)
		if err != nil {
			return nil, err // already path-decorated by LoadFixture
		}
		fixtures = append(fixtures, loaded{fx: fx, from: full})
	}

	// Build the store. Catch duplicate ids ourselves so we can name both files.
	seen := make(map[string]string, len(fixtures))
	bare := make([]Fixture, 0, len(fixtures))
	for _, l := range fixtures {
		if prior, dup := seen[l.fx.ID]; dup {
			return nil, fmt.Errorf("fixture: duplicate id %q in %s and %s", l.fx.ID, prior, l.from)
		}
		seen[l.fx.ID] = l.from
		bare = append(bare, l.fx)
	}

	return NewStore(bare...)
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
