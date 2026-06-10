package skillruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
)

// writeRepro lays a minimal .harness tree on disk: a shared run-dev core
// observational service plus two consumers (one assertion via depends_on,
// one observational that additionally attaches via a `uses: run-dev` step).
// The service announces readiness, emits request-log lines for the
// attaching consumer, then lingers — exactly the next-dev shape from
// issue #34. pidPath receives the service shell's PID so the test can
// assert the process was torn down.
func writeRepro(t *testing.T, repoRoot, pidPath string) {
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
  - key: log-line
    pattern: ".+"
    verdict: pass
steps:
  - id: boot
    run: 'echo $$ > %s; echo "ready - started server"; i=0; while [ $i -lt 100 ]; do echo "GET /health 200"; sleep 0.1; i=$((i+1)); done; sleep 600'
`, pidPath))

	mustWrite(".harness/sensors/health-check/s-health-check-http.yaml", `schema_version: 1.0.0
id: s-health-check-http
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
    run: "echo OK"
`)

	mustWrite(".harness/sensors/health-check/s-health-check-logs.yaml", `schema_version: 1.0.0
id: s-health-check-logs
use_case_id: health-check
scope: use-case
angle: logs
kind: observational
nature: computational
output_type: stream
uses: []
depends_on: [run-dev]
observe_window: "10s"
signal_matches:
  - key: served
    pattern: "GET /"
    verdict: pass
    expected: true
steps:
  - id: watch
    uses: run-dev
`)
}

// TestRunUseCaseSensors_SharedServiceCompletesAndTearsDown reproduces
// issue #34: validating a use case whose sensors depend on the shared
// run-dev observational service must (a) finish promptly instead of
// waiting for the never-exiting dev server, and (b) terminate the dev
// server's process once the last consumer detaches.
func TestRunUseCaseSensors_SharedServiceCompletesAndTearsDown(t *testing.T) {
	repoRoot := t.TempDir()
	pidPath := filepath.Join(t.TempDir(), "svc.pid")
	writeRepro(t, repoRoot, pidPath)

	b, err := BootLifecycle(repoRoot)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	defer func() { _ = b.Cleanup() }()

	gathered := b.Sensors.GatherForUseCase("health-check")
	if len(gathered) != 3 {
		t.Fatalf("gathered %d sensors, want 3 (run-dev + 2 consumers)", len(gathered))
	}

	type result struct {
		aggs      []aggSignal
		scheduled []string
		err       error
	}
	done := make(chan result, 1)
	go func() {
		aggs, scheduled, err := RunUseCaseSensors(context.Background(), b, gathered, 4)
		ids := make([]string, 0, len(scheduled))
		for _, s := range scheduled {
			ids = append(ids, s.ID)
		}
		simplified := make([]aggSignal, 0, len(aggs))
		for _, a := range aggs {
			simplified = append(simplified, aggSignal{SensorID: a.SensorID, Verdict: a.Verdict})
		}
		done <- result{aggs: simplified, scheduled: ids, err: err}
	}()

	var r result
	select {
	case r = <-done:
	case <-time.After(45 * time.Second):
		t.Fatal("RunUseCaseSensors hangs: shared run-dev service scheduled as a run-to-completion node (issue #34)")
	}
	if r.err != nil {
		t.Fatalf("RunUseCaseSensors: %v", r.err)
	}

	// The service is infrastructure: excluded from the scheduled node set
	// and from the per-sensor aggregates.
	for _, id := range r.scheduled {
		if id == "run-dev" {
			t.Fatal("run-dev must not be in the scheduled sensor set")
		}
	}
	verdicts := map[string]enums.Verdict{}
	for _, a := range r.aggs {
		verdicts[a.SensorID] = a.Verdict
	}
	if _, ok := verdicts["run-dev"]; ok {
		t.Fatal("run-dev must not emit a consumer-level aggregate")
	}
	if v := verdicts["s-health-check-http"]; v != enums.VerdictPass {
		t.Fatalf("http sensor verdict = %q, want pass", v)
	}
	if v := verdicts["s-health-check-logs"]; v != enums.VerdictPass {
		t.Fatalf("logs sensor verdict = %q, want pass (attach to run-dev stream)", v)
	}

	// Teardown: the service's shell must be dead once validation returns.
	pid := readPID(t, pidPath)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run-dev process %d still alive after validation returned (issue #34 orphan)", pid)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// And the lifecycle registry must hold no in-flight runs.
	running, err := b.Lifecycle.ListRunning()
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	if len(running) != 0 {
		t.Fatalf("registry still tracks %d running sensors, want 0: %+v", len(running), running)
	}
}

type aggSignal struct {
	SensorID string
	Verdict  enums.Verdict
}

func readPID(t *testing.T, pidPath string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		b, err := os.ReadFile(pidPath)
		if err == nil && len(b) > 0 {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(b)))
			if convErr != nil {
				t.Fatalf("parse pid file: %v", convErr)
			}
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatalf("pid file never written: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
