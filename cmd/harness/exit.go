package main

import (
	"context"
	"errors"
	"fmt"
)

// Exit codes mirror the spec §3 row 6 and standard sysexits.h values.
const (
	ExitOK            = 0
	ExitFail          = 1   // any obligatory-angle verdict was fail
	ExitInconclusive  = 2   // any verdict was inconclusive and none failed
	ExitInterrupt     = 130 // SIGINT
	ExitSignalTerm    = 143 // SIGTERM
	ExitUsage         = 64  // bad flag combination / missing required input — EX_USAGE
	ExitSoftware      = 70  // internal error / unimplemented — EX_SOFTWARE
)

// UsageError is the sentinel for flag-combination failures. Wraps a
// human-readable message; exitCodeFor maps it to ExitUsage.
type UsageError struct {
	Msg string
}

func (e *UsageError) Error() string { return e.Msg }

// VerdictFailError is returned by validate when at least one use case
// finished with verdict=fail. The CLI's stdout JSON still contains the
// full result; this sentinel signals only the exit code.
var VerdictFailError = errors.New("one or more use cases failed")

// VerdictInconclusiveError is returned by validate when the run is
// inconclusive (no fails but at least one inconclusive).
var VerdictInconclusiveError = errors.New("one or more use cases were inconclusive")

// exitCodeFor maps an error returned from cobra.Command.ExecuteContext
// to a process exit code. ctx is consulted so an interrupted run gets
// the SIGINT/SIGTERM code even when the error chain only reports
// context.Canceled.
func exitCodeFor(err error, ctx context.Context) int {
	if err == nil {
		// Belt-and-braces: if ctx was canceled but no error surfaced,
		// honor the interrupt anyway.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return interruptCode(ctx)
		}
		return ExitOK
	}

	switch {
	case errors.Is(err, VerdictFailError):
		return ExitFail
	case errors.Is(err, VerdictInconclusiveError):
		return ExitInconclusive
	case errors.Is(err, context.Canceled):
		return interruptCode(ctx)
	}

	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		// Surface the usage message to stderr so the user sees it
		// before exit; main() handles printing.
		_ = fmt.Errorf("usage: %w", usageErr)
		return ExitUsage
	}

	// Anything else — including ErrUnimplemented, ErrHealGated, and
	// uncategorized internal failures — is EX_SOFTWARE.
	return ExitSoftware
}

// interruptCode picks 130 (SIGINT) or 143 (SIGTERM) based on the
// context cancellation reason. main() stores the triggering signal
// on the ctx via a typed key so we can disambiguate.
type signalKey struct{}

// SetCancelSignal records which OS signal canceled ctx for downstream
// exitCodeFor consumption.
func SetCancelSignal(ctx context.Context, sig string) context.Context {
	return context.WithValue(ctx, signalKey{}, sig)
}

func interruptCode(ctx context.Context) int {
	if v, ok := ctx.Value(signalKey{}).(string); ok && v == "SIGTERM" {
		return ExitSignalTerm
	}
	return ExitInterrupt
}
