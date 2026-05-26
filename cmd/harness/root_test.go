package main

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestRootCmd_DefaultFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newRootCmd(context.Background(), cfg)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if cfg.Output != "text" {
		t.Errorf("default Output = %q, want %q", cfg.Output, "text")
	}
	if cfg.Quiet || cfg.Verbose {
		t.Errorf("default Quiet/Verbose = %v/%v, want false/false", cfg.Quiet, cfg.Verbose)
	}
	if cfg.Concurrency != 0 {
		t.Errorf("default Concurrency = %d, want 0", cfg.Concurrency)
	}
}

func TestRootCmd_FlagOverrides(t *testing.T) {
	cfg := &Config{}
	cmd := newRootCmd(context.Background(), cfg)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"--output", "json",
		"--quiet",
		"--policy", "/tmp/p.yaml",
		"--repo-root", "/tmp/repo",
		"--concurrency", "4",
		"--timeout", "30s",
		"version",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if cfg.Output != "json" {
		t.Errorf("Output = %q, want %q", cfg.Output, "json")
	}
	if !cfg.Quiet {
		t.Errorf("Quiet = false, want true")
	}
	if cfg.Policy != "/tmp/p.yaml" {
		t.Errorf("Policy = %q, want /tmp/p.yaml", cfg.Policy)
	}
	if cfg.RepoRoot != "/tmp/repo" {
		t.Errorf("RepoRoot = %q, want /tmp/repo", cfg.RepoRoot)
	}
	if cfg.Concurrency != 4 {
		t.Errorf("Concurrency = %d, want 4", cfg.Concurrency)
	}
	if cfg.Timeout.Seconds() != 30 {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
}

func TestConfig_EffectiveConcurrency(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{
		{0, runtime.GOMAXPROCS(0)},
		{-1, runtime.GOMAXPROCS(0)},
		{1, 1},
		{8, 8},
	}
	for _, tc := range tests {
		cfg := &Config{Concurrency: tc.in}
		if got := cfg.EffectiveConcurrency(); got != tc.want {
			t.Errorf("Concurrency=%d -> %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestValidateCmd_ConflictingFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newRootCmd(context.Background(), cfg)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"validate", "--use-case", "foo", "--all"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected UsageError, got nil")
	}
	var usageErr *UsageError
	if !asUsageError(err, &usageErr) {
		t.Fatalf("err = %v (%T), want *UsageError", err, err)
	}
	if !strings.Contains(usageErr.Msg, "exactly one") {
		t.Errorf("UsageError.Msg = %q, want substring 'exactly one'", usageErr.Msg)
	}
}

func TestValidateCmd_NoFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newRootCmd(context.Background(), cfg)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"validate"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected UsageError, got nil")
	}
	var usageErr *UsageError
	if !asUsageError(err, &usageErr) {
		t.Fatalf("err = %v (%T), want *UsageError", err, err)
	}
}

// asUsageError is a tiny errors.As wrapper avoiding extra imports in
// every test function.
func asUsageError(err error, target **UsageError) bool {
	for err != nil {
		if u, ok := err.(*UsageError); ok {
			*target = u
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
