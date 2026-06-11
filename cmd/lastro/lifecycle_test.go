package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// writeServiceRepro lays a minimal .harness tree on disk: a shared run-dev
// core observational service plus one assertion consumer that declares
// depends_on: [run-dev] and whose probe step only succeeds while the
// service process is actually alive — the issue #42 shape. pidPath
// receives the service shell's PID so the test can assert both that the
// service was up during the run and torn down afterwards.
func writeServiceRepro(t *testing.T, repoRoot, pidPath string) {
	t.Helper()
	mustWrite := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(repoRoot, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite(".harness/use-cases/health-check.yaml", `schema_version: 2.0.0
id: health-check
title: "Health check responds"
archetype_scope: [web-app]
entry_points:
  - id: health-endpoint
    archetype: web-app
    spec:
      path: /health
      method: GET
given:
  - "the dev server is running"
when:
  - "a health request is made"
then:
  - "it responds ok"
`)

	mustWrite(".harness/sensors/core/run-dev.yaml", fmt.Sprintf(`schema_version: 1.0.0
id: run-dev
scope: core
angle: environment
kind: observational
nature: computational
output_type: stream
uses: []
signal_matches:
  - key: ready
    pattern: "ready"
    verdict: pass
    expected: true
steps:
  - id: boot
    run: 'echo $$ > %s; echo "ready - started server"; sleep 600'
`, pidPath))

	mustWrite(".harness/sensors/health-check/s-health-check-e2e.yaml", fmt.Sprintf(`schema_version: 1.0.0
id: s-health-check-e2e
use_case_id: health-check
scope: use-case
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: []
depends_on: [run-dev]
signal_matches:
  - key: ok
    pattern: "OK"
    verdict: pass
steps:
  - id: probe
    run: 'kill -0 "$(cat %s)" && echo OK'
`, pidPath))
}

// TestRunRunSensor_StartsDependsOnServices reproduces issue #42: running a
// single assertion sensor that declares depends_on on a shared
// observational core service must bring the service up before executing
// the sensor's steps (and tear it down afterwards) — the same lifecycle
// validate-use-case already provides. Without it, the probe runs against a
// service that was never started and the run is graded as a phantom fail.
func TestRunRunSensor_StartsDependsOnServices(t *testing.T) {
	repoRoot := t.TempDir()
	pidPath := filepath.Join(t.TempDir(), "svc.pid")
	writeServiceRepro(t, repoRoot, pidPath)

	var stdout, stderr bytes.Buffer
	code := runRunSensor([]string{"run-sensor", "s-health-check-e2e"}, nil, &stdout, &stderr, repoRoot)
	if code != 0 {
		t.Fatalf("run-sensor exit code = %d, want 0 (depends_on service was not started?)\nstdout: %s\nstderr: %s",
			code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"verdict":"pass"`) {
		t.Fatalf("expected a pass verdict in output, got:\nstdout: %s", stdout.String())
	}

	// Teardown: the service's shell must be dead once the run returns.
	pidBytes, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("service pid file never written — run-dev was not started: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		t.Fatalf("parse pid file: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run-dev process %d still alive after run-sensor returned", pid)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
