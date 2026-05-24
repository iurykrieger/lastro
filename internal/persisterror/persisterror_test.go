package persisterror

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestError_ImplementsError(t *testing.T) {
	var _ error = (*Error)(nil)
}

func TestError_MessageWithID(t *testing.T) {
	e := &Error{Kind: SchemaViolation, EntityType: "sensor", EntityID: "s1", Message: "bad"}
	got := e.Error()
	want := `schema_violation on sensor "s1": bad`
	if got != want {
		t.Fatalf("Error()=%q, want %q", got, want)
	}
}

func TestError_MessageWithoutID(t *testing.T) {
	e := &Error{Kind: SchemaViolation, EntityType: "stack-manifest", Message: "bad"}
	got := e.Error()
	want := "schema_violation on stack-manifest: bad"
	if got != want {
		t.Fatalf("Error()=%q, want %q", got, want)
	}
}

func TestError_JSONRoundTrip(t *testing.T) {
	in := &Error{
		Kind:       FixtureBinding,
		EntityType: "use-case",
		EntityID:   "uc1",
		File:       "/tmp/uc.yaml",
		Path:       "fixture_ids[1]",
		Value:      "fx_missing",
		Expected:   "id present in .harness/fixtures/",
		Details:    map[string]any{"missing_fixture_ids": []any{"fx_missing"}},
		Message:    "fixture not found",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Error
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Kind != in.Kind || out.EntityType != in.EntityType ||
		out.EntityID != in.EntityID || out.File != in.File ||
		out.Path != in.Path || out.Expected != in.Expected ||
		out.Message != in.Message {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", out, in)
	}
}

func TestError_ErrorsAs(t *testing.T) {
	var wrapped error = &Error{Kind: Grounding, EntityType: "sensor", Message: "x"}
	var pe *Error
	if !errors.As(wrapped, &pe) {
		t.Fatal("errors.As failed to extract *Error")
	}
	if pe.Kind != Grounding {
		t.Fatalf("Kind=%q, want %q", pe.Kind, Grounding)
	}
}
