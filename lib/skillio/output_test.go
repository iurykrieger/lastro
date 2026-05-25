package skillio

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEmitJSON_AppendsNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := EmitJSON(&buf, map[string]string{"hello": "world"}); err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}
	got := buf.String()
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("output missing trailing newline: %q", got)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v (%q)", err, got)
	}
	if decoded["hello"] != "world" {
		t.Errorf("decoded[hello] = %q, want world", decoded["hello"])
	}
}

func TestEmitError_StructuredEnvelope(t *testing.T) {
	var buf bytes.Buffer
	EmitError(&buf, "bad-handle", "handle malformed", map[string]any{"input": "abc"})
	var decoded ScriptError
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v (%q)", err, buf.String())
	}
	if decoded.Code != "bad-handle" {
		t.Errorf("Code = %q, want bad-handle", decoded.Code)
	}
	if decoded.Details["input"] != "abc" {
		t.Errorf("Details[input] = %v, want abc", decoded.Details["input"])
	}
}
