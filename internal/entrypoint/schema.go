package entrypoint

import (
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/schemas"
)

var (
	schemaOnce     sync.Once
	schemaCompiled *jsonschema.Schema
	schemaErr      error
)

// compiledSchema returns the compiled JSON Schema for EntryPoint. It loads
// schemas/entry-point.yaml via the centralized schemas package's embed.FS
// on the first call and caches the result. The canonical schema is owned
// by the schemas package; this package is purely a consumer.
func compiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		raw, err := schemas.FS.ReadFile("entry-point.yaml")
		if err != nil {
			schemaErr = fmt.Errorf("entrypoint: read schema from schemas.FS: %w", err)
			return
		}
		var doc any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			schemaErr = fmt.Errorf("entrypoint: parse schema: %w", err)
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
