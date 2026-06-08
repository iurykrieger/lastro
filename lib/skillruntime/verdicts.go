package skillruntime

import (
	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/lib/skillio"
)

// WorstExitCode returns the exit code for ucVerdict but promotes to
// ExitFail if any AggregateSignal has verdict=fail. Prevents a vacuous
// policy (no obligatory angles) from hiding real sensor failures.
func WorstExitCode(ucVerdict enums.Verdict, aggs []aggregate.AggregateSignal) int {
	code := skillio.ExitCodeForVerdict(ucVerdict)
	for _, a := range aggs {
		if a.Verdict == enums.VerdictFail {
			return skillio.ExitFail
		}
		if a.Verdict == enums.VerdictInconclusive && code == skillio.ExitPass {
			code = skillio.ExitInconclusive
		}
	}
	return code
}

// WorstAggregateVerdict returns the worst verdict across all AggregateSignals.
func WorstAggregateVerdict(aggs []aggregate.AggregateSignal) enums.Verdict {
	worst := enums.VerdictPass
	for _, a := range aggs {
		switch a.Verdict {
		case enums.VerdictFail:
			return enums.VerdictFail
		case enums.VerdictInconclusive:
			worst = enums.VerdictInconclusive
		}
	}
	return worst
}
