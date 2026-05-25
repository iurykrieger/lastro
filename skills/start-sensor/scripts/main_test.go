package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/lib/skillruntime"
)

var fakeSensorBin string

func TestMain(m *testing.M) {
	bin, err := buildFakeSensor()
	if err != nil {
		panic("build fakesensor: " + err.Error())
	}
	fakeSensorBin = bin
	defer os.Remove(bin)
	os.Exit(m.Run())
}

func buildFakeSensor() (string, error) {
	dir, err := os.MkdirTemp("", "fakesensor-startsensor-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "fakesensor")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "../../../internal/testutil/fakesensor/main.go")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

// useCaseYAML matches sensor.use_case_id "test-uc" so BootLifecycle's
// executor.UseCaseLookup closure can resolve the sensor to a use case.
const useCaseYAML = `schema_version: 2.0.0
id: test-uc
title: Test use case
archetype_scope: [http-api]
entry_points:
  - id: test-ep
    archetype: http-api
    spec:
      method: GET
      path: /test
given:
  - "a request"
when:
  - "the test runs"
then:
  - "it passes"
`

func setupHarness(t *testing.T, sensorYAML string) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"sensors", "fixtures", "use-cases", "runtime"} {
		if err := os.MkdirAll(filepath.Join(root, ".harness", sub), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	if sensorYAML != "" {
		if err := os.WriteFile(filepath.Join(root, ".harness", "sensors", "s.yaml"), []byte(sensorYAML), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".harness", "use-cases", "test-uc.yaml"), []byte(useCaseYAML), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return root
}

func killPID(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func TestRun_RejectsAssertionKind(t *testing.T) {
	sensorYAML := `schema_version: 1.0.0
id: test-assertion
use_case_id: test-uc
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal pass"
`
	root := setupHarness(t, sensorYAML)
	var stdout, stderr bytes.Buffer
	code := run([]string{"start-sensor", "test-assertion"}, nil, &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "wrong-kind") {
		t.Errorf("stderr missing wrong-kind: %q", stderr.String())
	}
}

// TestRun_ObservationalSpawnsAndEmitsHandle uses the fakesensor "watch"
// subcommand (the observational equivalent that emits then waits for
// SIGTERM). The test kills the spawned PID to clean up.
func TestRun_ObservationalSpawnsAndEmitsHandle(t *testing.T) {
	sensorYAML := `schema_version: 1.0.0
id: test-obs
use_case_id: test-uc
angle: logs
kind: observational
nature: computational
output_type: stream
uses: [fake]
steps:
  - id: watch
    run: "` + fakeSensorBin + ` watch --emit order-received --interval 30ms"
`
	root := setupHarness(t, sensorYAML)
	var stdout, stderr bytes.Buffer
	code := run([]string{"start-sensor", "test-obs"}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &out); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, stdout.String())
	}
	handle, _ := out["handle"].(string)
	if !strings.Contains(handle, ":") {
		t.Errorf("handle missing colon: %q", handle)
	}
	sensorID, runID, perr := skillruntime.ParseHandle(handle)
	if perr != nil {
		t.Fatalf("handle does not parse: %v (%q)", perr, handle)
	}
	if sensorID != "test-obs" {
		t.Errorf("handle sensor-id = %q, want %q", sensorID, "test-obs")
	}
	if len(runID) != 26 {
		t.Errorf("handle run-id length = %d, want 26", len(runID))
	}
	// Cleanup: the watcher continues running after start-sensor returns;
	// kill it so the test doesn't leak processes.
	if pid, ok := out["pid"].(float64); ok {
		_ = killPID(int(pid))
	}
}

func TestRun_BadArgv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"start-sensor"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}
