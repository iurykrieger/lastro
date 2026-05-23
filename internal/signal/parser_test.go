package signal

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
