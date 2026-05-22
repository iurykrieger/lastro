package stack

import (
	"testing"
)

func TestEvidenceRefStringRendersFileColonPath(t *testing.T) {
	ev := EvidenceRef{File: "package.json", Path: "dependencies.express"}
	got := ev.String()
	want := "package.json:dependencies.express"
	if got != want {
		t.Errorf("EvidenceRef.String() = %q, want %q", got, want)
	}
}

func TestEvidenceRefStringIgnoresValue(t *testing.T) {
	ev := EvidenceRef{File: "package.json", Path: "dependencies.express", Value: "^4.18.0"}
	got := ev.String()
	want := "package.json:dependencies.express"
	if got != want {
		t.Errorf("EvidenceRef.String() with value = %q, want %q", got, want)
	}
}

func TestSchemaVersionConstant(t *testing.T) {
	if SchemaVersion != "1.0.0" {
		t.Errorf("SchemaVersion = %q, want %q", SchemaVersion, "1.0.0")
	}
}
