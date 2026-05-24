package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSynthesizeCrashHint_IncludesStderrTail(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.log")
	content := strings.Join([]string{
		"[2026-05-24T10:00:00.000000000Z step-01 stdout] noise",
		"[2026-05-24T10:00:00.000000000Z step-01 stderr] could not connect to redis",
		"[2026-05-24T10:00:00.000000000Z step-01 stderr] giving up",
	}, "\n") + "\n"
	if err := os.WriteFile(raw, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	hint := synthesizeCrashHint(raw, &SpawnError{Step: 1, Cause: errIO})
	if hint == nil {
		t.Fatalf("synthesizeCrashHint returned nil")
	}
	if !strings.Contains(hint.Summary, "step 1") {
		t.Errorf("Summary missing step ref: %q", hint.Summary)
	}
	if !strings.Contains(hint.Rationale, "could not connect to redis") {
		t.Errorf("Rationale missing stderr tail; got: %q", hint.Rationale)
	}
}

var errIO = stubErr("io broken")

type stubErr string

func (s stubErr) Error() string { return string(s) }
