package sensor

import (
	"testing"

	"github.com/iurykrieger/lastro/schemas"
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
