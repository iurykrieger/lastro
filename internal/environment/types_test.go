// internal/environment/types_test.go
package environment

import "testing"

func TestProvidedBy_RoundTrip(t *testing.T) {
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{File: "package.json", Path: "scripts.dev"}, DependsOn: []string{"postgres"}},
		Dependencies: map[string]Dependency{
			"postgres": {Type: "datastore", ProvidedBy: ProvidedBy{File: "docker-compose.yml", Path: "services.postgres"}},
		},
		Setup: []SetupNode{{ID: "migrate", Type: "setup", ProvidedBy: ProvidedBy{File: "package.json", Path: "scripts.db:migrate"}, DependsOn: []string{"postgres"}}},
	}
	if got := m.Dependencies["postgres"].Type; got != DependencyDatastore {
		t.Fatalf("type = %q, want %q", got, DependencyDatastore)
	}
	names := m.NodeNames()
	if len(names) != 2 { // postgres, migrate; application is not a depend-able node
		t.Fatalf("NodeNames = %v, want 2 (application excluded as a target)", names)
	}
}
