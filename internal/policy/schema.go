package policy

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

// compiledSchema returns the compiled JSON Schema for ValidationPolicy.
// It loads schemas/validation-policy.yaml via the centralized schemas
// package's embed.FS on the first call and caches the result.
func compiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		raw, err := schemas.FS.ReadFile("validation-policy.yaml")
		if err != nil {
			schemaErr = fmt.Errorf("policy: read schema from schemas.FS: %w", err)
			return
		}
		var doc any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			schemaErr = fmt.Errorf("policy: parse schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		const url = "https://lastro.dev/harness/schemas/validation-policy.yaml"
		if err := c.AddResource(url, doc); err != nil {
			schemaErr = fmt.Errorf("policy: add schema resource: %w", err)
			return
		}
		s, err := c.Compile(url)
		if err != nil {
			schemaErr = fmt.Errorf("policy: compile schema: %w", err)
			return
		}
		schemaCompiled = s
	})
	return schemaCompiled, schemaErr
}
