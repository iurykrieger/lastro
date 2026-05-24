package sensor

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/iurykrieger/lastro/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

const sensorSchemaURL = "https://lastro.dev/harness/schemas/sensor.yaml"

var (
	schemaOnce     sync.Once
	schemaCompiled *jsonschema.Schema
	schemaErr      error
)

// compiledSchema returns the compiled JSON Schema for Sensor, parsed
// once from the embedded schemas/sensor.yaml. Subsequent calls reuse
// the cached schema and the cached error.
func compiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		raw, err := schemas.FS.ReadFile("sensor.yaml")
		if err != nil {
			schemaErr = fmt.Errorf("sensor: read embedded schema: %w", err)
			return
		}
		asJSON, err := yaml.YAMLToJSON(raw)
		if err != nil {
			schemaErr = fmt.Errorf("sensor: yaml->json schema: %w", err)
			return
		}
		var doc any
		if err := json.Unmarshal(asJSON, &doc); err != nil {
			schemaErr = fmt.Errorf("sensor: unmarshal schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(sensorSchemaURL, doc); err != nil {
			schemaErr = fmt.Errorf("sensor: add schema resource: %w", err)
			return
		}
		s, err := c.Compile(sensorSchemaURL)
		if err != nil {
			schemaErr = fmt.Errorf("sensor: compile schema: %w", err)
			return
		}
		schemaCompiled = s
	})
	return schemaCompiled, schemaErr
}
