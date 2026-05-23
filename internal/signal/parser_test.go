package signal

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
)

// openTestdata opens a file under internal/signal/testdata and registers
// a t.Cleanup to close it.
func openTestdata(t *testing.T, name string) *os.File {
	t.Helper()
	path := filepath.Join("testdata", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// collectSignals exhausts a ParseSignals iteration and returns the
// signals and errors in the order they were yielded.
func collectSignals(seq func(yield func(Signal, error) bool)) ([]Signal, []error) {
	var sigs []Signal
	var errs []error
	for sig, err := range seq {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		sigs = append(sigs, sig)
	}
	return sigs, errs
}

func TestParseSignals_MixedStream(t *testing.T) {
	f := openTestdata(t, "mixed.jsonl")
	sigs, errs := collectSignals(ParseSignals(f))

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(sigs) != 3 {
		t.Fatalf("expected 3 signals, got %d", len(sigs))
	}

	// Signal 0: pass / build
	if got, want := sigs[0].Verdict, enums.VerdictPass; got != want {
		t.Errorf("sigs[0].Verdict = %q, want %q", got, want)
	}
	if got, want := sigs[0].Angle, enums.AngleBuild; got != want {
		t.Errorf("sigs[0].Angle = %q, want %q", got, want)
	}
	if sigs[0].HealHint != nil {
		t.Errorf("sigs[0].HealHint = %+v, want nil", sigs[0].HealHint)
	}

	// Signal 1: fail / unit-test + heal_hint
	if got, want := sigs[1].Verdict, enums.VerdictFail; got != want {
		t.Errorf("sigs[1].Verdict = %q, want %q", got, want)
	}
	if got, want := sigs[1].Angle, enums.AngleUnitTest; got != want {
		t.Errorf("sigs[1].Angle = %q, want %q", got, want)
	}
	if sigs[1].HealHint == nil {
		t.Fatal("sigs[1].HealHint = nil, want non-nil")
	}
	if got, want := sigs[1].HealHint.Summary, "createOrder throws on valid input; check the validation branch"; got != want {
		t.Errorf("sigs[1].HealHint.Summary = %q, want %q", got, want)
	}
	if len(sigs[1].HealHint.SuggestedLocus) != 1 {
		t.Fatalf("sigs[1].HealHint.SuggestedLocus len = %d, want 1", len(sigs[1].HealHint.SuggestedLocus))
	}
	if got, want := sigs[1].HealHint.SuggestedLocus[0].Path, "src/handlers/orders.ts"; got != want {
		t.Errorf("sigs[1].HealHint.SuggestedLocus[0].Path = %q, want %q", got, want)
	}
	if got, want := sigs[1].HealHint.SuggestedLocus[0].Symbol, "createOrder"; got != want {
		t.Errorf("sigs[1].HealHint.SuggestedLocus[0].Symbol = %q, want %q", got, want)
	}
	if fid, ok := sigs[1].Evidence.FixtureID(); !ok || fid != "order-input-fixture" {
		t.Errorf("sigs[1].Evidence.FixtureID = (%q, %v), want (\"order-input-fixture\", true)", fid, ok)
	}

	// Signal 2: inconclusive / code-structure / confidence 0.55
	if got, want := sigs[2].Verdict, enums.VerdictInconclusive; got != want {
		t.Errorf("sigs[2].Verdict = %q, want %q", got, want)
	}
	if got, want := sigs[2].Angle, enums.AngleCodeStructure; got != want {
		t.Errorf("sigs[2].Angle = %q, want %q", got, want)
	}
	if got, want := sigs[2].Confidence, 0.55; got != want {
		t.Errorf("sigs[2].Confidence = %v, want %v", got, want)
	}
}

func TestParseSignals_MalformedMidStream(t *testing.T) {
	f := openTestdata(t, "malformed-mid.jsonl")

	type yielded struct {
		sig Signal
		err error
	}
	var ys []yielded
	for sig, err := range ParseSignals(f) {
		ys = append(ys, yielded{sig, err})
	}

	if len(ys) != 3 {
		t.Fatalf("expected 3 yields, got %d: %+v", len(ys), ys)
	}
	if ys[0].err != nil {
		t.Errorf("ys[0] should be a clean parse, got err: %v", ys[0].err)
	}
	if ys[1].err == nil {
		t.Fatal("ys[1] should be an error yield, got err=nil")
	}
	if !strings.Contains(ys[1].err.Error(), "decode line") {
		t.Errorf("ys[1].err should mention 'decode line', got: %v", ys[1].err)
	}
	if !reflect.DeepEqual(ys[1].sig, Signal{}) {
		t.Errorf("ys[1].sig should be the zero Signal, got %+v", ys[1].sig)
	}
	if ys[2].err != nil {
		t.Errorf("ys[2] should be a clean parse, got err: %v", ys[2].err)
	}
}

func TestParseSignals_SchemaInvalidMidStream(t *testing.T) {
	// Three lines:
	//   1. valid signal
	//   2. valid JSON, but verdict=fail with no heal_hint (schema rule violation)
	//   3. valid signal
	stream := `{"schema_version":"1.0.0","sensor_id":"build-create-order-sensor","use_case_id":"create-order-use-case","angle":"build","emitted_at":"2026-05-22T10:15:00Z","verdict":"pass","confidence":1.0,"evidence":{"expected":"tsc exits 0","actual":"tsc exited 0"}}
{"schema_version":"1.0.0","sensor_id":"bad-sensor","use_case_id":"create-order-use-case","angle":"unit-test","emitted_at":"2026-05-22T10:16:00Z","verdict":"fail","confidence":1.0,"evidence":{"expected":"x","actual":"y"}}
{"schema_version":"1.0.0","sensor_id":"code-structure-create-order-sensor","use_case_id":"create-order-use-case","angle":"code-structure","emitted_at":"2026-05-22T10:17:15Z","verdict":"inconclusive","confidence":0.55,"evidence":{"expected":"Handler delegates to a service layer","actual":"Handler is mixed with business logic; LLM judgment uncertain"}}
`

	type yielded struct {
		sig Signal
		err error
	}
	var ys []yielded
	for sig, err := range ParseSignals(strings.NewReader(stream)) {
		ys = append(ys, yielded{sig, err})
	}

	if len(ys) != 3 {
		t.Fatalf("expected 3 yields, got %d", len(ys))
	}
	if ys[0].err != nil {
		t.Errorf("ys[0] should be clean, got: %v", ys[0].err)
	}
	if ys[1].err == nil {
		t.Fatal("ys[1] should be schema error, got nil")
	}
	if !strings.Contains(ys[1].err.Error(), "schema") {
		t.Errorf("ys[1].err should mention 'schema', got: %v", ys[1].err)
	}
	if !strings.Contains(ys[1].err.Error(), "heal_hint") {
		t.Errorf("ys[1].err should mention 'heal_hint' (the missing field), got: %v", ys[1].err)
	}
	if !reflect.DeepEqual(ys[1].sig, Signal{}) {
		t.Errorf("ys[1].sig should be the zero Signal on error, got %+v", ys[1].sig)
	}
	if ys[2].err != nil {
		t.Errorf("ys[2] should be clean, got: %v", ys[2].err)
	}
}

func TestParseSignals_TypedDecodeInvalidMidStream(t *testing.T) {
	// Three lines:
	//   1. valid signal
	//   2. valid JSON, passes schema (string emitted_at), but bad timestamp shape
	//      (jsonschema/v6 treats format:date-time as advisory, but
	//       time.Time.UnmarshalJSON rejects "not-a-timestamp")
	//   3. valid signal
	stream := `{"schema_version":"1.0.0","sensor_id":"build-create-order-sensor","use_case_id":"create-order-use-case","angle":"build","emitted_at":"2026-05-22T10:15:00Z","verdict":"pass","confidence":1.0,"evidence":{"expected":"tsc exits 0","actual":"tsc exited 0"}}
{"schema_version":"1.0.0","sensor_id":"build-create-order-sensor","use_case_id":"create-order-use-case","angle":"build","emitted_at":"not-a-timestamp","verdict":"pass","confidence":1.0,"evidence":{"expected":"x","actual":"y"}}
{"schema_version":"1.0.0","sensor_id":"code-structure-create-order-sensor","use_case_id":"create-order-use-case","angle":"code-structure","emitted_at":"2026-05-22T10:17:15Z","verdict":"inconclusive","confidence":0.55,"evidence":{"expected":"Handler delegates to a service layer","actual":"Handler is mixed with business logic; LLM judgment uncertain"}}
`

	type yielded struct {
		sig Signal
		err error
	}
	var ys []yielded
	for sig, err := range ParseSignals(strings.NewReader(stream)) {
		ys = append(ys, yielded{sig, err})
	}

	if len(ys) != 3 {
		t.Fatalf("expected 3 yields, got %d", len(ys))
	}
	if ys[0].err != nil {
		t.Errorf("ys[0] should be clean, got: %v", ys[0].err)
	}
	if ys[1].err == nil {
		t.Fatal("ys[1] should be typed-decode error, got nil")
	}
	if !strings.Contains(ys[1].err.Error(), "decode typed") {
		t.Errorf("ys[1].err should mention 'decode typed', got: %v", ys[1].err)
	}
	if !reflect.DeepEqual(ys[1].sig, Signal{}) {
		t.Errorf("ys[1].sig should be the zero Signal on error, got %+v", ys[1].sig)
	}
	if ys[2].err != nil {
		t.Errorf("ys[2] should be clean, got: %v", ys[2].err)
	}
}

func TestParseSignals_EmptyStream(t *testing.T) {
	f := openTestdata(t, "empty.jsonl")
	sigs, errs := collectSignals(ParseSignals(f))
	if len(sigs) != 0 {
		t.Errorf("expected 0 signals from empty stream, got %d", len(sigs))
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 errors from empty stream, got %d: %v", len(errs), errs)
	}
}

func TestParseSignals_BlankLines(t *testing.T) {
	f := openTestdata(t, "blank-lines.jsonl")
	sigs, errs := collectSignals(ParseSignals(f))
	if len(errs) != 0 {
		t.Fatalf("blank lines should be skipped silently, got errors: %v", errs)
	}
	if len(sigs) != 2 {
		t.Errorf("expected 2 signals (blank lines skipped), got %d", len(sigs))
	}
}

func TestParseSignals_BigEvidence(t *testing.T) {
	// Build one signal with an ~900 KiB evidence.actual string, inline.
	// This stays under the 1 MiB max-token-size; the parser should
	// consume it cleanly in one yield.
	const big = 900 * 1024
	pad := bytes.Repeat([]byte{'x'}, big)
	line := []byte(`{"schema_version":"1.0.0","sensor_id":"big-evidence-sensor","use_case_id":"create-order-use-case","angle":"logs","emitted_at":"2026-05-22T10:18:00Z","verdict":"pass","confidence":1.0,"evidence":{"expected":"short","actual":"`)
	line = append(line, pad...)
	line = append(line, []byte(`"}}`)...)
	line = append(line, '\n')

	sigs, errs := collectSignals(ParseSignals(bytes.NewReader(line)))
	if len(errs) != 0 {
		t.Fatalf("big evidence under 1 MiB should parse cleanly, got errors: %v", errs)
	}
	if len(sigs) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(sigs))
	}
	actual, ok := sigs[0].Evidence.Actual()
	if !ok {
		t.Fatal("evidence.actual missing")
	}
	s, _ := actual.(string)
	if len(s) != big {
		t.Errorf("evidence.actual length = %d, want %d", len(s), big)
	}
}

