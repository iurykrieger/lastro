// internal/environment/helpers_test.go
package environment

import (
	"os"
	"testing"

	"sigs.k8s.io/yaml"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func yamlMarshalForTest(t *testing.T, v any) ([]byte, error) {
	t.Helper()
	b, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b, nil
}
