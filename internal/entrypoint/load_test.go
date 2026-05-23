package entrypoint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestLoadEntryPoint_HTTPAPIExample(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "examples", "entry-point", "http-api.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	ep, err := LoadEntryPoint(raw)
	if err != nil {
		t.Fatalf("LoadEntryPoint: %v", err)
	}
	if ep.ID != "create-order-endpoint" {
		t.Errorf("ID = %q, want create-order-endpoint", ep.ID)
	}
	if ep.Archetype != enums.ArchetypeHTTPAPI {
		t.Errorf("Archetype = %q, want %q", ep.Archetype, enums.ArchetypeHTTPAPI)
	}
	if got := ep.Spec["method"]; got != "POST" {
		t.Errorf("Spec[method] = %v, want POST", got)
	}
	if got := ep.Spec["path"]; got != "/orders" {
		t.Errorf("Spec[path] = %v, want /orders", got)
	}
}

func TestLoadFromExample_AllArchetypes(t *testing.T) {
	cases := []struct {
		file     string
		wantArch enums.Archetype
		wantSpec map[string]any
	}{
		{
			file:     "http-api.yaml",
			wantArch: enums.ArchetypeHTTPAPI,
			wantSpec: map[string]any{"method": "POST", "path": "/orders"},
		},
		{
			file:     "event-consumer.yaml",
			wantArch: enums.ArchetypeEventConsumer,
		},
		{file: "event-producer.yaml", wantArch: enums.ArchetypeEventProducer},
		{file: "cli.yaml", wantArch: enums.ArchetypeCLI},
		{file: "sdk.yaml", wantArch: enums.ArchetypeSDK},
		{file: "library.yaml", wantArch: enums.ArchetypeLibrary},
		{file: "worker.yaml", wantArch: enums.ArchetypeWorker},
		{file: "batch-job.yaml", wantArch: enums.ArchetypeBatchJob},
		{file: "static-site.yaml", wantArch: enums.ArchetypeStaticSite},
	}
	for _, tc := range cases {
		t.Run(string(tc.wantArch), func(t *testing.T) {
			path := filepath.Join("..", "..", "schemas", "examples", "entry-point", tc.file)
			ep, err := LoadFromExample(path)
			if err != nil {
				t.Fatalf("LoadFromExample(%s): %v", path, err)
			}
			if ep.ID == "" {
				t.Errorf("ID is empty")
			}
			if ep.Archetype != tc.wantArch {
				t.Errorf("Archetype = %q, want %q", ep.Archetype, tc.wantArch)
			}
			if ep.Spec == nil {
				t.Fatalf("Spec is nil")
			}
			for k, want := range tc.wantSpec {
				if got, ok := ep.Spec[k]; !ok || got != want {
					t.Errorf("Spec[%q] = (%v, %v), want (%v, true)", k, got, ok, want)
				}
			}
		})
	}
}
