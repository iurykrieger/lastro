package executor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/signal"
)

// signalMatcher is a compiled signal_match: a regex over output lines plus the
// verdict/confidence/heal_hint of the Signal emitted on a match.
type signalMatcher struct {
	Key        string
	Re         *regexp.Regexp
	Verdict    enums.Verdict
	Confidence float64
	HealHint   *signal.HealHint
}

// signalConfig carries what the pumps need to synthesize signals from matched
// lines. Non-nil only for sensors that declare signal_matches (or are passed
// expected keys for JSON-signal observation collection).
type signalConfig struct {
	SchemaVersion string
	SensorID      string
	UseCaseID     string
	Angle         enums.ValidationAngle
	Now           func() time.Time
	Matchers      []signalMatcher
}

func (o *signalConfig) synthesize(m signalMatcher, sub []string) signal.Signal {
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	ev := signal.Evidence{"observation_key": m.Key, "matched_line": sub[0]}
	for i, name := range m.Re.SubexpNames() {
		if i == 0 || name == "" {
			continue
		}
		ev[name] = sub[i]
	}
	return signal.Signal{
		SchemaVersion: o.SchemaVersion,
		SensorID:      o.SensorID,
		UseCaseID:     o.UseCaseID,
		Angle:         o.Angle,
		EmittedAt:     now(),
		Verdict:       m.Verdict,
		Confidence:    m.Confidence,
		Evidence:      ev,
		HealHint:      m.HealHint,
	}
}

// matchLine tests line against every matcher; each hit synthesizes a signal,
// tees it to signalsJSONL, and records it in out. Shared by stdout/stderr pumps.
func (o *signalConfig) matchLine(line []byte, signalsJSONL *jsonlWriter, out *pumpOutput) {
	for _, m := range o.Matchers {
		sub := m.Re.FindStringSubmatch(string(line))
		if sub == nil {
			continue
		}
		sig := o.synthesize(m, sub)
		if b, err := json.Marshal(sig); err == nil {
			_ = signalsJSONL.WriteLine(b)
			out.Signals = append(out.Signals, sig)
			out.ObservationKeys = append(out.ObservationKeys, m.Key)
		}
	}
}

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
// If obs is non-nil the sensor is observational: a decoded Signal's
// "observation_key" evidence is collected, and any plain stdout line matching
// one of obs.Matchers synthesizes an observation signal (teed to signalsJSONL
// and collected under ObservationKeys).
//
// pumpStdout returns when r returns EOF or any scanner-level error. The
// scanner error (if any) is returned wrapped; a bare EOF returns nil.
func pumpStdout(r io.Reader, stepIdx int, rl *rawLog, signalsJSONL *jsonlWriter, obs *signalConfig) (pumpOutput, error) {
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
		// Only lines that look like a JSON object are candidate signals.
		// Plain human-readable stdout (e.g. "api ready on :3030" from a
		// computational sensor) is ordinary output, not a malformed signal:
		// it is already teed to raw.log above. For observational sensors it is
		// matched against the signal_matches regexes; otherwise it is
		// skipped without emitting a noisy parse-error. A parse-error is
		// reserved for lines that attempt to be a signal ('{' ...) but fail.
		if trimmed[0] != '{' {
			if obs != nil {
				obs.matchLine(trimmed, signalsJSONL, &out)
			}
			continue
		}
		sig, err := signal.DecodeLine(trimmed)
		if err != nil {
			if obs != nil {
				// The sensor uses signal_matches: a '{'-line that is not a harness
				// Signal is ordinary (JSON) app output. Match it against the
				// regexes and do NOT emit a parse-error — otherwise a JSON logger
				// (e.g. zap) would flood raw.log with one parse-error per line.
				obs.matchLine(trimmed, signalsJSONL, &out)
			} else {
				// No signal_matches: the line tried to be a Signal but is
				// malformed; surface it for diagnostics.
				rl.WriteAnnotated(stepIdx, "parse-error", []byte(err.Error()))
			}
			continue
		}
		out.Signals = append(out.Signals, sig)
		_ = signalsJSONL.WriteLine(trimmed)
		if obs != nil {
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
// "stderr". For observational sensors (obs != nil) each line is also matched
// against the expected-observation regexes, since tools like docker compose
// print status ("Container X Healthy") to stderr rather than stdout.
// Returns when r returns EOF.
func pumpStderr(r io.Reader, stepIdx int, rl *rawLog, signalsJSONL *jsonlWriter, obs *signalConfig) (pumpOutput, error) {
	out := pumpOutput{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxStdoutLineBytes)
	for scanner.Scan() {
		lineCopy := append([]byte(nil), scanner.Bytes()...)
		rl.WriteAnnotated(stepIdx, "stderr", lineCopy)
		if obs != nil {
			if trimmed := bytes.TrimSpace(lineCopy); len(trimmed) > 0 {
				obs.matchLine(trimmed, signalsJSONL, &out)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("signals: scan stderr: %w", err)
	}
	return out, nil
}

// jsonlWriter appends raw JSON lines (no annotation) to signals.jsonl.
// Goroutine-safe: the stdout and stderr pumps may both emit observation
// signals concurrently, so WriteLine/Close are serialized by mu.
type jsonlWriter struct {
	mu sync.Mutex
	f  *os.File
	w  *bufio.Writer
}

func newJSONLWriter(path string) (*jsonlWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("jsonl: open %s: %w", path, err)
	}
	return &jsonlWriter{f: f, w: bufio.NewWriter(f)}, nil
}

func (j *jsonlWriter) WriteLine(b []byte) error {
	j.mu.Lock()
	defer j.mu.Unlock()
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
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.f == nil {
		return nil
	}
	err := j.w.Flush()
	if cerr := j.f.Close(); err == nil {
		err = cerr
	}
	j.f = nil
	return err
}
