package skillio

import (
	"testing"

	"github.com/iurykrieger/lastro/internal/enums"
)

func TestExitCodeForVerdict(t *testing.T) {
	cases := []struct {
		v    enums.Verdict
		want int
	}{
		{enums.VerdictPass, ExitPass},
		{enums.VerdictFail, ExitFail},
		{enums.VerdictInconclusive, ExitInconclusive},
		{enums.VerdictWarn, ExitFail}, // warn is treated as fail for exit-code purposes
	}
	for _, c := range cases {
		if got := ExitCodeForVerdict(c.v); got != c.want {
			t.Errorf("ExitCodeForVerdict(%q) = %d, want %d", c.v, got, c.want)
		}
	}
}

func TestScriptError_Code(t *testing.T) {
	err := NewScriptError("bad-handle", "handle malformed", map[string]any{"input": "abc"})
	if err.Code != "bad-handle" {
		t.Errorf("Code = %q, want bad-handle", err.Code)
	}
	if err.Message != "handle malformed" {
		t.Errorf("Message = %q, want 'handle malformed'", err.Message)
	}
	if err.Details["input"] != "abc" {
		t.Errorf("Details[input] = %v, want abc", err.Details["input"])
	}
}
