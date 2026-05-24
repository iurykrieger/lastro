package executor

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/iurykrieger/lastro/internal/signal"
)

// maxStdoutLineBytes caps a single stdout line. Mirrors
// signal.maxSignalLineBytes (1 MiB).
const maxStdoutLineBytes = 1 << 20

// pumpOutput is the result of consuming a step's stdout: the successfully
// decoded signals (in arrival order) and the observation keys extracted
// from their evidence (observational sensors only; nil otherwise).
type pumpOutput struct {
	Signals         []signal.Signal
	ObservationKeys []string
}

// pumpStdout reads r line-by-line, writes each line to rl with stream
// "stdout", attempts to decode each non-empty line as a Signal, appends
// successful decodes to the returned slice, and tees the raw bytes to
// signalsJSONL. Decode failures are logged to rl with stream
// "parse-error" and skipped.
//
// If observational is true and a Signal's evidence carries the
// "observation_key" string, the value is appended to ObservationKeys.
//
// pumpStdout returns when r returns EOF or any scanner-level error. The
// scanner error (if any) is returned wrapped; a bare EOF returns nil.
func pumpStdout(r io.Reader, stepIdx int, rl *rawLog, signalsJSONL *jsonlWriter, observational bool) (pumpOutput, error) {
	out := pumpOutput{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxStdoutLineBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		// Copy because scanner reuses the underlying buffer.
		lineCopy := append([]byte(nil), line...)
		rl.WriteAnnotated(stepIdx, "stdout", lineCopy)

		trimmed := bytes.TrimSpace(lineCopy)
		if len(trimmed) == 0 {
			continue
		}
		sig, err := signal.DecodeLine(trimmed)
		if err != nil {
			rl.WriteAnnotated(stepIdx, "parse-error", []byte(err.Error()))
			continue
		}
		out.Signals = append(out.Signals, sig)
		_ = signalsJSONL.WriteLine(trimmed)
		if observational {
			if k, ok := sig.Evidence["observation_key"].(string); ok && k != "" {
				out.ObservationKeys = append(out.ObservationKeys, k)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("signals: scan stdout: %w", err)
	}
	return out, nil
}

// pumpStderr reads r line-by-line and writes each line to rl with stream
// "stderr". Returns when r returns EOF.
func pumpStderr(r io.Reader, stepIdx int, rl *rawLog) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxStdoutLineBytes)
	for scanner.Scan() {
		rl.WriteAnnotated(stepIdx, "stderr", append([]byte(nil), scanner.Bytes()...))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("signals: scan stderr: %w", err)
	}
	return nil
}

// jsonlWriter appends raw JSON lines (no annotation) to signals.jsonl.
// Not goroutine-safe; the executor uses it only from the stdout pump.
type jsonlWriter struct {
	f *os.File
	w *bufio.Writer
}

func newJSONLWriter(path string) (*jsonlWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("jsonl: open %s: %w", path, err)
	}
	return &jsonlWriter{f: f, w: bufio.NewWriter(f)}, nil
}

func (j *jsonlWriter) WriteLine(b []byte) error {
	if _, err := j.w.Write(b); err != nil {
		return err
	}
	if _, err := j.w.Write([]byte{'\n'}); err != nil {
		return err
	}
	// Flush after every line so that observational sensors (which remain
	// running while the caller polls the file) write signals incrementally
	// rather than buffering until Close.
	return j.w.Flush()
}

func (j *jsonlWriter) Close() error {
	if j == nil || j.f == nil {
		return nil
	}
	err := j.w.Flush()
	if cerr := j.f.Close(); err == nil {
		err = cerr
	}
	j.f = nil
	return err
}
