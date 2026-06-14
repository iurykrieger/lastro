// internal/environment/parse_test.go
package environment

import (
	"path/filepath"
	"testing"
)

func TestParse_DashboardShape(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"scripts":{"dev":"next dev","db:migrate":"drizzle-kit migrate"}}`)
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), "services:\n  postgres:\n    image: postgres:16-alpine\n")
	writeFile(t, filepath.Join(dir, ".env.example"), "DATABASE_URL=x\n")
	f, err := Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Scripts["dev"] != "next dev" {
		t.Errorf("scripts.dev = %q", f.Scripts["dev"])
	}
	if _, ok := f.ComposeServices["postgres"]; !ok {
		t.Errorf("missing compose postgres: %v", f.ComposeServices)
	}
	if f.ComposeFile != "docker-compose.yml" {
		t.Errorf("compose file = %q", f.ComposeFile)
	}
	if len(f.EnvKeys) != 1 || f.EnvKeys[0] != "DATABASE_URL" {
		t.Errorf("env keys = %v", f.EnvKeys)
	}
}

func TestParse_NoInfra(t *testing.T) {
	f, err := Parse(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Scripts) != 0 || len(f.ComposeServices) != 0 {
		t.Fatalf("want empty facts, got %+v", f)
	}
}
