package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_BadHandle_NoColon(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"stop-sensor", "no-colon-here"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "bad-handle") {
		t.Errorf("stderr missing bad-handle: %q", stderr.String())
	}
}

func TestRun_BadHandle_BadSensorID(t *testing.T) {
	// uppercase is not allowed in sensor-id slugs
	var stdout, stderr bytes.Buffer
	code := run([]string{"stop-sensor", "BadID:01HMG12RX9N6Z8WJ3D6PNHVQXC"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "bad-handle") {
		t.Errorf("stderr missing bad-handle: %q", stderr.String())
	}
}

func TestRun_BadHandle_BadRunID(t *testing.T) {
	// run-id must be 26-char ULID
	var stdout, stderr bytes.Buffer
	code := run([]string{"stop-sensor", "my-sensor:short"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "bad-handle") {
		t.Errorf("stderr missing bad-handle: %q", stderr.String())
	}
}

func TestRun_BadArgv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"stop-sensor"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}
