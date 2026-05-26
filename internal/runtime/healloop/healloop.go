package healloop

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	usecase "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	upkg "github.com/iurykrieger/lastro/internal/usecase"
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
// useCase is the in-scope use case object; it is forwarded into every
// PromptInput so the LLMClient can inspect given/when/then text and
// fixture IDs without a separate lookup.
//
// Run takes ownership of the working tree for the duration of the call:
// edits are applied and either committed (on heal) or reverted (on every
// failing iteration). On bare-error return, the working tree state depends
// on whether a snapshot was taken — see the matrix.
func Run(
	ctx context.Context,
	useCase *upkg.UseCase,
	verdict usecase.UseCaseVerdict,
	llm LLMClient,
	tx Transactor,
	rev Revalidator,
	cfg Config,
) (HealResult, error) {
	if verdict.Verdict == enums.VerdictPass {
		return HealResult{Status: StatusHealed, FinalVerdict: verdict}, nil
	}

	var attempts []Attempt
	for i := 1; i <= cfg.MaxIterations; i++ {
		if err := ctx.Err(); err != nil {
			return HealResult{}, err
		}
		promptIn := PromptInput{
			UseCase: useCase,
			Verdict: verdict,
			Hints:   verdict.HealHints,
			History: attempts,
		}

		plan, err := llm.Propose(ctx, promptIn)
		if err != nil {
			return HealResult{
				Status:         StatusAbandoned,
				IterationsUsed: i - 1,
				Attempts:       attempts,
				FinalVerdict:   verdict,
				Err:            err,
			}, nil
		}
		if err := ctx.Err(); err != nil {
			return HealResult{}, err
		}

		if len(plan.Files) == 0 {
			return HealResult{
				Status:         StatusAbandoned,
				IterationsUsed: i - 1,
				Attempts:       attempts,
				FinalVerdict:   verdict,
				Err:            ErrLLMEmptyPlan,
			}, nil
		}

		if err := validatePaths(plan); err != nil {
			return HealResult{
				Status:         StatusAbandoned,
				IterationsUsed: i - 1,
				Attempts:       attempts,
				FinalVerdict:   verdict,
				Err:            err,
			}, nil
		}

		paths := collectPaths(plan)
		handle, err := tx.Snapshot(ctx, paths)
		if err != nil {
			return HealResult{}, err
		}

		if err := handle.Apply(plan); err != nil {
			if rErr := handle.Revert(); rErr != nil {
				return HealResult{}, errors.Join(err, rErr)
			}
			return HealResult{}, err
		}

		newVerdict, err := rev.Revalidate(ctx, verdict.UseCaseID)
		if err != nil {
			if rErr := handle.Revert(); rErr != nil {
				return HealResult{}, errors.Join(err, rErr)
			}
			return HealResult{}, err
		}

		if newVerdict.Verdict == enums.VerdictPass {
			commitErr := handle.Commit()
			attempts = append(attempts, Attempt{Iteration: i, Plan: plan, Verdict: newVerdict, Reverted: false})
			return HealResult{
				Status:         StatusHealed,
				IterationsUsed: i,
				Attempts:       attempts,
				FinalVerdict:   newVerdict,
				Err:            commitErr,
			}, nil
		}

		revertErr := handle.Revert()
		attempts = append(attempts, Attempt{Iteration: i, Plan: plan, Verdict: newVerdict, Reverted: true})
		if revertErr != nil {
			// Spec §11 row 8: Revert() failure on a failing iteration exits early
			// with StatusExhausted and the revert error surfaced in HealResult.Err.
			// The working tree is dirty; the caller must surface that to the user.
			return HealResult{
				Status:         StatusExhausted,
				IterationsUsed: i,
				Attempts:       attempts,
				FinalVerdict:   verdict,
				Err:            revertErr,
			}, nil
		}
	}

	return HealResult{
		Status:         StatusExhausted,
		IterationsUsed: cfg.MaxIterations,
		Attempts:       attempts,
		FinalVerdict:   verdict,
	}, nil
}

// collectPaths flattens an EditPlan into the list of file paths it touches.
// Deduplicated, order preserved by first appearance.
func collectPaths(plan EditPlan) []string {
	seen := make(map[string]struct{}, len(plan.Files))
	out := make([]string, 0, len(plan.Files))
	for _, f := range plan.Files {
		if _, ok := seen[f.Path]; ok {
			continue
		}
		seen[f.Path] = struct{}{}
		out = append(out, f.Path)
	}
	return out
}

// validatePaths rejects EditPlan paths that are empty, absolute, or escape
// the repo root via "..". Uses filepath.Clean to normalize before checking.
func validatePaths(plan EditPlan) error {
	for _, f := range plan.Files {
		if f.Path == "" {
			return ErrInvalidEditPath
		}
		if filepath.IsAbs(f.Path) {
			return ErrInvalidEditPath
		}
		clean := filepath.ToSlash(filepath.Clean(f.Path))
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return ErrInvalidEditPath
		}
	}
	return nil
}
