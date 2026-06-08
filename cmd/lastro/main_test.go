package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_NoArgs(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"harness-tools"}, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Fatalf("expected usage in stderr, got: %q", stderr.String())
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"harness-tools", "does-not-exist"}, nil, &bytes.Buffer{}, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Fatalf("expected 'unknown subcommand' in stderr, got: %q", stderr.String())
	}
}
