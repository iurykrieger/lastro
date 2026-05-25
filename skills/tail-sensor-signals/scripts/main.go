// Command tail-sensor-signals backs the /tail-sensor-signals skill.
// See skills/tail-sensor-signals/skill.md.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

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

	fs := flag.NewFlagSet("tail-sensor-signals", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // suppress default error spam; we handle errors ourselves
	follow := fs.Bool("follow", false, "tail new signals as they arrive")
	since := fs.Int("since", 0, "skip the first N lines (1-indexed; 0 = no skip)")
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

	signalsPath := filepath.Join(skillio.HarnessDir(repoRoot), "runtime", sensorID, runID, "signals.jsonl")

	if *follow {
		if err := followLoop(signalsPath, *since, sensorID, runID, skillio.HarnessDir(repoRoot), stdout); err != nil {
			skillio.EmitError(stderr, "follow-failed", err.Error(), map[string]any{"path": signalsPath})
			return skillio.ExitScriptError
		}
		return skillio.ExitPass
	}

	if _, err := snapshot(signalsPath, *since, stdout); err != nil {
		skillio.EmitError(stderr, "read-failed", err.Error(), map[string]any{"path": signalsPath})
		return skillio.ExitScriptError
	}
	return skillio.ExitPass
}

// snapshot reads signals.jsonl from the beginning, skipping the first
// `since` lines if since > 0, and writes the rest to stdout (each
// followed by '\n'). Returns the number of lines emitted.
func snapshot(path string, since int, stdout io.Writer) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	idx := 0
	emitted := 0
	for scanner.Scan() {
		idx++
		if since > 0 && idx < since {
			continue
		}
		if _, err := stdout.Write(scanner.Bytes()); err != nil {
			return emitted, err
		}
		if _, err := stdout.Write([]byte{'\n'}); err != nil {
			return emitted, err
		}
		emitted++
	}
	return emitted, scanner.Err()
}

// followLoop tails signalsPath: emits existing lines (after `since`), then
// polls for new bytes every 200ms until the sensor leaves
// running_sensors.json AND no new bytes have arrived for 1s.
func followLoop(signalsPath string, since int, sensorID, runID, harnessDir string, stdout io.Writer) error {
	// First emit the existing snapshot.
	if _, err := snapshot(signalsPath, since, stdout); err != nil {
		return err
	}

	// Reopen for incremental reads.
	f, err := os.Open(signalsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	// Seek to end of what snapshot already emitted.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	const pollInterval = 200 * time.Millisecond
	const quietWindow = 1 * time.Second
	registryPath := filepath.Join(harnessDir, "runtime", "running_sensors.json")
	// Initialize to now so the quiet-window check is relative to when we
	// started tailing, not to the zero time (which would cause immediate exit).
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
		// EOF: decide whether to keep waiting.
		alive := isSensorInRegistry(registryPath, sensorID, runID)
		if !alive && time.Since(lastEmittedAt) > quietWindow {
			return nil
		}
		time.Sleep(pollInterval)
	}
}

// isSensorInRegistry returns true if running_sensors.json contains an
// entry for (sensorID, runID). Missing file or parse error → false.
func isSensorInRegistry(path, sensorID, runID string) bool {
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
