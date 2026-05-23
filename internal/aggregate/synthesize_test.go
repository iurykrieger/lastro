package aggregate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

func TestRollupSingleShotCarriesOverHealHint(t *testing.T) {
	s := sig(enums.VerdictFail)
	s.HealHint = &HealHint{
		Summary:        "fix broken handler",
		SuggestedLocus: []Locus{{Path: "src/handler.go", Symbol: "Handle"}},
		Rationale:      "handler panics on empty input",
	}
	in := baseInput([]signalstub.Signal{s})
	in.OutputType = enums.OutputSingleShot
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.HealHint == nil {
		t.Fatal("HealHint must not be nil for fail verdict")
	}
	if !reflect.DeepEqual(*got.HealHint, *s.HealHint) {
		t.Errorf("HealHint = %+v, want %+v", *got.HealHint, *s.HealHint)
	}
}

func TestStreamSynthesizesFailSummaryWithCounts(t *testing.T) {
	signals := []signalstub.Signal{
		sig(enums.VerdictPass), sig(enums.VerdictPass), sig(enums.VerdictPass),
		sig(enums.VerdictFail), sig(enums.VerdictFail),
	}
	signals[3].HealHint = &HealHint{
		Summary: "x", Rationale: "y",
		SuggestedLocus: []Locus{{Path: "a.go", Symbol: "f"}},
	}
	signals[4].HealHint = &HealHint{
		Summary: "x", Rationale: "y",
		SuggestedLocus: []Locus{{Path: "b.go", Symbol: "g"}},
	}
	in := baseInput(signals)
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.HealHint == nil {
		t.Fatal("HealHint must not be nil")
	}
	if !strings.Contains(got.HealHint.Summary, "2 of 5") {
		t.Errorf("Summary = %q, want substring '2 of 5'", got.HealHint.Summary)
	}
	if len(got.HealHint.SuggestedLocus) != 2 {
		t.Errorf("SuggestedLocus len = %d, want 2", len(got.HealHint.SuggestedLocus))
	}
}

func TestStreamSynthesizesWarnSummary(t *testing.T) {
	signals := []signalstub.Signal{
		sig(enums.VerdictPass), sig(enums.VerdictPass),
		sig(enums.VerdictWarn), sig(enums.VerdictWarn), sig(enums.VerdictWarn),
	}
	in := baseInput(signals)
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Verdict != enums.VerdictWarn {
		t.Fatalf("Verdict = %q, want warn", got.Verdict)
	}
	if got.HealHint == nil || !strings.Contains(got.HealHint.Summary, "3 warning") {
		t.Errorf("Summary = %q, want substring '3 warning'", got.HealHint.Summary)
	}
}

func TestStreamSynthesisDedupesLociAndCapsAt10(t *testing.T) {
	// Produce 15 fail signals with overlapping loci.
	var signals []signalstub.Signal
	for i := 0; i < 15; i++ {
		s := sig(enums.VerdictFail)
		s.HealHint = &HealHint{
			Summary:        "x",
			Rationale:      "y",
			SuggestedLocus: []Locus{{Path: "f.go", Symbol: "x"}}, // identical → must collapse to 1 entry
		}
		signals = append(signals, s)
	}
	// Add 12 unique fail loci.
	for i := 0; i < 12; i++ {
		s := sig(enums.VerdictFail)
		s.HealHint = &HealHint{
			Summary:   "x",
			Rationale: "y",
			SuggestedLocus: []Locus{
				{Path: "unique.go", Symbol: "sym-" + string(rune('a'+i))},
			},
		}
		signals = append(signals, s)
	}
	in := baseInput(signals)
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.HealHint == nil {
		t.Fatal("HealHint nil")
	}
	if len(got.HealHint.SuggestedLocus) > 10 {
		t.Errorf("SuggestedLocus len = %d, want ≤ 10", len(got.HealHint.SuggestedLocus))
	}
	// Verify dedup: the (f.go, x) locus should appear exactly once.
	count := 0
	for _, l := range got.HealHint.SuggestedLocus {
		if l.Path == "f.go" && l.Symbol == "x" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("(f.go, x) appeared %d times, want 1", count)
	}
}

func TestObservationalSynthesisListsMissingKeys(t *testing.T) {
	in := baseInput(nil)
	in.Kind = enums.KindObservational
	in.ExpectedObservations = []string{"a", "b", "c"}
	in.ObservedKeys = []string{"a"}
	got, err := Rollup(in)
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if got.Verdict != enums.VerdictFail {
		t.Fatalf("Verdict = %q, want fail", got.Verdict)
	}
	if got.HealHint == nil {
		t.Fatal("HealHint nil")
	}
	if !strings.Contains(got.HealHint.Summary, "b") || !strings.Contains(got.HealHint.Summary, "c") {
		t.Errorf("Summary = %q, expected to mention missing keys b and c", got.HealHint.Summary)
	}
}
