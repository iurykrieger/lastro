package signal

import (
	"os"
	"path/filepath"
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
