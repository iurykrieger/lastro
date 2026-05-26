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
	// fakesensor lives at <repo>/internal/testutil/fakesensor/main.go.
	// The skill script tests run from skills/run-sensor/scripts/, so the
	// relative path is ../../../internal/testutil/fakesensor/main.go.
	dir, err := os.MkdirTemp("", "fakesensor-runsensor-")
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

// useCaseYAML is the minimal valid use case the test harness writes
// alongside any sensor. The sensor YAML's use_case_id MUST be "test-uc"
// for the executor's UseCaseLookup to succeed.
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

// setupHarness writes a minimal .harness/ layout with one sensor + a
// matching use case. Returns the repo root.
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

func TestRun_PassingAssertion(t *testing.T) {
	sensorYAML := `schema_version: 1.0.0
id: test-pass
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
	code := run([]string{"run-sensor", "test-pass"}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}
	// last line should be the terminal aggregate
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatalf("no stdout")
	}
	var agg map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &agg); err != nil {
		t.Fatalf("terminal line not JSON: %v (%q)", err, lines[len(lines)-1])
	}
	if agg["verdict"] != "pass" {
		t.Errorf("verdict = %v, want pass", agg["verdict"])
	}
}

func TestRun_SensorNotFound(t *testing.T) {
	root := setupHarness(t, "") // setupHarness writes only the use case when sensorYAML is empty
	var stdout, stderr bytes.Buffer
	code := run([]string{"run-sensor", "no-such"}, nil, &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "sensor-not-found") && !strings.Contains(stderr.String(), "no-such") {
		t.Errorf("stderr missing context: %q", stderr.String())
	}
}

func TestRun_BadArgv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"run-sensor"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}
