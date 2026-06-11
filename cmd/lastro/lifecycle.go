package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/lifecycle"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
)

func runRunSensor(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
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
		skillio.EmitError(stderr, "sensor-not-found", fmt.Sprintf("no sensor %q", sensorID), map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}
	if s.Kind == enums.KindObservational {
		skillio.EmitError(stderr, "wrong-kind", "use /start-sensor for observational sensors", map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}

	// Bring up the shared observational core services the sensor's
	// dependency closure declares (e.g. run-dev), run against them, then
	// tear them down — the lifecycle validate-use-case already provides.
	agg, err := skillruntime.RunSensorWithServices(context.Background(), b, s)
	if err != nil {
		if errors.Is(err, lifecycle.ErrSensorNotFound) {
			skillio.EmitError(stderr, "sensor-not-found", err.Error(), map[string]any{"sensor_id": sensorID})
			return skillio.ExitScriptError
		}
		skillio.EmitError(stderr, "run-failed", err.Error(), map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}
	if err := skillruntime.ReplaySignals(b.RuntimeRoot, sensorID, stdout); err != nil {
		skillio.EmitError(stderr, "replay-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	if err := skillio.EmitJSON(stdout, agg); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	return skillio.ExitCodeForVerdict(agg.Verdict)
}

func runStartSensor(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
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

	if skillruntime.IsWatchMode(args) {
		if err := skillruntime.RunWatcherMode(b, args); err != nil {
			skillio.EmitError(stderr, "watch-failed", err.Error(), nil)
			return skillio.ExitScriptError
		}
		return skillio.ExitPass
	}

	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected sensor-id as first argument", nil)
		return skillio.ExitScriptError
	}
	sensorID := args[1]
	s, ok := b.Sensors.LookupSensor(sensorID)
	if !ok {
		skillio.EmitError(stderr, "sensor-not-found", fmt.Sprintf("no sensor %q", sensorID), map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}
	if s.Kind != enums.KindObservational {
		skillio.EmitError(stderr, "wrong-kind", "use /run-sensor for assertion sensors", map[string]any{"sensor_id": sensorID})
		return skillio.ExitScriptError
	}
	h, err := skillruntime.StartDetachedSensor(b, repoRoot, sensorID, nil)
	if err != nil {
		skillio.EmitError(stderr, "start-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	out := map[string]any{
		"handle":  skillruntime.FormatHandle(h.SensorID, h.RunID),
		"run_dir": h.RunDir,
		"pid":     h.PID,
	}
	if err := skillio.EmitJSON(stdout, out); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	return skillio.ExitPass
}

func runStopSensor(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected '<sensor-id>:<run-id>' as first argument", nil)
		return skillio.ExitScriptError
	}
	handle := args[1]
	sensorID, runID, err := skillruntime.ParseHandle(handle)
	if err != nil {
		skillio.EmitError(stderr, "bad-handle", err.Error(), map[string]any{"input": handle})
		return skillio.ExitScriptError
	}
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
	h, err := b.Lifecycle.LoadHandle(sensorID, runID)
	if err != nil {
		if errors.Is(err, lifecycle.ErrSensorNotFound) {
			skillio.EmitError(stderr, "sensor-not-found", fmt.Sprintf("no in-flight handle for %s:%s", sensorID, runID), map[string]any{"handle": handle})
			return skillio.ExitScriptError
		}
		skillio.EmitError(stderr, "load-handle-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	agg, err := b.Lifecycle.StopSensor(context.Background(), h)
	if err != nil {
		skillio.EmitError(stderr, "stop-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	if err := skillio.EmitJSON(stdout, agg); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	return skillio.ExitCodeForVerdict(agg.Verdict)
}

func runTailSensorSignals(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected '<sensor-id>:<run-id>' as first argument", nil)
		return skillio.ExitScriptError
	}
	handle := args[1]
	fs := flag.NewFlagSet("tail-sensor-signals", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	follow := fs.Bool("follow", false, "tail new signals as they arrive")
	since := fs.Int("since", 0, "start emitting at line N (1-indexed; 0 = from the beginning)")
	if err := fs.Parse(args[2:]); err != nil {
		skillio.EmitError(stderr, "bad-argv", err.Error(), nil)
		return skillio.ExitScriptError
	}
	sensorID, runID, err := skillruntime.ParseHandle(handle)
	if err != nil {
		skillio.EmitError(stderr, "bad-handle", err.Error(), map[string]any{"input": handle})
		return skillio.ExitScriptError
	}
	repoRoot, err := skillio.FindRepoRoot(cwd)
	if err != nil {
		skillio.EmitError(stderr, "repo-root-not-found", err.Error(), nil)
		return skillio.ExitScriptError
	}
	harnessDir := skillio.HarnessDir(repoRoot)
	signalsPath := filepath.Join(harnessDir, "runtime", sensorID, runID, "signals.jsonl")
	if *follow {
		if err := tailFollowLoop(signalsPath, *since, sensorID, runID, harnessDir, stdout); err != nil {
			skillio.EmitError(stderr, "follow-failed", err.Error(), map[string]any{"path": signalsPath})
			return skillio.ExitScriptError
		}
		return skillio.ExitPass
	}
	if _, err := tailSnapshot(signalsPath, *since, stdout); err != nil {
		skillio.EmitError(stderr, "read-failed", err.Error(), map[string]any{"path": signalsPath})
		return skillio.ExitScriptError
	}
	return skillio.ExitPass
}

// tailSnapshot reads signals.jsonl skipping the first `since` lines.
// Returns the number of lines emitted.
func tailSnapshot(path string, since int, stdout io.Writer) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	idx, emitted := 0, 0
	for sc.Scan() {
		idx++
		if since > 0 && idx < since {
			continue
		}
		if _, err := stdout.Write(sc.Bytes()); err != nil {
			return emitted, err
		}
		if _, err := stdout.Write([]byte{'\n'}); err != nil {
			return emitted, err
		}
		emitted++
	}
	return emitted, sc.Err()
}

// tailFollowLoop tails signalsPath: emits existing lines (after `since`),
// then polls every 200ms until the sensor leaves running_sensors.json AND
// no new bytes have arrived for 1 s.
func tailFollowLoop(signalsPath string, since int, sensorID, runID, harnessDir string, stdout io.Writer) error {
	if _, err := tailSnapshot(signalsPath, since, stdout); err != nil {
		return err
	}
	f, err := os.Open(signalsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	const pollInterval = 200 * time.Millisecond
	const quietWindow = 1 * time.Second
	registryPath := filepath.Join(harnessDir, "runtime", "running_sensors.json")
	lastEmittedAt := time.Now()
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if _, werr := stdout.Write([]byte(line)); werr != nil {
				return werr
			}
			lastEmittedAt = time.Now()
		}
		if err == nil {
			continue
		}
		if err != io.EOF {
			return err
		}
		alive := tailIsInRegistry(registryPath, sensorID, runID)
		if !alive && time.Since(lastEmittedAt) > quietWindow {
			return nil
		}
		time.Sleep(pollInterval)
	}
}

func tailIsInRegistry(path, sensorID, runID string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc struct {
		Entries []struct {
			SensorID string `json:"sensor_id"`
			RunID    string `json:"run_id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	for _, e := range doc.Entries {
		if e.SensorID == sensorID && e.RunID == runID {
			return true
		}
	}
	return false
}
