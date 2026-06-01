package executor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/signal"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// topStepArgs is the per-top-level-step input handed to execTopStep. A
// top-level step is either a run-step (executed once) or a uses-step
// (expanded into the referenced primitive's inner steps).
type topStepArgs struct {
	Sensor      sensor.Sensor
	Step        sensor.Step
	GlobalIdx   *int // monotonic physical-step counter, mutated by execTopStep
	RunDir      string
	UseCase     *usecase.UseCase
	ExpectedObs []string
	RawLog      *rawLog
	SignalsW    *jsonlWriter
	Stop        <-chan struct{}
	StepOutEnv  map[string]string // outputs of prior top-level steps
}

// topStepResult is the rolled-up outcome of one top-level step. Outputs
// is what the step re-exports for later consumer steps (the parsed
// HARNESS_OUTPUT for a run-step; the primitive's declared Outputs for a
// uses-step). TermReason is TerminationCompleted on clean success.
type topStepResult struct {
	Signals         []signal.Signal
	ObservationKeys []string
	Outputs         map[string]string
	TermReason      enums.TerminationReason
	StepErr         error
}

// execTopStep dispatches a top-level step to either the run-step path or
// the composition (uses-step) path.
func (e *Executor) execTopStep(ctx context.Context, a topStepArgs) topStepResult {
	if a.Step.Uses != "" {
		return e.execUsesStep(ctx, a)
	}
	return e.execRunStep(ctx, a)
}

// execRunStep runs a single shell run-step. It preserves the exact
// termination semantics the executor relied on before composition.
func (e *Executor) execRunStep(ctx context.Context, a topStepArgs) topStepResult {
	*a.GlobalIdx++
	idx := *a.GlobalIdx
	outcome, err := runStep(ctx, stepArgs{
		Step:        a.Step,
		StepIdx:     idx,
		OutTag:      strconv.Itoa(idx) + "-" + a.Step.ID,
		RunDir:      a.RunDir,
		UseCase:     a.UseCase,
		Store:       e.opts.FixtureStore,
		Resolver:    e.opts.Resolver,
		Signaler:    e.opts.GroupSignaler,
		Shell:       e.opts.Shell,
		ExpectedObs: a.ExpectedObs,
		RawLog:      a.RawLog,
		SignalsW:    a.SignalsW,
		Stop:        a.Stop,
		OnStart:     e.opts.OnStepStart,
		StepOutEnv:  a.StepOutEnv,
	})
	term, stepErr := evalTermination(ctx, outcome, err)
	return topStepResult{
		Signals:         outcome.Signals,
		ObservationKeys: outcome.ObservationKeys,
		Outputs:         outcome.Outputs,
		TermReason:      term,
		StepErr:         stepErr,
	}
}

// execUsesStep expands a uses-step into the referenced core primitive's
// inner steps, running each with the bound input env and the accumulated
// step-output env (prior consumer steps plus prior inner steps). After
// the inner steps complete, it resolves the primitive's declared outputs
// and returns them keyed under the consumer's uses-step id.
func (e *Executor) execUsesStep(ctx context.Context, a topStepArgs) topStepResult {
	if e.opts.SensorLookup == nil {
		return topStepResult{
			TermReason: enums.TerminationError,
			StepErr:    errors.New("executor: composition requires SensorLookup"),
		}
	}
	prim, ok := e.opts.SensorLookup(a.Step.Uses)
	if !ok {
		return topStepResult{
			TermReason: enums.TerminationError,
			StepErr:    fmt.Errorf("executor: composed primitive %q not found", a.Step.Uses),
		}
	}
	if prim.Scope != enums.ScopeCore {
		return topStepResult{
			TermReason: enums.TerminationError,
			StepErr:    fmt.Errorf("executor: composed sensor %q is not a core primitive (scope=%q)", prim.ID, prim.Scope),
		}
	}

	inputEnv, err := buildInputEnv(prim, a.Step.With, a.StepOutEnv)
	if err != nil {
		return topStepResult{TermReason: enums.TerminationError, StepErr: err}
	}

	// innerOutputs collects each inner step's parsed HARNESS_OUTPUT so the
	// primitive's declared Outputs can reference them, and so later inner
	// steps can read earlier inner steps' outputs.
	innerOutputs := map[string]map[string]string{}
	var res topStepResult
	res.TermReason = enums.TerminationCompleted

	for _, inner := range prim.Steps {
		// Inner-step env: prior consumer-step outputs + prior inner-step
		// outputs. Recompute per inner step so chained inner outputs flow.
		stepOutEnv := map[string]string{}
		for k, v := range a.StepOutEnv {
			stepOutEnv[k] = v
		}
		for sid, kv := range innerOutputs {
			for name, val := range kv {
				stepOutEnv[stepOutEnvName(sid, name)] = val
			}
		}

		*a.GlobalIdx++
		idx := *a.GlobalIdx
		outcome, runErr := runStep(ctx, stepArgs{
			Step:        inner,
			StepIdx:     idx,
			OutTag:      strconv.Itoa(idx) + "-" + a.Step.ID + "-" + inner.ID,
			RunDir:      a.RunDir,
			UseCase:     a.UseCase,
			Store:       e.opts.FixtureStore,
			Resolver:    e.opts.Resolver,
			Signaler:    e.opts.GroupSignaler,
			Shell:       e.opts.Shell,
			ExpectedObs: a.ExpectedObs,
			RawLog:      a.RawLog,
			SignalsW:    a.SignalsW,
			Stop:        a.Stop,
			OnStart:     e.opts.OnStepStart,
			InputEnv:    inputEnv,
			StepOutEnv:  stepOutEnv,
		})

		innerOutputs[inner.ID] = outcome.Outputs
		res.Signals = append(res.Signals, outcome.Signals...)
		res.ObservationKeys = append(res.ObservationKeys, outcome.ObservationKeys...)

		term, stepErr := evalTermination(ctx, outcome, runErr)
		if term != enums.TerminationCompleted {
			res.TermReason = term
			res.StepErr = stepErr
			return res
		}
	}

	// Re-export the primitive's declared outputs under the uses-step id.
	res.Outputs = resolveOutputs(prim.Outputs, innerOutputs)
	return res
}

