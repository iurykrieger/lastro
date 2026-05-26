//go:build integration

// Run with: go test -tags=integration ./skills/...
package skills_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// repoRoot walks up from the test working directory to find the lastro
// repo root (the dir containing go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
}

func goRun(t *testing.T, ctx context.Context, scriptDir string, args ...string) (string, string, int) {
	t.Helper()
	full := append([]string{"run", "./" + scriptDir}, args...)
	cmd := exec.CommandContext(ctx, "go", full...)
	cmd.Dir = repoRoot(t)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("go run failed (not ExitError): %v; stderr=%s", err, stderr.String())
	}
	return stdout.String(), stderr.String(), code
}

func TestSkills_RunSensor_PassingAssertion(t *testing.T) {
	// Assumes the user has set up a known-good .harness/ on disk for this
	// integration test, OR we create one inline. For B5 sub-PR 1, we
	// create one inline because no dogfood examples exist yet.
	t.Skip("Integration test stub. Populate with a t.TempDir() harness layout including a known-passing assertion sensor + use case + fixture. Build fakesensor binary, write sensor YAML referencing it. Run /run-sensor and assert exit=0, stdout last line is aggregate with verdict=pass. Wire into CI with -tags=integration only.")
}

func TestSkills_StartTailStop_Observational(t *testing.T) {
	t.Skip("Integration test stub. Populate with a t.TempDir() harness layout for a known-good observational sensor. Spawn /start-sensor, capture handle. In parallel: /tail-sensor-signals --follow and /stop-sensor handle. Assert tail exits within 1.5s of stop and emitted at least 1 signal.")
}

// Compile-time guards that the test file participates in `go build -tags=integration`.
var (
	_ = json.Marshal
	_ = strings.TrimSpace
	_ = time.Now
)
