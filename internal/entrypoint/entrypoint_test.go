package entrypoint

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestSpecFieldHitAndMiss(t *testing.T) {
	ep := EntryPoint{
		ID:        "ep-create",
		Archetype: enums.ArchetypeHTTPAPI,
		Spec:      map[string]any{"method": "POST", "path": "/orders"},
	}
	if v, ok := ep.SpecField("method"); !ok || v != "POST" {
		t.Errorf("SpecField(method) = %v, %v; want POST, true", v, ok)
	}
	if _, ok := ep.SpecField("missing"); ok {
		t.Error("SpecField(missing) ok=true; want false")
	}
}

func TestLabelFormat(t *testing.T) {
	ep := EntryPoint{ID: "ep-create", Archetype: enums.ArchetypeHTTPAPI}
	if got := ep.Label(); got != "http-api:ep-create" {
		t.Errorf("Label = %q, want %q", got, "http-api:ep-create")
	}
}
