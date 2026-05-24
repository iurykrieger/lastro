// Package fixturebinder writes a sensor step's fixture payloads to disk
// and returns the environment variables a step command needs to read them.
// See docs/harness-framework/plan.md §5 (fixture binding contract).
package fixturebinder

// StepBinding is the resolved per-step view a sensor step's executor consumes.
type StepBinding struct {
	// Env maps HARNESS_FIXTURE_<NORMALIZED_ID> -> absolute file path.
	Env map[string]string
	// Files maps fixture id -> absolute file path. For diagnostics/tests.
	Files map[string]string
	// BoundIDs is the canonical-ordered (ascending) list of bound fixture ids.
	BoundIDs []string
}

// BindError reports a failure during fixture binding.
type BindError struct {
	Code      string // "fixture-not-found" | "fixture-not-owned" | "write-failed"
	FixtureID string
	UseCaseID string
	Cause     error // non-nil only for "write-failed"
}

func (e *BindError) Error() string {
	if e.Cause != nil {
		return "fixturebinder: " + e.Code + ": fixture=" + e.FixtureID + " use_case=" + e.UseCaseID + ": " + e.Cause.Error()
	}
	return "fixturebinder: " + e.Code + ": fixture=" + e.FixtureID + " use_case=" + e.UseCaseID
}

func (e *BindError) Unwrap() error { return e.Cause }
