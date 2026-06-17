// internal/environment/load.go
package environment

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/schemas"
)

const modelSchemaURL = "https://lastro.dev/harness/schemas/environment-model.yaml"

// Load reads, schema-validates, unmarshals, and programmatically validates an
// environment model file.
func Load(path string) (EnvironmentModel, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return EnvironmentModel{}, fmt.Errorf("read %s: %w", path, err)
	}
	m, err := LoadBytes(b)
	if err != nil {
		return EnvironmentModel{}, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// LoadBytes is the in-memory entrypoint (used by Persist's pre-write check).
func LoadBytes(b []byte) (EnvironmentModel, error) {
	sch, err := compileSchema()
	if err != nil {
		return EnvironmentModel{}, err
	}
	if err := validateAgainstSchema(b, sch); err != nil {
		return EnvironmentModel{}, err
	}
	var m EnvironmentModel
	if err := yaml.Unmarshal(b, &m); err != nil {
		return EnvironmentModel{}, fmt.Errorf("unmarshal: %w", err)
	}
	if err := m.Validate(); err != nil {
		return EnvironmentModel{}, err
	}
	return m, nil
}

func compileSchema() (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	b, err := schemas.FS.ReadFile("environment-model.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded environment-model.yaml: %w", err)
	}
	j, err := yaml.YAMLToJSON(b)
	if err != nil {
		return nil, fmt.Errorf("yaml->json: %w", err)
	}
	var doc any
	if err := json.Unmarshal(j, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}
	if err := c.AddResource(modelSchemaURL, doc); err != nil {
		return nil, fmt.Errorf("register schema: %w", err)
	}
	return c.Compile(modelSchemaURL)
}

func validateAgainstSchema(b []byte, sch *jsonschema.Schema) error {
	j, err := yaml.YAMLToJSON(b)
	if err != nil {
		return fmt.Errorf("yaml->json: %w", err)
	}
	var doc any
	if err := json.Unmarshal(j, &doc); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	if err := sch.Validate(doc); err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	return nil
}