// buildInputEnv computes the HARNESS_INPUT_<NAME> env for a primitive's
// declared inputs. Precedence per input: with-value > default (when
// declared) > error (when required) > skip. A with-value containing a
// single ${{ steps.<id>.outputs.<name> }} ref is substituted from the
// accumulated step-output env; plain literals pass through verbatim.
//
// Limitation (v1): ${{ fixtures.<id> }} inside a with-value is not
// resolved to a bound path here. Bind such fixtures inside the
// primitive's own run-steps instead. Step-output substitution only
// supports a with-value that is exactly one ref (no mixed literal+ref).
func buildInputEnv(prim sensor.Sensor, with, stepOutEnv map[string]string) (map[string]string, error) {
	env := map[string]string{}
	for name, spec := range prim.Inputs {
		raw, bound := with[name]
		switch {
		case bound:
			val, err := resolveWithValue(raw, stepOutEnv)
			if err != nil {
				return nil, fmt.Errorf("input %q: %w", name, err)
			}
			env[inputEnvName(name)] = val
		case spec.HasDefault:
			env[inputEnvName(name)] = spec.Default
		case spec.Required:
			return nil, fmt.Errorf("executor: required input %q unbound", name)
		default:
			// optional, no default → leave unset
		}
	}
	return env, nil
}

// resolveWithValue interprets a with-value. If it is a single
// step-output ref, the value is read from stepOutEnv. Otherwise the raw
// value is returned (literals and fixture refs pass through).
func resolveWithValue(raw string, stepOutEnv map[string]string) (string, error) {
	segs, err := template.Parse(raw)
	if err != nil {
		return "", err
	}
	// Single pure ref: substitute step outputs; everything else passes
	// through verbatim (compile is only used to detect the shape).
	if len(segs) == 1 {
		if so, ok := segs[0].(template.StepOutputRef); ok {
			key := stepOutEnvName(so.StepID, so.Name)
			return stepOutEnv[key], nil
		}
	}
	return raw, nil
}

// resolveOutputs maps each declared output name to its resolved value by
// interpreting OutputSpec.From. Supported From shape: a single
// ${{ steps.<inner>.outputs.<name> }} ref resolved against innerOutputs.
// Unresolvable refs yield an empty value (best effort; never an error).
func resolveOutputs(specs map[string]sensor.OutputSpec, innerOutputs map[string]map[string]string) map[string]string {
	if len(specs) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	for name, spec := range specs {
		out[name] = resolveFrom(spec.From, innerOutputs)
	}
	return out
}

func resolveFrom(from string, innerOutputs map[string]map[string]string) string {
	segs, err := template.Parse(from)
	if err != nil {
		return ""
	}
	if len(segs) == 1 {
		if so, ok := segs[0].(template.StepOutputRef); ok {
			if kv, ok := innerOutputs[so.StepID]; ok {
				return kv[so.Name]
			}
			return ""
		}
		if lit, ok := segs[0].(template.Literal); ok {
			return lit.Text
		}
	}
	return ""
}

func inputEnvName(name string) string {
	return "HARNESS_INPUT_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// evalTermination derives the termination reason for one physical step
// execution. It mirrors the original Run loop's precedence: external
// stop / ctx cancel / timeout first, then structural error, then crash.
// A clean step returns TerminationCompleted with a nil error.
func evalTermination(ctx context.Context, outcome stepOutcome, err error) (enums.TerminationReason, error) {
	switch {
	case outcome.StoppedExternally:
		return enums.TerminationStopped, nil
	case errors.Is(outcome.CtxErr, context.DeadlineExceeded):
		return enums.TerminationTimeout, outcome.CtxErr
	case errors.Is(outcome.CtxErr, context.Canceled):
		return enums.TerminationStopped, nil
	}
	if err != nil && len(outcome.Signals) == 0 {
		return enums.TerminationError, err
	}
	if outcome.ExitErr != nil && len(outcome.Signals) == 0 {
		return enums.TerminationError, fmt.Errorf("%w: %v", ErrStepCrashed, outcome.ExitErr)
	}
	return enums.TerminationCompleted, nil
}
