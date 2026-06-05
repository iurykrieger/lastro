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
	dir, err := os.MkdirTemp("", "fakesensor-validate-")
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

// setupHarness writes a .harness/ layout with the test-uc use case and
// the provided sensors. sensors maps "file-name" → YAML body.
func setupHarness(t *testing.T, sensors map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"sensors", "fixtures", "use-cases", "runtime"} {
		if err := os.MkdirAll(filepath.Join(root, ".harness", sub), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".harness", "use-cases", "test-uc.yaml"), []byte(useCaseYAML), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for name, body := range sensors {
		if err := os.WriteFile(filepath.Join(root, ".harness", "sensors", name+".yaml"), []byte(body), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return root
}

func TestRun_BadArgv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-use-case"}, nil, &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}

func TestRun_UseCaseNotFound(t *testing.T) {
	root := setupHarness(t, nil)
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-use-case", "no-such-uc"}, nil, &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit = %d, want 3; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "use-case-not-found") {
		t.Errorf("stderr missing use-case-not-found: %q", stderr.String())
	}
}

func TestRun_TwoPassingSensors(t *testing.T) {
	sBuild := `schema_version: 1.0.0
id: test-build
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
	sUnit := `schema_version: 1.0.0
id: test-unit
use_case_id: test-uc
angle: unit-test
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
depends_on: [test-build]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal pass"
`
	root := setupHarness(t, map[string]string{"test-build": sBuild, "test-unit": sUnit})

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-use-case", "test-uc"}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("got %d lines, want ≥3: %q", len(lines), stdout.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &envelope); err != nil {
		t.Fatalf("verdict envelope not JSON: %v (%q)", err, lines[len(lines)-1])
	}
	verdict, _ := envelope["use_case_verdict"].(map[string]any)
	if verdict == nil || verdict["Verdict"] != "pass" {
		// UseCaseVerdict's Verdict field has no json tag; encoder uses "Verdict" capitalized
		t.Errorf("verdict = %v, want pass (envelope=%v)", verdict, envelope)
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".harness", "runtime", "use-cases", "test-uc", "*", "verdict.json"))
	if len(matches) != 1 {
		t.Errorf("verdict.json count = %d, want 1", len(matches))
	}
}

func TestRun_CoreSensorIncludedViaGather(t *testing.T) {
	// Core sensor: scope=core, no use_case_id, environment angle.
	// Written flat into sensors/ — LoadDirectory handles both flat and subfolder layouts.
	sCore := `schema_version: 1.0.0
id: core-run-dev
scope: core
angle: environment
kind: assertion
nature: computational
output_type: single-shot
uses: []
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal pass"
`
	// Use-case e2e sensor that depends on the core sensor.
	sE2e := `schema_version: 1.0.0
id: test-e2e
use_case_id: test-uc
angle: e2e-test
kind: assertion
nature: computational
output_type: single-shot
uses: []
depends_on: [core-run-dev]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal pass"
`
	root := setupHarness(t, map[string]string{"core-run-dev": sCore, "test-e2e": sE2e})

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-use-case", "test-uc"}, nil, &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s; stdout=%s", code, stderr.String(), stdout.String())
	}
	// core-run-dev must appear in stdout: GatherForUseCase pulls it in
	// via the depends_on edge and the lifecycle runs it.
	if !strings.Contains(stdout.String(), "core-run-dev") {
		t.Errorf("core sensor not included in run output; stdout=%q", stdout.String())
	}
	// Both sensors' AggregateSignals should be present.
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected ≥3 lines (2 sensors + verdict), got %d: %q", len(lines), stdout.String())
	}
}

func TestRun_DependencyFailedSkipsDependent(t *testing.T) {
	sBuild := `schema_version: 1.0.0
id: test-build
use_case_id: test-uc
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal fail --summary build-failure"
`
	sUnit := `schema_version: 1.0.0
id: test-unit
use_case_id: test-uc
angle: unit-test
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
depends_on: [test-build]
steps:
  - id: only
    run: "` + fakeSensorBin + ` signal pass"
`
	root := setupHarness(t, map[string]string{"test-build": sBuild, "test-unit": sUnit})

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate-use-case", "test-uc"}, nil, &stdout, &stderr, root)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (fail); stderr=%s; stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "skipped: depends_on test-build failed") {
		t.Errorf("expected dependency-skip heal_hint in stdout: %q", stdout.String())
	}
}
