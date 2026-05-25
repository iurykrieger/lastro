package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	validSensorID = "test-sensor"
	validRunID    = "01HMG12RXATAFM4N0F0X5Y4SGE"
)

func setupSignalsFile(t *testing.T, lines []string) string {
	t.Helper()
	root := t.TempDir()
	runDir := filepath.Join(root, ".harness", "runtime", validSensorID, validRunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "signals.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return root
}

func TestRun_Snapshot_AllLines(t *testing.T) {
	root := setupSignalsFile(t, []string{`{"verdict":"pass","n":1}`, `{"verdict":"pass","n":2}`, `{"verdict":"pass","n":3}`})
	var stdout, stderr bytes.Buffer
	code := run([]string{"tail-sensor-signals", validSensorID + ":" + validRunID}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit = %d; stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("got %d lines, want 3 (%q)", len(lines), stdout.String())
	}
}

func TestRun_Snapshot_Since(t *testing.T) {
	root := setupSignalsFile(t, []string{`{"n":1}`, `{"n":2}`, `{"n":3}`, `{"n":4}`})
	var stdout, stderr bytes.Buffer
	code := run([]string{"tail-sensor-signals", validSensorID + ":" + validRunID, "--since=3"}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit = %d; stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("got %d lines, want 2 (lines 3 and 4); stdout=%q", len(lines), stdout.String())
	}
	if !strings.Contains(stdout.String(), `"n":3`) || !strings.Contains(stdout.String(), `"n":4`) {
		t.Errorf("stdout missing expected lines: %q", stdout.String())
	}
}

func TestRun_BadHandle(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"tail-sensor-signals", "garbage"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}

func TestRun_MissingFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".harness"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"tail-sensor-signals", validSensorID + ":" + validRunID}, nil, &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}
