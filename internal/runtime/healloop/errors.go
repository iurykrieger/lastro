package healloop

import "errors"

var (
	// ErrLLMEmptyPlan is returned via HealResult.Err when the LLMClient
	// returns an EditPlan with no files (Status will be StatusAbandoned).
	ErrLLMEmptyPlan = errors.New("healloop: LLM returned empty EditPlan")

	// ErrInvalidEditPath is returned via HealResult.Err when an EditFile.Path
	// is empty, absolute, or escapes the repo root via "..".
	ErrInvalidEditPath = errors.New("healloop: EditFile.Path is empty, absolute, or escapes repo root")

	// ErrUseCaseNotFound is returned by the default Revalidator when the
	// requested useCaseID is not in the UseCaseLookup.
	ErrUseCaseNotFound = errors.New("healloop: use case not found in revalidator")
)
