// internal/environment/parse_compose_test.go
package environment

import (
	"path/filepath"
	"testing"
)

func TestParseCompose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), `services:
  postgres:
    image: postgres:16-alpine
    ports: ["5432:5432"]
    environment:
      POSTGRES_USER: reliable
    depends_on: []
volumes:
  pgdata:
`)
	svcs, file, err := parseCompose(dir)
	if err != nil {
		t.Fatal(err)
	}
	if file != "docker-compose.yml" {
		t.Fatalf("file = %q", file)
	}
	pg, ok := svcs["postgres"]
	if !ok || pg.Image != "postgres:16-alpine" || pg.Ports[0] != "5432:5432" {
		t.Fatalf("postgres = %+v ok=%v", pg, ok)
	}
}

func TestParseCompose_Absent(t *testing.T) {
	svcs, file, err := parseCompose(t.TempDir())
	if err != nil || len(svcs) != 0 || file != "" {
		t.Fatalf("absent compose: svcs=%v file=%q err=%v", svcs, file, err)
	}
}

func TestParseDotenvKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env.example"), "# comment\nDATABASE_URL=postgres://x\nNEXTAUTH_SECRET=changeme\n\nEMPTY=\n")
	keys := parseDotenvKeys(dir)
	want := map[string]bool{"DATABASE_URL": true, "NEXTAUTH_SECRET": true, "EMPTY": true}
	if len(keys) != 3 {
		t.Fatalf("keys = %v", keys)
	}
	for _, k := range keys {
		if !want[k] {
			t.Fatalf("unexpected key %q in %v", k, keys)
		}
	}
}
