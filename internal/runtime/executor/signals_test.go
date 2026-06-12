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

	out, err := pumpStdout(stdout, 1, rl, signalsJSONL, &signalConfig{} /*observational, no matchers*/, nil)
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

	out, err := pumpStdout(stdout, 1, rl, jw, nil, nil)
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

// An observational sensor with signal_matches regexes synthesizes an
// observation signal for each matched stdout line, writing it to signals.jsonl
// and collecting the matcher key — even though the line is plain text.
func TestPumpStdout_RegexMatchSynthesizesObservationSignal(t *testing.T) {
	stdout := strings.NewReader("Container dynamodb Started\napi ready on :3030\nnoise line\n")

	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	jw, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer jw.Close()

	obs := &signalConfig{
		SchemaVersion: "1.0.0",
		SensorID:      "run-dev",
		UseCaseID:     "", // core sensor: empty
		Angle:         "environment",
		Now:           fixedNow(t),
		Matchers: []signalMatcher{
			{Key: "api-ready", Re: regexp.MustCompile(`api ready on :`), Verdict: "pass", Confidence: 1},
			{Key: "dynamodb-up", Re: regexp.MustCompile(`Container dynamodb\s+Started`), Verdict: "pass", Confidence: 1},
		},
	}
	out, err := pumpStdout(stdout, 1, rl, jw, obs, nil)
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

func TestPumpStdout_SignalMatchSynthesis(t *testing.T) {
	stdout := strings.NewReader(`{"path":"/v1/charges","status_code":500}` + "\n" + `served /health ok` + "\n")

	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	jw, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer jw.Close()

	cfg := &signalConfig{
		SchemaVersion: "1.0.0", SensorID: "s", UseCaseID: "", Angle: "environment",
		Now: fixedNow(t),
		Matchers: []signalMatcher{
			{Key: "http-5xx", Re: regexp.MustCompile(`"status_code":(?P<status>5\d\d)`),
				Verdict: "fail", Confidence: 1,
				HealHint: &signal.HealHint{Summary: "5xx", Rationale: "server error"}},
			{Key: "served", Re: regexp.MustCompile(`served (?P<path>\S+)`),
				Verdict: "pass", Confidence: 1},
		},
	}
	out, err := pumpStdout(stdout, 1, rl, jw, cfg, nil)
	if err != nil {
		t.Fatalf("pumpStdout: %v", err)
	}
	if len(out.Signals) != 2 {
		t.Fatalf("Signals = %d, want 2", len(out.Signals))
	}
	var failSig signal.Signal
	for _, s := range out.Signals {
		if s.Verdict == "fail" {
			failSig = s
		}
	}
	if failSig.Verdict != "fail" {
		t.Fatal("no fail signal emitted")
	}
	if got, _ := failSig.Evidence["status"].(string); got != "500" {
		t.Errorf("evidence.status = %q, want 500", got)
	}
	if failSig.HealHint == nil {
		t.Error("fail signal missing heal_hint")
	}
	b, _ := json.Marshal(failSig)
	if _, derr := signal.DecodeLine(b); derr != nil {
		t.Errorf("synthesized fail signal invalid: %v", derr)
	}
}

// A JSON app-log line (starts with '{', not a harness Signal) matched by a
// signal_match must emit a signal WITHOUT a parse-error in raw.log.
func TestPumpStdout_JSONAppLogMatchedNoParseError(t *testing.T) {
	stdout := strings.NewReader(`{"level":"info","status_code":500,"path":"/x"}` + "\n")
	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	jw, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer jw.Close()
	obs := &signalConfig{
		SchemaVersion: "1.0.0", SensorID: "s", Angle: "environment", Now: fixedNow(t),
		Matchers: []signalMatcher{
			{Key: "http-5xx", Re: regexp.MustCompile(`"status_code":5\d\d`), Verdict: "fail", Confidence: 1,
				HealHint: &signal.HealHint{Summary: "5xx", Rationale: "server error"}},
		},
	}
	out, err := pumpStdout(stdout, 1, rl, jw, obs, nil)
	if err != nil {
		t.Fatalf("pumpStdout: %v", err)
	}
	if len(out.Signals) != 1 {
		t.Fatalf("Signals = %d, want 1", len(out.Signals))
	}
	_ = rl.Flush()
	raw, _ := os.ReadFile(dir + "/raw.log")
	if strings.Contains(string(raw), "parse-error") {
		t.Errorf("JSON app log matched by signal_matches must not produce a parse-error; raw.log:\n%s", raw)
	}
}

func TestSynthesize_ShortSubmatchDoesNotPanic(t *testing.T) {
	cfg := &signalConfig{SchemaVersion: "1.0.0", SensorID: "s", Angle: "environment", Now: fixedNow(t)}
	m := signalMatcher{Key: "k", Re: regexp.MustCompile(`(?P<status>\d+)`), Verdict: "pass", Confidence: 1}
	// Probe-style call: only sub[0] populated. Must not panic.
	sig := cfg.synthesize(m, []string{"<probe>"})
	if sig.Evidence["observation_key"] != "k" {
		t.Errorf("observation_key = %v, want k", sig.Evidence["observation_key"])
	}
}

func TestSynthesize_CaptureCannotShadowReservedKeys(t *testing.T) {
	cfg := &signalConfig{SchemaVersion: "1.0.0", SensorID: "s", Angle: "environment", Now: fixedNow(t)}
	m := signalMatcher{Key: "k", Re: regexp.MustCompile(`(?P<observation_key>\w+) (?P<matched_line>\w+)`), Verdict: "pass", Confidence: 1}
	sub := m.Re.FindStringSubmatch("aaa bbb")
	sig := cfg.synthesize(m, sub)
	if sig.Evidence["observation_key"] != "k" {
		t.Errorf("capture shadowed reserved observation_key: %v", sig.Evidence["observation_key"])
	}
	if sig.Evidence["matched_line"] != "aaa bbb" {
		t.Errorf("matched_line = %v, want full line", sig.Evidence["matched_line"])
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

	out, err := pumpStdout(stdout, 2, rl, jw, nil, nil)
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
