package executor

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/signal"
)

func TestPumpStdout_HappyPath(t *testing.T) {
	const passLine = `{"schema_version":"1.0.0","sensor_id":"s","use_case_id":"u","angle":"build","emitted_at":"2026-05-24T10:00:00Z","verdict":"pass","confidence":1,"evidence":{}}`
	const obsLine = `{"schema_version":"1.0.0","sensor_id":"s","use_case_id":"u","angle":"logs","emitted_at":"2026-05-24T10:00:00Z","verdict":"pass","confidence":1,"evidence":{"observation_key":"order-received"}}`

	stdout := strings.NewReader(passLine + "\n\n" + obsLine + "\n")
	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	signalsJSONL, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer signalsJSONL.Close()

	out, err := pumpStdout(stdout, 1, rl, signalsJSONL, &observationConfig{} /*observational, no matchers*/)
	if err != nil {
		t.Fatalf("pumpStdout: %v", err)
	}
	if got, want := len(out.Signals), 2; got != want {
		t.Errorf("len(Signals) = %d, want %d", got, want)
	}
	if got, want := out.ObservationKeys, []string{"order-received"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("ObservationKeys = %v, want %v", got, want)
	}
}

func TestPumpStdout_BadJSONLineKeepsStreaming(t *testing.T) {
	const passLine = `{"schema_version":"1.0.0","sensor_id":"s","use_case_id":"u","angle":"build","emitted_at":"2026-05-24T10:00:00Z","verdict":"pass","confidence":1,"evidence":{}}`
	// A line that LOOKS like a signal ('{' ...) but is malformed must still be
	// flagged as a parse-error, and streaming must continue.
	stdout := strings.NewReader(`{"oops": not valid` + "\n" + passLine + "\n")

	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	jw, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer jw.Close()

	out, err := pumpStdout(stdout, 1, rl, jw, nil)
	if err != nil {
		t.Fatalf("pumpStdout: %v", err)
	}
	if got, want := len(out.Signals), 1; got != want {
		t.Errorf("len(Signals) = %d, want %d", got, want)
	}
	_ = rl.Flush()
	raw, _ := os.ReadFile(dir + "/raw.log")
	if !strings.Contains(string(raw), "parse-error") {
		t.Errorf("expected a parse-error for the malformed JSON object; raw.log:\n%s", raw)
	}
}

// An observational sensor with expected_observations regexes synthesizes an
// observation signal for each matched stdout line, writing it to signals.jsonl
// and collecting the matcher key — even though the line is plain text.
func TestPumpStdout_RegexMatchSynthesizesObservationSignal(t *testing.T) {
	stdout := strings.NewReader("Container dynamodb Started\napi ready on :3030\nnoise line\n")

	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	jw, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer jw.Close()

	obs := &observationConfig{
		SchemaVersion: "1.0.0",
		SensorID:      "run-dev",
		UseCaseID:     "", // core sensor: empty
		Angle:         "environment",
		Now:           fixedNow(t),
		Matchers: []observationMatcher{
			{Key: "api-ready", Re: regexp.MustCompile(`api ready on :`)},
			{Key: "dynamodb-up", Re: regexp.MustCompile(`Container dynamodb\s+Started`)},
		},
	}
	out, err := pumpStdout(stdout, 1, rl, jw, obs)
	if err != nil {
		t.Fatalf("pumpStdout: %v", err)
	}
	if got, want := len(out.Signals), 2; got != want {
		t.Fatalf("len(Signals) = %d, want %d", got, want)
	}
	if got := out.ObservationKeys; len(got) != 2 {
		t.Fatalf("ObservationKeys = %v, want 2", got)
	}
	// The synthesized signals must be written to signals.jsonl and be valid.
	_ = jw.Close()
	data, _ := os.ReadFile(dir + "/signals.jsonl")
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("signals.jsonl lines = %d, want 2; content:\n%s", len(lines), data)
	}
	for _, ln := range lines {
		if _, derr := signal.DecodeLine([]byte(ln)); derr != nil {
			t.Errorf("synthesized signal failed schema validation: %v\nline: %s", derr, ln)
		}
		var probe map[string]any
		_ = json.Unmarshal([]byte(ln), &probe)
		ev, _ := probe["evidence"].(map[string]any)
		if ev["observation_key"] == nil {
			t.Errorf("synthesized signal missing evidence.observation_key: %s", ln)
		}
	}
}

// Plain human-readable stdout (no leading '{') must NOT be treated as a
// malformed signal: no parse-error, no decoded signal — just teed to raw.log.
func TestPumpStdout_PlainTextIsNotAParseError(t *testing.T) {
	stdout := strings.NewReader("api ready on :3030\nContainer dynamodb Started\n")

	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	jw, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer jw.Close()

	out, err := pumpStdout(stdout, 2, rl, jw, nil)
	if err != nil {
		t.Fatalf("pumpStdout: %v", err)
	}
	if len(out.Signals) != 0 {
		t.Errorf("len(Signals) = %d, want 0", len(out.Signals))
	}
	_ = rl.Flush()
	raw, _ := os.ReadFile(dir + "/raw.log")
	if strings.Contains(string(raw), "parse-error") {
		t.Errorf("plain text should not produce a parse-error; raw.log:\n%s", raw)
	}
	if !strings.Contains(string(raw), "api ready on :3030") {
		t.Errorf("plain text should still be teed to raw.log as stdout; raw.log:\n%s", raw)
	}
}
