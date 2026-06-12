package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/runtime/fixturebinder"
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
	Obs         *signalConfig
	RawLog      *rawLog
	SignalsW    *jsonlWriter
	Stop        <-chan struct{}
	StepOutEnv  map[string]string // outputs of prior top-level steps
	Redactor    *redactor
	EnvView     envView // merged host+env_file ambient view, loaded once per run
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
		if e.opts.SensorLookup != nil {
			// A shared service is, by invariant, a core + observational sensor
			// (see servicemgr). Only those targets attach to a live stream;
			// every other uses-step inline-expands as before.
			if prim, ok := e.opts.SensorLookup(a.Step.Uses); ok && prim.Scope == enums.ScopeCore && prim.Kind == enums.KindObservational {
				return e.attachToService(ctx, a, prim)
			}
		}
		return e.execUsesStep(ctx, a)
	}
	return e.execRunStep(ctx, a)
}

// attachToService runs the attach-step path for a uses-step whose target is a
// shared observational service. Falls back to inline expansion if no service
// is registered (ServiceAttach nil or returns false) so non-validate callers
// keep working.
//
// Note on env: the consumer uses-step's env: map is NOT applied on the attach
// path — the shared service process was spawned elsewhere with its own
// environment (design: shared services read their own .env). The env: map IS
// applied when the step falls back to inline expansion via execUsesStep.
func (e *Executor) attachToService(ctx context.Context, a topStepArgs, prim sensor.Sensor) topStepResult {
	if e.opts.ServiceAttach == nil {
		return e.execUsesStep(ctx, a)
	}
	att, ok := e.opts.ServiceAttach(ctx, prim.ID)
	if !ok {
		return e.execUsesStep(ctx, a)
	}
	*a.GlobalIdx++
	r := execAttachStep(ctx, attachArgs{
		Consumer:      a.Sensor,
		Attachment:    att,
		ExpectedKeys:  expectedKeysOf(a.Sensor),
		ObserveWindow: observeWindowOf(a.Sensor),
		Now:           e.opts.Now,
		SignalsW:      a.SignalsW,
		Stop:          a.Stop,
	})
	return topStepResult{
		Signals:         r.Signals,
		ObservationKeys: r.ObservationKeys,
		Outputs:         map[string]string{},
		TermReason:      r.TermReason,
		StepErr:         r.StepErr,
	}
}

// expectedKeysOf returns the consumer's expected observation keys (pass
// matchers flagged expected:true).
func expectedKeysOf(s sensor.Sensor) []string {
	var keys []string
	for _, sm := range s.SignalMatches {
		if sm.Expected {
			keys = append(keys, sm.Key)
		}
	}
	return keys
}

