package entrypoint

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadEntryPoint_RejectsInvalidFixtures(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		expectSubs string // case-insensitive substring expected in the error
	}{
		{"missing id", "testdata/missing-id.yaml", "id"},
		{"unknown archetype", "testdata/unknown-archetype.yaml", "archetype"},
		{"http-api missing method", "testdata/http-api-missing-method.yaml", "method"},
		{"event-consumer bad channel_kind", "testdata/event-consumer-bad-channel-kind.yaml", "channel_kind"},
		{"worker bad trigger_kind", "testdata/worker-bad-trigger-kind.yaml", "trigger_kind"},
		{"cli extra spec field", "testdata/extra-spec-field.yaml", "additional"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("read %s: %v", tc.file, err)
			}
			_, err = LoadEntryPoint(raw)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.expectSubs)) {
				t.Errorf("error %q missing expected substring %q", err.Error(), tc.expectSubs)
			}
		})
	}
}
