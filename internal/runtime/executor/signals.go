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
	"github.com/iurykrieger/lastro/internal/sensor"
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

// compileMatchers compiles signal_matches into matchers, applying defaults
// (verdict=pass, confidence=1) and synthesizing a heal_hint for fail/warn
// matchers that omit one (the Signal schema requires it). The second return
// is the keys of matchers flagged expected:true, in declaration order.
func compileMatchers(sms []sensor.SignalMatch) ([]signalMatcher, []string, error) {
	matchers := make([]signalMatcher, 0, len(sms))
	var expectedKeys []string
	for _, sm := range sms {
		re, cerr := regexp.Compile(sm.Pattern)
		if cerr != nil {
			return nil, nil, fmt.Errorf("executor: signal_match %q: bad pattern %q: %w", sm.Key, sm.Pattern, cerr)
		}
		verdict := sm.Verdict
		if verdict == "" {
			verdict = enums.VerdictPass
		}
		confidence := 1.0
		if sm.Confidence != nil {
			confidence = *sm.Confidence
		}
		var hh *signal.HealHint
		if sm.HealHint != nil {
			hh = &signal.HealHint{Summary: sm.HealHint.Summary, Rationale: sm.HealHint.Rationale}
		}
		if (verdict == enums.VerdictFail || verdict == enums.VerdictWarn) && hh == nil {
			hh = &signal.HealHint{
				Summary:   fmt.Sprintf("matched %s pattern %q", verdict, sm.Key),
				Rationale: fmt.Sprintf("a stdout/stderr line matched signal_match %q", sm.Key),
			}
		}
		matchers = append(matchers, signalMatcher{Key: sm.Key, Re: re, Verdict: verdict, Confidence: confidence, HealHint: hh})
		if sm.Expected {
			expectedKeys = append(expectedKeys, sm.Key)
		}
	}
	return matchers, expectedKeys, nil
}

// probeValidate checks each matcher's signal envelope (schema fields +
// heal_hint requirement) by synthesizing a probe signal through obs.
// Evidence values are dynamic strings (additionalProperties), so only the
// envelope is checked here.
func probeValidate(obs *signalConfig, matchers []signalMatcher) error {
	for _, m := range matchers {
		probeSub := make([]string, m.Re.NumSubexp()+1)
		probeSub[0] = "<probe>"
		probe := obs.synthesize(m, probeSub)
		b, mErr := json.Marshal(probe)
		if mErr != nil {
			return fmt.Errorf("executor: marshal probe signal for %q: %w", m.Key, mErr)
		}
		if _, vErr := signal.DecodeLine(b); vErr != nil {
			return fmt.Errorf("executor: signal_match %q produces an invalid signal: %w", m.Key, vErr)
		}
	}
	return nil
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
	ev := signal.Evidence{}
	// Named capture groups become evidence fields. Written first so the
	// reserved keys below always win even if a group is named observation_key
	// or matched_line.
	for i, name := range m.Re.SubexpNames() {
		if i == 0 || name == "" || i >= len(sub) {
			continue
		}
		ev[name] = sub[i]
	}
	ev["observation_key"] = m.Key
	ev["matched_line"] = sub[0]
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
	s := string(line)
	for _, m := range o.Matchers {
		sub := m.Re.FindStringSubmatch(s)
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
func pumpStdout(r io.Reader, stepIdx int, rl *rawLog, signalsJSONL *jsonlWriter, obs *signalConfig, red *redactor) (pumpOutput, error) {
	out := pumpOutput{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxStdoutLineBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		// Copy because scanner reuses the underlying buffer.
		lineCopy := append([]byte(nil), line...)
		// Redact before ANYTHING sees the line — raw.log, signal matching,
		// JSON decode — so matched_line evidence and aggregates are clean.
		lineCopy = red.Apply(lineCopy)
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
func pumpStderr(r io.Reader, stepIdx int, rl *rawLog, signalsJSONL *jsonlWriter, obs *signalConfig, red *redactor) (pumpOutput, error) {
	out := pumpOutput{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxStdoutLineBytes)
	for scanner.Scan() {
		lineCopy := append([]byte(nil), scanner.Bytes()...)
		// Redact before ANYTHING sees the line — raw.log, signal matching,
		// JSON decode — so matched_line evidence and aggregates are clean.
		lineCopy = red.Apply(lineCopy)
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
	// red masks registered secret values in persisted signal lines. nil-safe.
	red *redactor
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
	b = j.red.Apply(b)
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
