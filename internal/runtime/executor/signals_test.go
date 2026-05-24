package executor

import (
	"strings"
	"testing"
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

	out, err := pumpStdout(stdout, 1, rl, signalsJSONL, true /*observational*/)
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
	stdout := strings.NewReader("not json\n" + passLine + "\n")

	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	jw, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer jw.Close()

	out, err := pumpStdout(stdout, 1, rl, jw, false)
	if err != nil {
		t.Fatalf("pumpStdout: %v", err)
	}
	if got, want := len(out.Signals), 1; got != want {
		t.Errorf("len(Signals) = %d, want %d", got, want)
	}
}
