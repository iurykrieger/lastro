package stack

import (
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestStackManifest_Validate_AcceptsPatchBumpedVersion(t *testing.T) {
	m := StackManifest{
		SchemaVersion: "1.0.7",
		Archetype:     enums.ArchetypeHTTPAPI,
		Components: []StackComponent{{
			SchemaVersion:     "1.0.7",
			ID:                "express",
			Kind:              enums.StackKindLibrary,
			Name:              "express",
			Version:           "4.18.0",
			Capabilities:      []string{"http-routing"},
			DetectionEvidence: []EvidenceRef{{File: "package.json", Path: ".dependencies.express"}},
		}},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate rejected patch-bumped version: %v", err)
	}
}

func TestStackManifest_Validate_RejectsDifferentMajor(t *testing.T) {
	m := StackManifest{
		SchemaVersion: "2.0.0",
		Archetype:     enums.ArchetypeHTTPAPI,
		Components: []StackComponent{{
			SchemaVersion:     "2.0.0",
			ID:                "express",
			Kind:              enums.StackKindLibrary,
			Name:              "express",
			Version:           "4.18.0",
			Capabilities:      []string{"http-routing"},
			DetectionEvidence: []EvidenceRef{{File: "package.json", Path: ".dependencies.express"}},
		}},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("Validate accepted incompatible major version")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("error should mention schema_version, got: %v", err)
	}
}
