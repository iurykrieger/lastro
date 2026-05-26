package healloop

import (
	usecase "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
)

// BuildPromptInput assembles a PromptInput from a failing UseCaseVerdict,
// the prior attempts (oldest first), and a UseCaseLookup. Returns
// ErrUseCaseNotFound when the lookup does not contain verdict.UseCaseID.
//
// Callers may build PromptInput directly; this helper exists so the
// common case (look up the use case by id) is a one-liner.
func BuildPromptInput(verdict usecase.UseCaseVerdict, history []Attempt, ucs UseCaseLookup) (PromptInput, error) {
	uc, ok := ucs.Lookup(verdict.UseCaseID)
	if !ok {
		return PromptInput{}, ErrUseCaseNotFound
	}
	return PromptInput{
		UseCase: uc,
		Verdict: verdict,
		Hints:   verdict.HealHints,
		History: history,
	}, nil
}
