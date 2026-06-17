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

func TestParseMakeTargets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Makefile"),
		"build:\n\tgo build ./...\n\n.PHONY: build test\n\nCC = gcc\n")
	got := parseMakeTargets(dir)

	if got["build"] != "make build" {
		t.Fatalf("normal target: want %q, got %q", "make build", got["build"])
	}
	if _, ok := got[".PHONY"]; ok {
		t.Fatalf(".PHONY line must be excluded, got %v", got)
	}
	if _, ok := got["CC"]; ok {
		t.Fatalf("variable assignment must be excluded, got %v", got)
	}
	if _, ok := got["go build ./..."]; ok {
		t.Fatalf("tab-indented recipe line must be excluded, got %v", got)
	}
	if len(got) != 1 {
		t.Fatalf("want exactly one target, got %v", got)
	}
}

func TestParseMakeTargets_Absent(t *testing.T) {
	if got := parseMakeTargets(t.TempDir()); len(got) != 0 {
		t.Fatalf("absent Makefile must yield empty, got %v", got)
	}
}
