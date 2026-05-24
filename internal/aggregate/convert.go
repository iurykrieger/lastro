package aggregate

import "github.com/iurykrieger/lastro/internal/signal"

// ConvertHealHint copies a signal.HealHint (defined in the signal
// package) into the structurally-equivalent aggregate.HealHint (alias
// for signalstub.HealHint). The two types have the same shape but live
// in different packages, so a field-by-field copy is required. The
// input may be nil.
func ConvertHealHint(h *signal.HealHint) *HealHint {
	if h == nil {
		return nil
	}
	out := &HealHint{Summary: h.Summary, Rationale: h.Rationale}
	if len(h.SuggestedLocus) > 0 {
		out.SuggestedLocus = make([]Locus, len(h.SuggestedLocus))
		for i, l := range h.SuggestedLocus {
			out.SuggestedLocus[i] = Locus{Path: l.Path, Symbol: l.Symbol}
		}
	}
	return out
}
