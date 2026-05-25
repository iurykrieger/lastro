package healloop

import (
	"context"
	"time"

	usecase "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
)

// Config gathers the runtime knobs Run needs. MaxIterations should be set
// from policy.EffectivePolicy.MaxHealIterations; zero is valid and short-
// circuits Run to StatusExhausted with no LLM call.
type Config struct {
	// MaxIterations caps how many Propose/Apply/Revalidate cycles Run
	// performs before returning StatusExhausted. 0 disables heal.
	MaxIterations int
	// Now is the clock used for stash-message uniqueness and other
	// timestamps. Optional; defaults to time.Now.
	Now func() time.Time
}

// HealResult is Run's complete return surface. Status is set by the exit
// path; Err carries supplementary information (LLM error, joined revert
// error, etc.). See spec §11 for the full exit matrix.
type HealResult struct {
	Status         Status
	IterationsUsed int
	// Attempts records every iteration, oldest first. The successful
	// attempt (when Status == StatusHealed) is included; its Reverted
	// flag is false. All other entries have Reverted=true.
	Attempts     []Attempt
	FinalVerdict usecase.UseCaseVerdict
	// Err is populated when Status alone is ambiguous: LLM Propose error,
	// joined revert error, or a Commit failure that leaked a stash entry.
	// A bare error return from Run signals a different class of failure
	// (infrastructure error or ctx cancellation) and HealResult is zero.
	Err error
}

// Run drives the heal loop. See spec §6 for the algorithm and §11 for the
// exit matrix.
//
// Run takes ownership of the working tree for the duration of the call:
// edits are applied and either committed (on heal) or reverted (on every
// failing iteration). On bare-error return, the working tree state depends
// on whether a snapshot was taken — see the matrix.
func Run(
	ctx context.Context,
	verdict usecase.UseCaseVerdict,
	llm LLMClient,
	tx Transactor,
	rev Revalidator,
	cfg Config,
) (HealResult, error) {
	// Implementation lands in Phase 3.
	return HealResult{}, nil
}
