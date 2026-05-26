// Command stop-sensor backs the /stop-sensor skill.
// See skills/stop-sensor/skill.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

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

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
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
