// Command start-sensor backs the /start-sensor skill.
// See skills/start-sensor/skill.md.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/iurykrieger/lastro/internal/enums"
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

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
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

	// Detached-watcher mode: this process IS the watcher. Run the sensor to
	// completion in-process (writing raw.log/signals/aggregate) and exit.
	// Output goes to <runDir>/watcher.log via the parent's redirection.
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
		skillio.EmitError(stderr, "wrong-kind", "use /run-sensor for assertion sensors", map[string]any{"sensor_id": sensorID, "kind": string(s.Kind)})
		return skillio.ExitScriptError
	}

	// F3: ExpectedObservations is currently not a field on Sensor; pass nil.
	// When B4 adds it, populate from s here.
	var expectedObs []string

	// Spawn a detached watcher process and return its handle immediately.
	// The watcher outlives this process; /stop-sensor terminates it.
	h, err := skillruntime.StartDetachedSensor(b, repoRoot, sensorID, expectedObs)
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
