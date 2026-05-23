package stack

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/iurykrieger/lastro/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

const (
	componentSchemaURL = "https://lastro.dev/harness/schemas/stack-component.yaml"
	manifestSchemaURL  = "https://lastro.dev/harness/schemas/stack-manifest.yaml"
)

// compileSchemas builds a jsonschema compiler with both stack-component.yaml
// and stack-manifest.yaml registered, so the manifest's $ref to the component
// schema resolves.
func compileSchemas() (manifest *jsonschema.Schema, component *jsonschema.Schema, err error) {
	c := jsonschema.NewCompiler()

	for url, file := range map[string]string{
		componentSchemaURL: "stack-component.yaml",
		manifestSchemaURL:  "stack-manifest.yaml",
	} {
		b, readErr := schemas.FS.ReadFile(file)
		if readErr != nil {
			return nil, nil, fmt.Errorf("read embedded %s: %w", file, readErr)
		}
		j, jErr := yaml.YAMLToJSON(b)
		if jErr != nil {
			return nil, nil, fmt.Errorf("yaml->json %s: %w", file, jErr)
		}
		var doc any
		if uErr := json.Unmarshal(j, &doc); uErr != nil {
			return nil, nil, fmt.Errorf("unmarshal %s: %w", file, uErr)
		}
		if rErr := c.AddResource(url, doc); rErr != nil {
			return nil, nil, fmt.Errorf("register %s: %w", file, rErr)
		}
	}

	manifest, err = c.Compile(manifestSchemaURL)
	if err != nil {
		return nil, nil, fmt.Errorf("compile manifest schema: %w", err)
	}
	component, err = c.Compile(componentSchemaURL)
	if err != nil {
		return nil, nil, fmt.Errorf("compile component schema: %w", err)
	}
	return manifest, component, nil
}

// Load reads, JSON-Schema-validates, unmarshals, programmatically validates,
// and indexes a stack manifest YAML file. It is the canonical entrypoint.
func Load(path string) (StackManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return StackManifest{}, fmt.Errorf("read %s: %w", path, err)
	}
	manifestSch, _, err := compileSchemas()
	if err != nil {
		return StackManifest{}, err
	}
	if err := validateAgainstSchema(b, manifestSch); err != nil {
		return StackManifest{}, fmt.Errorf("%s: %w", path, err)
	}
	var m StackManifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return StackManifest{}, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return StackManifest{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := m.buildIndex(); err != nil {
		return StackManifest{}, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// LoadComponent loads a single StackComponent YAML — useful for fixtures and
// tests. Follows the same pipeline as Load, against the component schema.
func LoadComponent(path string) (StackComponent, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return StackComponent{}, fmt.Errorf("read %s: %w", path, err)
	}
	_, componentSch, err := compileSchemas()
	if err != nil {
		return StackComponent{}, err
	}
	if err := validateAgainstSchema(b, componentSch); err != nil {
		return StackComponent{}, fmt.Errorf("%s: %w", path, err)
	}
	var c StackComponent
	if err := yaml.Unmarshal(b, &c); err != nil {
		return StackComponent{}, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return StackComponent{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
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

// buildIndex populates m.byID and rejects duplicate component ids by naming
// both occurrences.
func (m *StackManifest) buildIndex() error {
	m.byID = make(map[string]StackComponent, len(m.Components))
	first := make(map[string]int, len(m.Components))
	for i, c := range m.Components {
		if prev, dup := first[c.ID]; dup {
			return fmt.Errorf(
				"duplicate component id %q at components[%d] and components[%d]",
				c.ID, prev, i,
			)
		}
		first[c.ID] = i
		m.byID[c.ID] = c
	}
	return nil
}

// yamlMarshal is exposed for tests that need to write a manifest back out.
func yamlMarshal(v any) ([]byte, error) {
	return yaml.Marshal(v)
}