func TestParseSignals_LineExceedsCap(t *testing.T) {
	// 2 MiB line — exceeds the 1 MiB cap. bufio.Scanner should emit
	// bufio.ErrTooLong; the parser wraps it as a single reader-level
	// error yield, then terminates.
	const huge = 2 * 1024 * 1024
	pad := bytes.Repeat([]byte{'x'}, huge)
	line := append([]byte(`{"x":"`), pad...)
	line = append(line, []byte(`"}`)...)
	line = append(line, '\n')

	type yielded struct {
		sig Signal
		err error
	}
	var ys []yielded
	for sig, err := range ParseSignals(bytes.NewReader(line)) {
		ys = append(ys, yielded{sig, err})
	}

	if len(ys) != 1 {
		t.Fatalf("expected exactly 1 yield (the scan error), got %d", len(ys))
	}
	if ys[0].err == nil {
		t.Fatal("expected scan error, got nil")
	}
	if !strings.Contains(ys[0].err.Error(), "scan") {
		t.Errorf("expected error to mention 'scan', got: %v", ys[0].err)
	}
	if !reflect.DeepEqual(ys[0].sig, Signal{}) {
		t.Errorf("expected zero Signal on scan error, got %+v", ys[0].sig)
	}
}

func TestParseSignals_StreamingBehavior(t *testing.T) {
	// Verify the parser yields per-line as bytes arrive — it does not
	// buffer the whole stream before yielding the first signal.
	// Strategy: io.Pipe with a writer that sleeps between two writes;
	// assert the second yield arrives meaningfully after the first.
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pr.Close() })

	line1 := []byte(`{"schema_version":"1.0.0","sensor_id":"build-create-order-sensor","use_case_id":"create-order-use-case","angle":"build","emitted_at":"2026-05-22T10:15:00Z","verdict":"pass","confidence":1.0,"evidence":{"expected":"a","actual":"a"}}` + "\n")
	line2 := []byte(`{"schema_version":"1.0.0","sensor_id":"build-create-order-sensor","use_case_id":"create-order-use-case","angle":"build","emitted_at":"2026-05-22T10:16:00Z","verdict":"pass","confidence":1.0,"evidence":{"expected":"b","actual":"b"}}` + "\n")

	const gap = 50 * time.Millisecond

	go func() {
		defer pw.Close()
		_, _ = pw.Write(line1)
		time.Sleep(gap)
		_, _ = pw.Write(line2)
	}()

	type stamped struct {
		sig Signal
		err error
		at  time.Time
	}
	var ys []stamped
	for sig, err := range ParseSignals(pr) {
		ys = append(ys, stamped{sig, err, time.Now()})
	}

	if len(ys) != 2 {
		t.Fatalf("expected 2 yields, got %d", len(ys))
	}
	if ys[0].err != nil || ys[1].err != nil {
		t.Fatalf("unexpected errors: %v, %v", ys[0].err, ys[1].err)
	}
	observed := ys[1].at.Sub(ys[0].at)
	// Use a margin smaller than the gap to keep the test robust against
	// scheduler jitter; if the parser buffers the whole stream, this
	// gap collapses to ~0.
	if observed < gap/2 {
		t.Errorf("second yield arrived too quickly (gap=%v, observed=%v); parser may be buffering", gap, observed)
	}
}

