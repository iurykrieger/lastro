// internal/environment/integration_test.go
package environment

import (
	"os"
	"path/filepath"
	"testing"
)

func fixtureDir(t *testing.T) string {
	t.Helper()
	// repo-root-relative: internal/environment -> ../../examples/...
	d, err := filepath.Abs(filepath.Join("..", "..", "examples", "nextjs-drizzle-sample"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	return d
}

func TestIntegration_DashboardChain(t *testing.T) {
	dir := fixtureDir(t)
	facts, err := Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Scripts["dev"] != "next dev" || facts.ComposeServices["postgres"].Image != "postgres:16-alpine" {
		t.Fatalf("facts wrong: %+v", facts)
	}

	// The model the classifier should produce.
	model := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"package.json", "scripts.dev"}, DependsOn: []string{"postgres", "migrate"}},
		Dependencies:  map[string]Dependency{"postgres": {Type: DependencyDatastore, ProvidedBy: ProvidedBy{"docker-compose.yml", "services.postgres"}}},
		Setup:         []SetupNode{{ID: "migrate", Type: "setup", ProvidedBy: ProvidedBy{"package.json", "scripts.db:migrate"}, DependsOn: []string{"postgres"}}},
	}
	if err := model.Validate(); err != nil {
		t.Fatalf("model invalid: %v", err)
	}
	if err := ValidateGrounding(model, facts); err != nil {
		t.Fatalf("grounding failed: %v", err)
	}

	// Persist round-trips and reloads.
	harness := t.TempDir()
	out, _ := yamlMarshalForTest(t, model)
	factsOut, _ := yamlMarshalForTest(t, facts)
	if err := Persist(out, factsOut, harness); err != nil {
		t.Fatalf("persist: %v", err)
	}
	reloaded, err := Load(filepath.Join(harness, "environment-model.yaml"))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Application.DependsOn) != 2 {
		t.Fatalf("reloaded app deps = %v", reloaded.Application.DependsOn)
	}
}
