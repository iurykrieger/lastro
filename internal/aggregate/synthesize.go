package aggregate

import (
	"github.com/iurykrieger/lastro/internal/enums"
)

// synthesizeHealHint produces the heal_hint attached to a warn/fail
// aggregate. For pass and inconclusive, returns nil (the caller must not
// attach a heal_hint).
//
// Single-shot: carry over the sole signal's hint verbatim (deep copy).
// Stream / observational: synthesize a meta-hint with templated summary,
// deduplicated loci (capped at 10), and structured rationale.
func synthesizeHealHint(in RollupInput, a AggregateSignal) *HealHint {
	if a.Verdict != enums.VerdictWarn && a.Verdict != enums.VerdictFail {
		return nil
	}
	if in.OutputType == enums.OutputSingleShot && len(in.Signals) == 1 && in.Signals[0].HealHint != nil {
		return deepCopyHealHint(in.Signals[0].HealHint)
	}
	return synthesizeStreamHealHint(in, a)
}

func deepCopyHealHint(h *HealHint) *HealHint {
	if h == nil {
		return nil
	}
	out := &HealHint{
		Summary:   h.Summary,
		Rationale: h.Rationale,
	}
	if len(h.SuggestedLocus) > 0 {
		out.SuggestedLocus = append([]Locus(nil), h.SuggestedLocus...)
	}
	return out
}

// synthesizeStreamHealHint will be implemented in Task 21.
func synthesizeStreamHealHint(in RollupInput, a AggregateSignal) *HealHint {
	// placeholder so the package compiles; Task 21 replaces this.
	return &HealHint{Summary: "stream synthesis pending", Rationale: "pending"}
}
