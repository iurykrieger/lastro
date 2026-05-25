// Command run-sensor is the Go binary backing the /run-sensor skill.
// See skills/run-sensor/skill.md for the contract.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/lifecycle"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		skillio.EmitError(os.Stderr, "cwd-failed", err.Error(), nil)
		os.Exit(skillio.ExitScriptError)
	}
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr, cwd))
}

// run is the testable entry point. args is the full os.Args slice; cwd
// is the working directory used for repo-root discovery.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected sensor-id as first argument", nil)
		return skillio.ExitScriptError
	}
	sensorID := args[1]

	repoRoot, err := skillio.FindRepoRoot(cwd)
	if err != nil {
		skillio.EmitError(stderr, "repo-root-not-found", err.Error(), nil)
		return skillio.ExitScriptError
	}

	b, err := skillruntime.BootLifecycle(repoRoot)
	if err != nil {
		skillio.EmitError(stderr, "boot-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	defer func() { _ = b.Cleanup() }()

	s, ok := b.Sensors.LookupSensor(sensorID)
	if !ok {
		skillio.EmitError(stderr, "sensor-not-found", fmt.Sprintf("no sensor %q in .harness/sensors/", sensorID), map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}
	if s.Kind == enums.KindObservational {
		skillio.EmitError(stderr, "wrong-kind", "use /start-sensor for observational sensors", map[string]any{"sensor_id": sensorID, "kind": string(s.Kind)})
		return skillio.ExitScriptError
	}

	ctx := context.Background()
	agg, err := b.Lifecycle.RunSensor(ctx, sensorID, nil)
	if err != nil {
		if errors.Is(err, lifecycle.ErrSensorNotFound) {
			skillio.EmitError(stderr, "sensor-not-found", err.Error(), map[string]any{"sensor_id": sensorID})
			return skillio.ExitScriptError
		}
		skillio.EmitError(stderr, "run-failed", err.Error(), map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}

	if err := replaySignals(b, sensorID, stdout); err != nil {
		skillio.EmitError(stderr, "replay-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	if err := skillio.EmitJSON(stdout, agg); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	return skillio.ExitCodeForVerdict(agg.Verdict)
}

// replaySignals streams the most recent per-run signals.jsonl to stdout.
// Best effort: missing file is not an error (single-shot sensors may
// emit zero streamed signals). The terminal aggregate is appended by
// the caller.
//
// Run-dir discovery: <RuntimeRoot>/<sensor-id>/<run-id>/signals.jsonl.
// ULID run ids sort lexically by time, so the alphabetically-greatest
// subdir is the most recent run — which is the one RunSensor just
// completed (RunSensor blocks until the run finishes).
func replaySignals(b *skillruntime.Booted, sensorID string, stdout io.Writer) error {
	sensorDir := filepath.Join(b.RuntimeRoot, sensorID)
	entries, err := os.ReadDir(sensorDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var latest string
	for _, e := range entries {
		if e.IsDir() && (latest == "" || e.Name() > latest) {
			latest = e.Name()
		}
	}
	if latest == "" {
		return nil
	}
	signalsPath := filepath.Join(sensorDir, latest, "signals.jsonl")
	f, err := os.Open(signalsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // allow long lines
	for scanner.Scan() {
		if _, err := stdout.Write(scanner.Bytes()); err != nil {
			return err
		}
		if _, err := stdout.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return scanner.Err()
}
