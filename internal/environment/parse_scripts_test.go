// internal/environment/parse_scripts_test.go
package environment

import (
	"path/filepath"
	"testing"
)

func TestParsePackageScripts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{
	  "scripts": { "dev": "next dev", "db:migrate": "drizzle-kit migrate" }
	}`)
	got, err := parsePackageScripts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got["dev"] != "next dev" || got["db:migrate"] != "drizzle-kit migrate" {
		t.Fatalf("scripts = %v", got)
	}
}

func TestParsePackageScripts_Absent(t *testing.T) {
	got, err := parsePackageScripts(t.TempDir())
	if err != nil {
		t.Fatalf("absent package.json must not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}

func TestParseProcfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Procfile"), "web: node server.js\nworker: node worker.js\n")
	got := parseProcfile(dir)
	if got["web"] != "node server.js" || got["worker"] != "node worker.js" {
		t.Fatalf("procfile = %v", got)
	}
}
