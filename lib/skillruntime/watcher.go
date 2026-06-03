package skillruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/iurykrieger/lastro/internal/lifecycle"
	"github.com/iurykrieger/lastro/internal/runtime/process"
)

// WatchModeArg is the hidden first argument that puts the /start-sensor
// binary into detached-watcher mode. In that mode the binary does not
// spawn anything; it runs the sensor to completion in-process via
// lifecycle.RunWatcher.
const WatchModeArg = "__watch"

// watcherRegisterTimeout bounds how long StartDetachedSensor waits for the
// spawned watcher to register itself in running_sensors.json.
const watcherRegisterTimeout = 10 * time.Second

// IsWatchMode reports whether argv selects detached-watcher mode.
func IsWatchMode(argv []string) bool {
	return len(argv) >= 2 && argv[1] == WatchModeArg
}

// RunWatcherMode parses "__watch <sensorID> <runID> <runDir> [obs...]" and
// runs the sensor synchronously to completion in the current process. It is
// invoked by the detached watcher process that StartDetachedSensor spawns.
func RunWatcherMode(b *Booted, argv []string) error {
	if len(argv) < 5 {
		return fmt.Errorf("skillruntime: watch mode expects %s <sensorID> <runID> <runDir> [obs...]", WatchModeArg)
	}
	sensorID, runID, runDir := argv[2], argv[3], argv[4]
	expectedObs := argv[5:]
	if len(expectedObs) == 0 {
		expectedObs = nil
	}
	return b.Lifecycle.RunWatcher(context.Background(), sensorID, runID, runDir, expectedObs)
}

// StartDetachedSensor spawns a detached watcher OS process that runs the
// observational sensor to completion, then polls running_sensors.json until
// the watcher has registered itself, and returns its Handle.
//
// The watcher re-execs the current binary in WatchModeArg mode. It survives
// this process's exit because it is placed in its own process group and its
// stdio is redirected to <runDir>/watcher.log rather than inherited pipes
// that close when the parent exits.
func StartDetachedSensor(b *Booted, repoRoot, sensorID string, expectedObs []string) (lifecycle.Handle, error) {
	runID := b.Lifecycle.GenerateRunID()
	runDir := b.Lifecycle.RunDirFor(sensorID, runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return lifecycle.Handle{}, fmt.Errorf("skillruntime: mkdir runDir: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return lifecycle.Handle{}, fmt.Errorf("skillruntime: locate executable: %w", err)
	}

	logPath := filepath.Join(runDir, "watcher.log")
	logf, err := os.Create(logPath)
	if err != nil {
		return lifecycle.Handle{}, fmt.Errorf("skillruntime: create watcher.log: %w", err)
	}
	defer logf.Close()

	args := append([]string{WatchModeArg, sensorID, runID, runDir}, expectedObs...)
	cmd := exec.Command(exe, args...)
	cmd.Dir = repoRoot
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	// Place the watcher in its own process group so it both survives the
	// parent's exit and can be targeted as a group by a cross-process
	// StopSensor (SignalGroup(-PGID)).
	if err := process.Default().Spawn(cmd); err != nil {
		return lifecycle.Handle{}, fmt.Errorf("skillruntime: spawn setup: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return lifecycle.Handle{}, fmt.Errorf("skillruntime: start watcher: %w", err)
	}
	// Detach: we never Wait on the watcher; it outlives us.
	_ = cmd.Process.Release()

	deadline := time.Now().Add(watcherRegisterTimeout)
	for time.Now().Before(deadline) {
		if h, ok, err := b.Lifecycle.FindRunning(sensorID, runID); err == nil && ok {
			return h, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return lifecycle.Handle{}, fmt.Errorf(
		"skillruntime: watcher did not register within %s (see %s)", watcherRegisterTimeout, logPath)
}
