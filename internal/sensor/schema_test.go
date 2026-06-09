package sensor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/schemas"
	"sigs.k8s.io/yaml"
)

func TestSchemasFSContainsSensorYAML(t *testing.T) {
	b, err := schemas.FS.ReadFile("sensor.yaml")
	if err != nil {
		t.Fatalf("schemas.FS.ReadFile(sensor.yaml): %v", err)
	}
	if len(b) == 0 {
		t.Fatal("schemas.FS sensor.yaml is empty")
	}
}

func TestCompiledSchemaIsAvailableAndCached(t *testing.T) {
	first, err := compiledSchema()
	if err != nil {
		t.Fatalf("compiledSchema first call: %v", err)
	}
	if first == nil {
		t.Fatal("compiledSchema first call returned nil schema")
	}
	second, err := compiledSchema()
	if err != nil {
		t.Fatalf("compiledSchema second call: %v", err)
	}
	if first != second {
		t.Errorf("compiledSchema not cached: first=%p second=%p", first, second)
	}
}

func TestSchemaAcceptsCompositionExamples(t *testing.T) {
	for _, f := range []string{"core-e2e-primitive.yaml", "uc-consumer.yaml", "core-run-dev.yaml"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "schemas", "examples", "sensor", f))
		if err != nil {
			t.Fatal(err)
		}
		asJSON, err := yaml.YAMLToJSON(raw)
		if err != nil {
			t.Fatalf("%s: yaml->json: %v", f, err)
		}
		if err := validateAgainstSchema(asJSON); err != nil {
			t.Errorf("%s: schema validation: %v", f, err)
		}
	}
}

func TestSchemaRejectsStepWithBothRunAndUses(t *testing.T) {
	raw := []byte("schema_version: 1.0.0\nid: x\nscope: core\nangle: build\nkind: assertion\nnature: computational\noutput_type: single-shot\nuses: []\nsteps:\n  - id: s\n    run: echo\n    uses: p\n")
	asJSON, _ := yaml.YAMLToJSON(raw)
	if err := validateAgainstSchema(asJSON); err == nil {
		t.Fatal("expected schema rejection of step with both run and uses")
	}
}

func TestSchemaAcceptsObserveWindow(t *testing.T) {
	good := []byte("schema_version: 1.0.0\nid: logs\nscope: use-case\nuse_case_id: uc\nangle: logs\nkind: observational\nnature: computational\noutput_type: stream\nuses: []\nobserve_window: 45s\nsteps:\n  - id: watch\n    uses: run-dev\n")
	asJSON, err := yaml.YAMLToJSON(good)
	if err != nil {
		t.Fatalf("yaml->json: %v", err)
	}
	if err := validateAgainstSchema(asJSON); err != nil {
		t.Errorf("observe_window 45s should validate: %v", err)
	}

	bad := []byte("schema_version: 1.0.0\nid: logs\nscope: use-case\nuse_case_id: uc\nangle: logs\nkind: observational\nnature: computational\noutput_type: stream\nuses: []\nobserve_window: 45seconds\nsteps:\n  - id: watch\n    uses: run-dev\n")
	asJSONBad, _ := yaml.YAMLToJSON(bad)
	if err := validateAgainstSchema(asJSONBad); err == nil {
		t.Error("observe_window 45seconds should be rejected by the duration pattern")
	}
}
