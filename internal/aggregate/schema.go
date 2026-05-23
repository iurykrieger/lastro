package aggregate

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

//go:embed schema.yaml
var embeddedSchemaYAML []byte

var (
	schemaOnce     sync.Once
	schemaCompiled *jsonschema.Schema
	schemaErr      error
)

const schemaURL = "https://lastro.dev/harness/schemas/aggregate-signal.yaml"

func compiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		var doc any
		if err := yaml.Unmarshal(embeddedSchemaYAML, &doc); err != nil {
			schemaErr = fmt.Errorf("aggregate: parse embedded schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(schemaURL, doc); err != nil {
			schemaErr = fmt.Errorf("aggregate: add schema resource: %w", err)
			return
		}
		s, err := c.Compile(schemaURL)
		if err != nil {
			schemaErr = fmt.Errorf("aggregate: compile schema: %w", err)
			return
		}
		schemaCompiled = s
	})
	return schemaCompiled, schemaErr
}
