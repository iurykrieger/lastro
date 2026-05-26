package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRun_Follow_ExitsWhenSensorTerminates(t *testing.T) {
	root := setupSignalsFile(t, []string{`{"n":1}`, `{"n":2}`})
	// No entry in running_sensors.json — follow should drain and exit.
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"tail-sensor-signals", validSensorID + ":" + validRunID, "--follow"}, nil, &stdout, &stderr, root)
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit = %d, want 0; stderr=%s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), `"n":1`) || !strings.Contains(stdout.String(), `"n":2`) {
			t.Errorf("stdout missing existing lines: %q", stdout.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("follow did not exit within 5s; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRun_Follow_PicksUpNewLines(t *testing.T) {
	root := setupSignalsFile(t, []string{`{"n":1}`})
	runDir := filepath.Join(root, ".harness", "runtime", validSensorID, validRunID)

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"tail-sensor-signals", validSensorID + ":" + validRunID, "--follow"}, nil, &stdout, &stderr, root)
	}()

	// Give the follower a moment to read the existing line, then append.
	time.Sleep(400 * time.Millisecond)
	f, err := os.OpenFile(filepath.Join(runDir, "signals.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	_, _ = f.WriteString(`{"n":2}` + "\n")
	_ = f.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("follow did not exit within 5s; stdout=%q", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"n":2`) {
		t.Errorf("stdout missing appended line: %q", stdout.String())
	}
}
