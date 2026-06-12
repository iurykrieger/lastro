package schemas

import (
	"testing"
)

func TestFSContainsKeySchemas(t *testing.T) {
	wanted := []string{
		"stack-component.yaml",
		"stack-manifest.yaml",
		"enums/stack-kinds.yaml",
		"enums/archetypes.yaml",
		"core-input-baseline.yaml",
		"core-inputs/e2e-test.yaml",
		"core-inputs/database.yaml",
		"core-inputs/performance.yaml",
		"core-inputs/logs.yaml",
		"core-inputs/metrics.yaml",
		"core-inputs/provision-auth.yaml",
	}
	for _, name := range wanted {
		b, err := FS.ReadFile(name)
		if err != nil {
			t.Errorf("FS.ReadFile(%q): %v", name, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("FS.ReadFile(%q): empty file", name)
		}
	}
}