func TestParseSignals_EarlyCallerStop(t *testing.T) {
	// Iterate mixed.jsonl, break after the first signal. Assert exactly
	// one signal is observed — iteration honored the early stop.
	f := openTestdata(t, "mixed.jsonl")

	var got []Signal
	for sig, err := range ParseSignals(f) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, sig)
		break
	}

	if len(got) != 1 {
		t.Fatalf("expected exactly 1 signal (caller stopped), got %d", len(got))
	}
}

func TestParseSignals_IOError(t *testing.T) {
	// io.Pipe + CloseWithError surfaces as a reader-level error.
	// The parser yields one (Signal{}, error) and terminates.
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pr.Close() })

	line := []byte(`{"schema_version":"1.0.0","sensor_id":"build-create-order-sensor","use_case_id":"create-order-use-case","angle":"build","emitted_at":"2026-05-22T10:15:00Z","verdict":"pass","confidence":1.0,"evidence":{"expected":"a","actual":"a"}}` + "\n")

	go func() {
		_, _ = pw.Write(line)
		_ = pw.CloseWithError(io.ErrUnexpectedEOF)
	}()

	type yielded struct {
		sig Signal
		err error
	}
	var ys []yielded
	for sig, err := range ParseSignals(pr) {
		ys = append(ys, yielded{sig, err})
	}

	if len(ys) != 2 {
		t.Fatalf("expected 2 yields (one signal, one error), got %d", len(ys))
	}
	if ys[0].err != nil {
		t.Errorf("first yield should be a clean signal, got err: %v", ys[0].err)
	}
	if ys[1].err == nil {
		t.Fatal("second yield should be the I/O error, got nil")
	}
	if !strings.Contains(ys[1].err.Error(), "scan") {
		t.Errorf("error should mention 'scan' wrapping, got: %v", ys[1].err)
	}
	if !reflect.DeepEqual(ys[1].sig, Signal{}) {
		t.Errorf("expected zero Signal on I/O error, got %+v", ys[1].sig)
	}
}
