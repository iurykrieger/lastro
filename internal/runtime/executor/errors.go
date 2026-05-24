// Package executor runs a single Sensor end-to-end. It owns the per-
// step process lifecycle, stdout/stderr capture, signal decoding, and
// the call to aggregate.Rollup that produces the terminal AggregateSignal.
// It knows nothing about sensor IDs, sidecars, or .harness/ layout —
// those are internal/lifecycle's concern.
package executor

import (
	"errors"
	"fmt"
)

// ErrTemplateFixtureInRun is returned when a step's `run:` string
// contains a `{{fixtures.X}}` reference. Fixtures must reach steps
// via env vars from fixturebinder, not via shell-string interpolation,
// to avoid shell-injection from arbitrary fixture content.
var ErrTemplateFixtureInRun = errors.New("executor: {{fixtures.X}} not allowed in step.run; use env vars")

// ErrStepCrashed is returned when a step's process exits non-zero and
// has emitted zero Signals. Multi-step sensors short-circuit on this.
var ErrStepCrashed = errors.New("executor: step exited non-zero without emitting signals")

// TemplateError wraps a template parse or resolve failure with the
// owning step index.
type TemplateError struct {
	Step  int
	Cause error
}

func (e *TemplateError) Error() string {
	return fmt.Sprintf("executor: template error at step %d: %v", e.Step, e.Cause)
}

func (e *TemplateError) Unwrap() error { return e.Cause }

// SpawnError wraps an exec.Cmd.Start failure with the owning step index.
type SpawnError struct {
	Step  int
	Cause error
}

func (e *SpawnError) Error() string {
	return fmt.Sprintf("executor: spawn failed at step %d: %v", e.Step, e.Cause)
}

func (e *SpawnError) Unwrap() error { return e.Cause }
