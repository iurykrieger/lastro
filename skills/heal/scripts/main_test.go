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
	dir, err := os.MkdirTemp("", "fakesensor-heal-")
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

func setupHarness(t *testing.T, sensorRun string) string {
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
	sensor := `schema_version: 1.0.0
id: test-build
use_case_id: test-uc
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses: [fake]
steps:
  - id: only
    run: "` + sensorRun + `"
`
	if err := os.WriteFile(filepath.Join(root, ".harness", "sensors", "test-build.yaml"), []byte(sensor), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return root
}

func TestRun_BadArgv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"heal"}, strings.NewReader(`{}`), &stdout, &stderr, t.TempDir())
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}

func TestRun_BadStdin(t *testing.T) {
	root := setupHarness(t, fakeSensorBin+" signal pass")
	var stdout, stderr bytes.Buffer
	code := run([]string{"heal", "test-uc"}, strings.NewReader(`not json`), &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "bad-edit-plan") {
		t.Errorf("stderr missing bad-edit-plan: %q", stderr.String())
	}
}

func TestRun_BadEditPath_DotDot(t *testing.T) {
	root := setupHarness(t, fakeSensorBin+" signal pass")
	plan := `{"Files":[{"Path":"../escape.txt","Op":"write","Content":"x"}],"Rationale":"x"}`
	var stdout, stderr bytes.Buffer
	code := run([]string{"heal", "test-uc"}, strings.NewReader(plan), &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "bad-edit-plan") {
		t.Errorf("stderr missing bad-edit-plan: %q", stderr.String())
	}
}

func TestRun_HealSucceeds_EditCommitted(t *testing.T) {
	root := setupHarness(t, fakeSensorBin+" signal pass")
	plan := `{"Files":[{"Path":"newfile.txt","Op":"write","Content":"hello"}],"Rationale":"add file"}`
	var stdout, stderr bytes.Buffer
	code := run([]string{"heal", "test-uc"}, strings.NewReader(plan), &stdout, &stderr, root)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "newfile.txt")); err != nil {
		t.Errorf("expected newfile.txt to remain on disk after heal: %v", err)
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) < 1 {
		t.Fatalf("no stdout")
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &env); err != nil {
		t.Fatalf("envelope not JSON: %v; stdout=%q", err, stdout.String())
	}
	if env["status"] != "healed" {
		t.Errorf("status = %v, want healed", env["status"])
	}
}

func TestRun_HealFails_EditReverted(t *testing.T) {
	root := setupHarness(t, fakeSensorBin+" signal fail --summary fake")
	plan := `{"Files":[{"Path":"newfile.txt","Op":"write","Content":"hello"}],"Rationale":"add file"}`
	var stdout, stderr bytes.Buffer
	code := run([]string{"heal", "test-uc"}, strings.NewReader(plan), &stdout, &stderr, root)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "newfile.txt")); !os.IsNotExist(err) {
		t.Errorf("expected newfile.txt to be reverted; stat err=%v", err)
	}
	statePath := filepath.Join(root, ".harness", "runtime", "heal-state.json")
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("heal-state.json missing: %v", err)
	}
	if !strings.Contains(string(state), `"iteration": 1`) {
		t.Errorf("expected iteration: 1 in heal-state.json; got %s", string(state))
	}
}

func TestRun_HealExhausted(t *testing.T) {
	root := setupHarness(t, fakeSensorBin+" signal fail --summary fake")
	stateDir := filepath.Join(root, ".harness", "runtime")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	state := `{"use_case_id":"test-uc","iteration":3,"max_iterations":3,"history":[]}`
	if err := os.WriteFile(filepath.Join(stateDir, "heal-state.json"), []byte(state), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	plan := `{"Files":[{"Path":"newfile.txt","Op":"write","Content":"x"}],"Rationale":"x"}`
	var stdout, stderr bytes.Buffer
	code := run([]string{"heal", "test-uc"}, strings.NewReader(plan), &stdout, &stderr, root)
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "heal-exhausted") {
		t.Errorf("stderr missing heal-exhausted: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "newfile.txt")); !os.IsNotExist(err) {
		t.Errorf("exhausted run must not apply the edit; file exists")
	}
}
