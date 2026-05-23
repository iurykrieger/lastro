package entrypoint

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

// compiledSchema returns the compiled JSON Schema for EntryPoint, parsed
// once from the embedded schema.yaml. Subsequent calls reuse the cached
// schema.
func compiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		var doc any
		if err := yaml.Unmarshal(embeddedSchemaYAML, &doc); err != nil {
			schemaErr = fmt.Errorf("entrypoint: parse embedded schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		const url = "https://lastro.dev/harness/schemas/entry-point.yaml"
		if err := c.AddResource(url, doc); err != nil {
			schemaErr = fmt.Errorf("entrypoint: add schema resource: %w", err)
			return
		}
		s, err := c.Compile(url)
		if err != nil {
			schemaErr = fmt.Errorf("entrypoint: compile schema: %w", err)
			return
		}
		schemaCompiled = s
	})
	return schemaCompiled, schemaErr
}
