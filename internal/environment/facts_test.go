// internal/environment/facts_test.go
package environment

import "testing"

func TestRawFacts_Resolve(t *testing.T) {
	f := RawFacts{
		Scripts:         map[string]string{"dev": "next dev", "db:migrate": "drizzle-kit migrate"},
		ComposeServices: map[string]ComposeService{"postgres": {Image: "postgres:16-alpine"}},
		MakeTargets:     map[string]string{"run": "go run ."},
		ProcfileEntries: map[string]string{"web": "node server.js"},
	}
	cases := []struct {
		p    ProvidedBy
		want string
		ok   bool
	}{
		{ProvidedBy{"package.json", "scripts.dev"}, "next dev", true},
		{ProvidedBy{"package.json", "scripts.db:migrate"}, "drizzle-kit migrate", true},
		{ProvidedBy{"docker-compose.yml", "services.postgres"}, "docker compose up -d postgres", true},
		{ProvidedBy{"Makefile", "run"}, "go run .", true},
		{ProvidedBy{"Procfile", "web"}, "node server.js", true},
		{ProvidedBy{"package.json", "scripts.missing"}, "", false},
		{ProvidedBy{"unknown.txt", "x"}, "", false},
	}
	for _, c := range cases {
		got, ok := f.Resolve(c.p)
		if ok != c.ok || got != c.want {
			t.Errorf("Resolve(%+v) = (%q,%v), want (%q,%v)", c.p, got, ok, c.want, c.ok)
		}
	}
}
