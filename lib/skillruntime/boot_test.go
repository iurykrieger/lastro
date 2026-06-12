package skillruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeSensorBin is set by TestMain, which builds the fakesensor helper once
// before any tests run. The EnvFile probe test needs it to run a real sensor.
var fakeSensorBin string

func TestMain(m *testing.M) {
	if os.Getenv("HARNESS_TEST_CHILD") != "1" {
		bin, err := buildFakeSensor()
		if err != nil {
			panic("build fakesensor: " + err.Error())
		}
		fakeSensorBin = bin
		defer os.RemoveAll(filepath.Dir(bin))
	}
	os.Exit(m.Run())
}

func buildFakeSensor() (string, error) {
	dir, err := os.MkdirTemp("", "fakesensor-boot-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "fakesensor")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "../../internal/testutil/fakesensor/main.go")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

// TestBootLifecycle_EmptyHarness verifies the function does not blow up
// on an .harness/ tree containing empty sensors/, fixtures/, and
// use-cases/ directories. This is the minimum smoke test.
func TestBootLifecycle_EmptyHarness(t *testing.T) {
	repo := t.TempDir()
	for _, sub := range []string{"sensors", "fixtures", "use-cases", "runtime"} {
		if err := os.MkdirAll(filepath.Join(repo, ".harness", sub), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	b, err := BootLifecycle(repo)
	if err != nil {
		t.Fatalf("BootLifecycle: %v", err)
	}
	defer func() {
		if cerr := b.Cleanup(); cerr != nil {
			t.Errorf("Cleanup: %v", cerr)
		}
	}()

	if b.Lifecycle == nil {
		t.Errorf("Lifecycle is nil")
	}
	if b.Sensors == nil {
		t.Errorf("Sensors is nil")
	}
	if b.Fixtures == nil {
		t.Errorf("Fixtures is nil")
	}
	if b.UseCases == nil {
		t.Errorf("UseCases is nil")
	}
}

func TestBootLifecycle_MissingHarness(t *testing.T) {
	repo := t.TempDir()
	_, err := BootLifecycle(repo)
	if err == nil {
		t.Errorf("expected error on missing .harness/")
	}
}

func TestBootLifecycle_InvalidManifestErrors(t *testing.T) {
	repo := t.TempDir()
	harness := filepath.Join(repo, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatal(err)
	}
	// Present-but-invalid manifest must surface, not be silently ignored.
	if err := os.WriteFile(filepath.Join(harness, "stack-manifest.yaml"), []byte("archetype: [not, a, string]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BootLifecycle(repo); err == nil {
		t.Error("invalid stack manifest should fail boot")
	}
}

func TestBootLifecycle_ManifestAbsentStillBoots(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := BootLifecycle(repo); err != nil {
		t.Errorf("boot without a manifest must keep working: %v", err)
	}
}

// TestBootLifecycle_EnvFileReachesExecutor proves that a stack manifest's
// env_file is carried all the way through BootLifecycle into the executor so
// that sensor steps can see the declared env var.
//
// Approach (a): run a real sensor through Booted.Lifecycle.RunSensor. The
// step uses a shell conditional — if $HARNESS_BOOT_TOKEN equals the expected
// value, fakesensor emits pass; otherwise fail. This exercises the full path
// without exposing the secret value in any signal field (the redactor masks
// env_file values, but the verdict itself is unmasked).
//
// Approach (b) — exposing an accessor — was ruled out because it adds API
// surface purely for testing. The real user-visible contract is that the env
// var arrives in the sensor process, and (a) verifies exactly that.
func TestBootLifecycle_EnvFileReachesExecutor(t *testing.T) {
	if fakeSensorBin == "" {
		t.Skip("fakesensor binary not available (child re-exec?)")
	}

	repo := t.TempDir()
	harness := filepath.Join(repo, ".harness")
	if err := os.MkdirAll(harness, 0o755); err != nil {
		t.Fatalf("mkdir harness: %v", err)
	}

	// Write .env with a sentinel value.
	envFile := filepath.Join(repo, ".env")
	if err := os.WriteFile(envFile, []byte("HARNESS_BOOT_TOKEN=bootvalue\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	// Write a minimal but valid stack manifest that declares env_file: .env
	// The manifest must pass JSON-Schema + programmatic validation; use a
	// known-valid cli archetype with one stub component.
	manifest := `schema_version: 1.0.0
archetype: cli
env_file: .env
applicable_angles:
  - security
  - build
  - code-structure
  - unit-test
  - contracts
  - logs
components:
  - schema_version: 1.0.0
    id: go-stdlib
    kind: runtime
    name: go
    version: "1.24"
    capabilities:
      - cli-entrypoint
    detection_evidence:
      - file: go.mod
        path: go
        value: "1.24.2"
`
	if err := os.WriteFile(filepath.Join(harness, "stack-manifest.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	// Write a sensor YAML referencing fakesensor. The run command is
	// injected with the built binary path, which is safe here because the
	// path comes from t.TempDir() (no shell-special characters).
	sensorYAML := fmt.Sprintf(`schema_version: 1.0.0
id: boot-env-inject
use_case_id: boot-uc
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses:
  - fake
steps:
  - id: only
    run: "if [ \"$HARNESS_BOOT_TOKEN\" = \"bootvalue\" ]; then %s signal pass; else %s signal fail; fi"
`, fakeSensorBin, fakeSensorBin)

	sensorsDir := filepath.Join(harness, "sensors")
	if err := os.MkdirAll(sensorsDir, 0o755); err != nil {
		t.Fatalf("mkdir sensors: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sensorsDir, "boot-env-inject.yaml"), []byte(sensorYAML), 0o600); err != nil {
		t.Fatalf("write sensor yaml: %v", err)
	}

	// Write a minimal use case so the executor's UseCaseLookup resolves.
	useCaseYAML := `schema_version: 2.0.0
id: boot-uc
title: "Boot env injection smoke test"
archetype_scope: [cli]
entry_points:
  - id: boot-ep
    archetype: cli
    spec:
      command: "boot-test"
given:
  - "The app is running"
when:
  - "A sensor step executes"
then:
  - "The env var from env_file is visible"
source_refs:
  - path: main.go
    symbol: main
    reason: "test stub"
fixture_ids: []
`
	useCasesDir := filepath.Join(harness, "use-cases")
	if err := os.MkdirAll(useCasesDir, 0o755); err != nil {
		t.Fatalf("mkdir use-cases: %v", err)
	}
	if err := os.WriteFile(filepath.Join(useCasesDir, "boot-uc.yaml"), []byte(useCaseYAML), 0o600); err != nil {
		t.Fatalf("write use-case yaml: %v", err)
	}

	b, err := BootLifecycle(repo)
	if err != nil {
		t.Fatalf("BootLifecycle: %v", err)
	}
	defer func() { _ = b.Cleanup() }()

	agg, err := b.Lifecycle.RunSensor(context.Background(), "boot-env-inject", nil)
	if err != nil {
		t.Fatalf("RunSensor: %v", err)
	}
	if agg.Verdict != "pass" {
		t.Errorf("verdict = %q, want pass (env_file not carried through BootLifecycle to executor)", agg.Verdict)
	}
}
