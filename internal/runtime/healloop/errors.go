package healloop

import "errors"

// ErrLLMEmptyPlan is returned via HealResult.Err when the LLMClient
// returns an EditPlan with no files (Status will be StatusAbandoned).
var ErrLLMEmptyPlan = errors.New("healloop: LLM returned empty EditPlan")

// ErrInvalidEditPath is returned via HealResult.Err when an EditFile.Path
// is empty, absolute, or escapes the repo root via "..".
var ErrInvalidEditPath = errors.New("healloop: EditFile.Path escapes repo root")

// ErrUseCaseNotFound is returned by the default Revalidator when the
// requested useCaseID is not in the UseCaseLookup.
var ErrUseCaseNotFound = errors.New("healloop: use case not found in revalidator")
