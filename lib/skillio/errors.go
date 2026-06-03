// Package skillio holds the conventions every harness skill script shares:
// exit codes, structured stderr errors, JSONL stdout helpers, and repo-root
// discovery. Both B4 (detection/generation) and B5 (execution) skills import
// this package; runtime logic lives elsewhere.
package skillio

import "github.com/iurykrieger/lastro/internal/enums"

// Exit-code conventions, uniform across every harness skill script.
const (
	ExitPass         = 0
	ExitFail         = 1
	ExitInconclusive = 2
	ExitScriptError  = 3
)

// ExitCodeForVerdict maps an enums.Verdict to the script's exit code.
// Verdicts outside the pass/inconclusive set map to ExitFail.
func ExitCodeForVerdict(v enums.Verdict) int {
	switch v {
	case enums.VerdictPass:
		return ExitPass
	case enums.VerdictInconclusive:
		return ExitInconclusive
	default:
		return ExitFail
	}
}

// ScriptError is the structured envelope written to stderr on
// script-level failures (bad argv, missing file, unparseable input).
// Runtime failures (failing sensor, malformed YAML inside a fixture)
// surface as terminal AggregateSignals on stdout instead.
type ScriptError struct {
	Code    string         `json:"code"`
	Message string         `json:"error"`
	Details map[string]any `json:"details,omitempty"`
}

// NewScriptError constructs a ScriptError. Details may be nil.
func NewScriptError(code, message string, details map[string]any) *ScriptError {
	return &ScriptError{Code: code, Message: message, Details: details}
}