// observeWindowOf returns the sensor's ObserveWindow (parsed) or 0 for default.
func observeWindowOf(s sensor.Sensor) time.Duration {
	if s.ObserveWindow == "" {
		return 0
	}
	d, err := time.ParseDuration(s.ObserveWindow)
	if err != nil {
		return 0
	}
	return d
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
		Obs:         a.Obs,
		RawLog:      a.RawLog,
		SignalsW:    a.SignalsW,
		Stop:        a.Stop,
		OnStart:     e.opts.OnStepStart,
		StepOutEnv:  a.StepOutEnv,
		Redactor:    a.Redactor,
		EnvView:     a.EnvView,
	})
	var me *MissingEnvError
	if errors.As(err, &me) {
		sig := missingEnvSignal(a.Sensor, me.Names, me.EnvFile, e.opts.Now)
		writeSignal(a.SignalsW, sig)
		return topStepResult{
			Signals:    []signal.Signal{sig},
			Outputs:    map[string]string{},
			TermReason: enums.TerminationError,
			StepErr:    me,
		}
	}
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

	// Collect and bind any ${{ fixtures.<id> }} refs in the with values AND
	// env values before building the input env and resolving consumer env.
	fixturePaths := map[string]string{}
	ids, err := fixturebinder.CollectFixtureRefs("", a.Step.With)
	if err != nil {
		return topStepResult{TermReason: enums.TerminationError, StepErr: fmt.Errorf("executor: collect fixture refs in with: %w", err)}
	}
	envIDs, err := fixturebinder.CollectFixtureRefs("", a.Step.Env)
	if err != nil {
		return topStepResult{TermReason: enums.TerminationError, StepErr: fmt.Errorf("executor: collect fixture refs in env: %w", err)}
	}
	ids = mergeKeys(ids, envIDs)
	if len(ids) > 0 {
		binder := &fixturebinder.Binder{ScratchDir: filepath.Join(a.RunDir, "scratch")}
		if err := os.MkdirAll(binder.ScratchDir, 0o700); err != nil {
			return topStepResult{TermReason: enums.TerminationError, StepErr: fmt.Errorf("executor: mkdir scratch: %w", err)}
		}
		binding, err := binder.Bind(ids, a.UseCase, e.opts.FixtureStore)
		if err != nil {
			return topStepResult{TermReason: enums.TerminationError, StepErr: err}
		}
		fixturePaths = binding.Files
	}

	inputEnv, err := buildInputEnv(prim, a.Step.With, fixturePaths, a.StepOutEnv)
	if err != nil {
		return topStepResult{TermReason: enums.TerminationError, StepErr: err}
	}

	// Consumer-declared env: resolved once, injected into every inner step
	// of the expansion (issue #49) — this is how a use-case sensor hands
	// NEXTAUTH_SECRET to provision-auth's recipe. inputs.* is not valid at
	// the consumer level (mirrors with-value semantics), hence nil.
	consumerEnv, refDerived, consumerMissing, err := resolveStepEnv(a.Step.Env, a.EnvView, nil, a.StepOutEnv, fixturePaths)
	if err != nil {
		return topStepResult{TermReason: enums.TerminationError, StepErr: fmt.Errorf("executor: step %q: %w", a.Step.ID, err)}
	}
	// Primitive-declared ambient requirements: satisfied by the merged
	// host+env_file view or by the consumer's own env: injection.
	for name, spec := range prim.Env {
		if !spec.IsRequired() {
			continue
		}
		if v, ok := consumerEnv[name]; ok && v != "" {
			continue
		}
		if v, ok := a.EnvView.lookup(name); ok && v != "" {
			continue
		}
		consumerMissing = append(consumerMissing, name)
	}
	if len(consumerMissing) > 0 {
		consumerMissing = mergeKeys(consumerMissing, nil) // dedupe
		sort.Strings(consumerMissing)
		me := &MissingEnvError{Names: consumerMissing, EnvFile: a.EnvView.source}
		sig := missingEnvSignal(a.Sensor, consumerMissing, a.EnvView.source, e.opts.Now)
		writeSignal(a.SignalsW, sig)
		return topStepResult{
			Signals:    []signal.Signal{sig},
			Outputs:    map[string]string{},
			TermReason: enums.TerminationError,
			StepErr:    me,
		}
	}
	for name, val := range consumerEnv {
		if refDerived[name] {
			a.Redactor.Add(val)
		}
	}

	// Make the primitive's own signal_matches live for its inner steps
	// (issue #46): merge them under the consumer's matchers so the
	// grade-and-emit contract holds when the primitive runs composed, not
	// only when it self-runs. Signals stay attributed to the consumer.
	obs, err := e.composeObs(a, prim)
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
			Obs:         obs,
			RawLog:      a.RawLog,
			SignalsW:    a.SignalsW,
			Stop:        a.Stop,
			OnStart:     e.opts.OnStepStart,
			InputEnv:    inputEnv,
			StepOutEnv:  stepOutEnv,
			ExtraEnv:    consumerEnv,
			Redactor:    a.Redactor,
			EnvView:     a.EnvView,
		})

		var me *MissingEnvError
		if errors.As(runErr, &me) {
			sig := missingEnvSignal(a.Sensor, me.Names, me.EnvFile, e.opts.Now)
			writeSignal(a.SignalsW, sig)
			res.Signals = append(res.Signals, sig)
			res.TermReason = enums.TerminationError
			res.StepErr = me
			return res
		}

		innerOutputs[inner.ID] = outcome.Outputs
		res.Signals = append(res.Signals, outcome.Signals...)
		res.ObservationKeys = append(res.ObservationKeys, outcome.ObservationKeys...)

		term, stepErr := evalTermination(ctx, outcome, runErr)
		if term != enums.TerminationCompleted {
			res.TermReason = term
			res.StepErr = stepErr
			return res
		}
		// Grading gate: an inner step that exits non-zero after reporting a
		// fail or inconclusive signal has terminally graded its work. End
		// the expansion AND the run with TerminationError so the aggregate
		// rolls up via Rule 0 (explicit fail wins, else inconclusive) — a
		// downstream gated step must never run and outrank the grade
		// (issue #45 / #46). Pass/warn signals and zero exits keep the
		// run-step parity: exit non-zero with signals = completed.
		if outcome.ExitErr != nil {
			if v, gated := gradedVerdict(outcome.Signals); gated {
				res.TermReason = enums.TerminationError
				res.StepErr = fmt.Errorf("executor: composed primitive %q step %q graded %s and exited non-zero", prim.ID, inner.ID, v)
				return res
			}
		}
	}

	// Re-export the primitive's declared outputs under the uses-step id.
	res.Outputs = resolveOutputs(prim.Outputs, innerOutputs)
	return res
}

