// internal/environment/validate_test.go
package environment

import (
	"strings"
	"testing"
)

func TestValidate_DanglingEdge(t *testing.T) {
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"package.json", "scripts.dev"}, DependsOn: []string{"ghost"}},
	}
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("want dangling-edge error naming ghost, got %v", err)
	}
}

func TestValidate_Cycle(t *testing.T) {
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"package.json", "scripts.dev"}},
		Dependencies: map[string]Dependency{
			"a": {Type: "datastore", ProvidedBy: ProvidedBy{"docker-compose.yml", "services.a"}, DependsOn: []string{"b"}},
			"b": {Type: "datastore", ProvidedBy: ProvidedBy{"docker-compose.yml", "services.b"}, DependsOn: []string{"a"}},
		},
	}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}

func TestValidate_OK(t *testing.T) {
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"package.json", "scripts.dev"}, DependsOn: []string{"postgres", "migrate"}},
		Dependencies:  map[string]Dependency{"postgres": {Type: "datastore", ProvidedBy: ProvidedBy{"docker-compose.yml", "services.postgres"}}},
		Setup:         []SetupNode{{ID: "migrate", Type: "setup", ProvidedBy: ProvidedBy{"package.json", "scripts.db:migrate"}, DependsOn: []string{"postgres"}}},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid model rejected: %v", err)
	}
}

func TestValidate_DuplicateNodeName(t *testing.T) {
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"package.json", "scripts.dev"}, DependsOn: []string{"migrate"}},
		Dependencies:  map[string]Dependency{"migrate": {Type: "datastore", ProvidedBy: ProvidedBy{"docker-compose.yml", "services.migrate"}}},
		Setup:         []SetupNode{{ID: "migrate", Type: "setup", ProvidedBy: ProvidedBy{"package.json", "scripts.db:migrate"}}},
	}
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("want duplicate-node error, got %v", err)
	}
}

func TestValidateGrounding_DependencyAndSetup(t *testing.T) {
	facts := RawFacts{
		Scripts:         map[string]string{"dev": "next dev"},
		ComposeServices: map[string]ComposeService{},
	}
	m := EnvironmentModel{
		SchemaVersion: "1.0.0",
		Application:   Application{ProvidedBy: ProvidedBy{"package.json", "scripts.dev"}},
		Dependencies:  map[string]Dependency{"postgres": {Type: "datastore", ProvidedBy: ProvidedBy{"docker-compose.yml", "services.postgres"}}},
		Setup:         []SetupNode{{ID: "migrate", Type: "setup", ProvidedBy: ProvidedBy{"package.json", "scripts.db:migrate"}}},
	}
	err := ValidateGrounding(m, facts)
	if err == nil {
		t.Fatal("want grounding error for unresolvable provided_by, got nil")
	}
	if !strings.Contains(err.Error(), "postgres") && !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("error should name the bad node, got %v", err)
	}
}

func TestLoadBytes_SchemaReject(t *testing.T) {
	// type not in enum → schema violation
	_, err := LoadBytes([]byte("schema_version: 1.0.0\napplication:\n  provided_by: {file: package.json, path: scripts.dev}\ndependencies:\n  x: {type: bogus, provided_by: {file: docker-compose.yml, path: services.x}}\n"))
	if err == nil {
		t.Fatal("want schema violation for bogus type")
	}
}
