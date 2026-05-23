package aggregate

import (
	"fmt"
	"strings"

	"github.com/iurykrieger/lastro/internal/aggregate/internal/signalstub"
	"github.com/iurykrieger/lastro/internal/enums"
)

const maxLoci = 10
const maxKeysInSummary = 5

// synthesizeHealHint produces the heal_hint attached to a warn/fail
// aggregate. For pass and inconclusive, returns nil (the caller must not
// attach a heal_hint).
//
// Single-shot: carry over the sole signal's hint verbatim (deep copy).
// Stream / observational: synthesize a meta-hint with templated summary,
// deduplicated loci (capped at maxLoci), and structured rationale.
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
	out := &HealHint{Summary: h.Summary, Rationale: h.Rationale}
	if len(h.SuggestedLocus) > 0 {
		out.SuggestedLocus = append([]Locus(nil), h.SuggestedLocus...)
	}
	return out
}

func synthesizeStreamHealHint(in RollupInput, a AggregateSignal) *HealHint {
	// Observational with missing observations gets a dedicated summary
	// listing the missing keys (capped, with ellipsis when truncated).
	if a.Completeness != nil && len(a.Completeness.MissingObservations) > 0 {
		return observationalMissingHint(a)
	}

	loci := collectLoci(in.Signals, a.Verdict)
	rationale := "see individual non-pass signals for per-record detail"

	var summary string
	switch a.Verdict {
	case enums.VerdictFail:
		summary = fmt.Sprintf("%d of %d %s signals failed",
			a.Rollup.FailCount, a.Rollup.TotalSignals, a.Angle)
	case enums.VerdictWarn:
		noun := "warnings"
		if a.Rollup.WarnCount == 1 {
			noun = "warning"
		}
		summary = fmt.Sprintf("%d %s across %d %s signals",
			a.Rollup.WarnCount, noun, a.Rollup.TotalSignals, a.Angle)
	}

	return &HealHint{
		Summary:        summary,
		SuggestedLocus: loci,
		Rationale:      rationale,
	}
}

func observationalMissingHint(a AggregateSignal) *HealHint {
	missing := a.Completeness.MissingObservations
	keys := missing
	suffix := ""
	if len(missing) > maxKeysInSummary {
		keys = missing[:maxKeysInSummary]
		suffix = ", ..."
	}
	summary := fmt.Sprintf("%s sensor missing %d of %d expected observations: %s%s",
		a.Angle, len(missing), len(a.Completeness.ExpectedObservations),
		strings.Join(keys, ", "), suffix)
	return &HealHint{
		Summary:   summary,
		Rationale: "the sensor did not observe one or more required events; the corresponding code path likely failed silently or is missing instrumentation",
	}
}

// collectLoci returns up to maxLoci deduplicated (path, symbol) entries
// drawn from signals whose verdict matches the aggregate verdict,
// preserving first-seen order.
func collectLoci(signals []signalstub.Signal, verdict enums.Verdict) []Locus {
	seen := make(map[Locus]bool)
	var out []Locus
	for _, s := range signals {
		if s.Verdict != verdict {
			continue
		}
		if s.HealHint == nil {
			continue
		}
		for _, l := range s.HealHint.SuggestedLocus {
			if seen[l] {
				continue
			}
			seen[l] = true
			out = append(out, l)
			if len(out) >= maxLoci {
				return out
			}
		}
	}
	return out
}