// composeObs returns the signal config the expansion's inner steps run
// under. When the primitive declares no signal_matches the consumer's
// config passes through unchanged. Otherwise the primitive's matchers are
// compiled and merged after the consumer's, deduplicated by key with the
// consumer winning on collision, and the merged config keeps the
// consumer's identity (sensor_id / use_case_id / angle) so synthesized
// signals are attributed to the running sensor. The primitive's
// expected:true flags do not feed the run-level completeness: expected
// observations remain the consumer's declaration.
func (e *Executor) composeObs(a topStepArgs, prim sensor.Sensor) (*signalConfig, error) {
	if len(prim.SignalMatches) == 0 {
		return a.Obs, nil
	}
	primMatchers, _, err := compileMatchers(prim.SignalMatches)
	if err != nil {
		return nil, fmt.Errorf("executor: composed primitive %q: %w", prim.ID, err)
	}
	seen := map[string]struct{}{}
	var merged []signalMatcher
	if a.Obs != nil {
		merged = append(merged, a.Obs.Matchers...)
		for _, m := range a.Obs.Matchers {
			seen[m.Key] = struct{}{}
		}
	}
	for _, m := range primMatchers {
		if _, dup := seen[m.Key]; dup {
			continue // consumer wins on key collision
		}
		merged = append(merged, m)
	}
	obs := &signalConfig{
		SchemaVersion: observationSignalSchemaVersion,
		SensorID:      a.Sensor.ID,
		UseCaseID:     a.Sensor.UseCaseID,
		Angle:         a.Sensor.Angle,
		Now:           e.opts.Now,
		Matchers:      merged,
	}
	if err := probeValidate(obs, primMatchers); err != nil {
		return nil, fmt.Errorf("executor: composed primitive %q: %w", prim.ID, err)
	}
	return obs, nil
}

// gradedVerdict reports whether signals carry a terminally grading verdict
// (fail outranks inconclusive). Pass/warn signals are not grading: they
// never end an expansion.
func gradedVerdict(signals []signal.Signal) (enums.Verdict, bool) {
	var verdict enums.Verdict
	for _, s := range signals {
		switch s.Verdict {
		case enums.VerdictFail:
			return enums.VerdictFail, true
		case enums.VerdictInconclusive:
			verdict = enums.VerdictInconclusive
		}
	}
	return verdict, verdict != ""
}

// buildInputEnv computes the HARNESS_INPUT_<NAME> env for a primitive's
// declared inputs. Precedence per input: with-value > default (when
// declared) > error (when required) > skip. A with-value is fully
// interpolated: literals pass through verbatim, ${{ fixtures.<id> }} refs
// resolve to the bound file path (from fixturePaths), and
// ${{ steps.<id>.outputs.<name> }} refs resolve from stepOutEnv. Mixed
// literal+ref values are supported.
func buildInputEnv(prim sensor.Sensor, with, fixturePaths, stepOutEnv map[string]string) (map[string]string, error) {
	env := map[string]string{}
	for name, spec := range prim.Inputs {
		raw, bound := with[name]
		switch {
		case bound:
			val, err := resolveWithValue(raw, fixturePaths, stepOutEnv)
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

// resolveWithValue interprets a with-value into a concrete string. Each
// segment resolves: literal -> text; ${{ fixtures.<id> }} -> the bound
// fixture's file path; ${{ steps.<id>.outputs.<name> }} -> the accumulated
// step-output value. Input/entry-point refs are not valid inside a
// with-value.
func resolveWithValue(raw string, fixturePaths, stepOutEnv map[string]string) (string, error) {
	segs, err := template.Parse(raw)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, s := range segs {
		switch v := s.(type) {
		case template.Literal:
			b.WriteString(v.Text)
		case template.FixtureRef:
			if len(v.JSONPath) > 0 {
				return "", fmt.Errorf("fixture jsonpath drilling not supported in with-values")
			}
			p, ok := fixturePaths[v.ID]
			if !ok {
				return "", fmt.Errorf("fixture %q referenced in with-value was not bound", v.ID)
			}
			b.WriteString(p)
		case template.StepOutputRef:
			b.WriteString(stepOutEnv[stepOutEnvName(v.StepID, v.Name)])
		case template.InputRef:
			return "", fmt.Errorf("inputs.* is not valid inside a with-value")
		case template.EntryPointRef:
			return "", fmt.Errorf("entry_points.* is not valid inside a with-value")
		case template.EnvRef:
			return "", fmt.Errorf("env.* is not valid inside a with-value; declare it under the step's env: map instead")
		default:
			return "", fmt.Errorf("unsupported segment %T in with-value", s)
		}
	}
	return b.String(), nil
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
