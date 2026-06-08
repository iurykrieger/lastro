package skillruntime

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
)

// ReplaySignals streams the most recent signals.jsonl for sensorID to w.
// Best-effort: a missing file is not an error (single-shot sensors emit
// zero streamed signals before producing the terminal aggregate).
// Run IDs are ULIDs and sort lexically by creation time, so the
// alphabetically-greatest subdirectory of runtimeRoot/sensorID is the
// most recent run.
func ReplaySignals(runtimeRoot, sensorID string, w io.Writer) error {
	sensorDir := filepath.Join(runtimeRoot, sensorID)
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
	f, err := os.Open(filepath.Join(sensorDir, latest, "signals.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if _, err := w.Write(sc.Bytes()); err != nil {
			return err
		}
		if _, err := w.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return sc.Err()
}
