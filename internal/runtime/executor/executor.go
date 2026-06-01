package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/runtime/process"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/signal"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// Options is the dependency wiring an Executor needs. All fields are
// read-only after New; concurrent Run calls are safe.
type Options struct {
	RepoRoot      string
	Resolver      *template.Resolver
	FixtureStore  fixture.FixtureStore
	UseCaseLookup func(sensorID string) (*usecase.UseCase, bool)
	Now           func() time.Time
	Shell         []string
	GroupSignaler process.GroupSignaler
	OnStepStart   func(stepIdx, pid, pgid int)
}

// Executor is a stateless function-table over Options. Construct once,
// call Run as many times as needed.
type Executor struct{ opts Options }

// OptionsRef returns the Executor's Options. Used by Lifecycle to build
// per-run Executors that share the same dependencies (Resolver,
// FixtureStore, etc.) but install fresh OnStepStart hooks per call.
func (e *Executor) OptionsRef() Options { return e.opts }

// New creates an Executor wired with the given options. If opts.Now is
// nil it defaults to time.Now. If opts.GroupSignaler is nil it defaults
// to process.Default().
func New(opts Options) *Executor {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.GroupSignaler == nil {
		opts.GroupSignaler = process.Default()
	}
	return &Executor{opts: opts}
}

// Run executes one sensor end-to-end against runDir. runDir must already
// exist; Run creates runDir/scratch on demand. The caller controls Stop
// (typically nil for assertion sensors).
//
// expectedObs is forwarded to aggregate.RollupInput; pass nil for
// assertion sensors.
func (e *Executor) Run(
	ctx context.Context,
	s sensor.Sensor,
	runDir string,
	expectedObs []string,
	stop <-chan struct{},
) (aggregate.AggregateSignal, error) {
	uc, ok := e.opts.UseCaseLookup(s.ID)
	if !ok {
		return aggregate.AggregateSignal{}, fmt.Errorf("executor: use case lookup failed for sensor %q", s.ID)
	}

	if err := os.MkdirAll(filepath.Join(runDir, "scratch"), 0o700); err != nil {
		return aggregate.AggregateSignal{}, fmt.Errorf("executor: mkdir runDir: %w", err)
	}
	rawPath := filepath.Join(runDir, "raw.log")
	signalsPath := filepath.Join(runDir, "signals.jsonl")
	rl, err := newRawLog(rawPath, e.opts.Now)
	if err != nil {
		return aggregate.AggregateSignal{}, err
	}
	defer rl.Close()
	sw, err := newJSONLWriter(signalsPath)
	if err != nil {
		return aggregate.AggregateSignal{}, err
	}
	defer sw.Close()

	startedAt := e.opts.Now()
	allSignals := []aggregate.Signal{}
	observedKeys := []string{}
	termReason := enums.TerminationCompleted
	var stepErr error

	stepOutputs := map[string]map[string]string{}

	for i, step := range s.Steps {
		stepIdx := i + 1

		// Build the step-output env from outputs collected by prior steps.
		stepOutEnv := map[string]string{}
		for sid, kv := range stepOutputs {
			for name, val := range kv {
				stepOutEnv[stepOutEnvName(sid, name)] = val
			}
		}

		outcome, err := runStep(ctx, stepArgs{
			Step:        step,
			StepIdx:     stepIdx,
			RunDir:      runDir,
			UseCase:     uc,
			Store:       e.opts.FixtureStore,
			Resolver:    e.opts.Resolver,
			Signaler:    e.opts.GroupSignaler,
			Shell:       e.opts.Shell,
			ExpectedObs: expectedObs,
			RawLog:      rl,
			SignalsW:    sw,
			Stop:        stop,
			OnStart:     e.opts.OnStepStart,
			StepOutEnv:  stepOutEnv,
		})

		// Store outputs for use by subsequent steps.
		stepOutputs[step.ID] = outcome.Outputs

		allSignals = append(allSignals, toAggregateSignals(outcome.Signals)...)
		observedKeys = append(observedKeys, outcome.ObservationKeys...)

		// Context cancellation / timeout / external stop takes priority over
		// any scan or exit error: when the OS kills the child, the pipe closes
		// with an error that looks like a structural failure but is actually a
		// clean shutdown.
		switch {
		case outcome.StoppedExternally:
			termReason = enums.TerminationStopped
		case errors.Is(outcome.CtxErr, context.DeadlineExceeded):
			termReason = enums.TerminationTimeout
			stepErr = outcome.CtxErr
		case errors.Is(outcome.CtxErr, context.Canceled):
			termReason = enums.TerminationStopped
		}
		if termReason != enums.TerminationCompleted {
			break
		}

		// Structural error (template/spawn/binder) → halt with error.
		// Exception: if signals were already collected, a scan error on
		// stdout is a spurious pipe-close race (the process exited and the
		// OS closed the pipe before our scanner finished). Treat it as
		// non-fatal so the collected signals drive the verdict.
		if err != nil && len(outcome.Signals) == 0 {
			termReason = enums.TerminationError
			stepErr = err
			break
		}

		// Non-zero exit without any signals emitted → crash.
		if outcome.ExitErr != nil && len(outcome.Signals) == 0 {
			termReason = enums.TerminationError
			stepErr = fmt.Errorf("%w: %v", ErrStepCrashed, outcome.ExitErr)
			break
		}
	}

	endedAt := e.opts.Now()

	agg, rollupErr := aggregate.Rollup(aggregate.RollupInput{
		Signals:              allSignals,
		SensorID:             s.ID,
		UseCaseID:            s.UseCaseID,
		Angle:                s.Angle,
		Kind:                 s.Kind,
		OutputType:           s.OutputType,
		StartedAt:            startedAt,
		EndedAt:              endedAt,
		TerminationReason:    termReason,
		ExpectedObservations: expectedObs,
		ObservedKeys:         observedKeys,
	})
	if rollupErr != nil {
		return aggregate.AggregateSignal{}, fmt.Errorf("executor: rollup: %w", rollupErr)
	}

	// Crash-hint patch: if error termination produced an aggregate with
	// no heal hint, synthesize one from raw.log.
	if termReason == enums.TerminationError && agg.HealHint == nil && stepErr != nil {
		// Flush the buffered rawLog writer before reading the file so that
		// the stderr lines captured by pumpStderr are visible to synthesizeCrashHint.
		_ = rl.Flush()
		agg.HealHint = synthesizeCrashHint(rawPath, stepErr)
	}

	return agg, nil
}

// toAggregateSignals converts []signal.Signal (the public typed record)
// into []aggregate.Signal (a type alias to signalstub.Signal). A
// field-for-field copy is required because signal.Signal and
// signalstub.Signal are separate named types with identical shapes.
func toAggregateSignals(in []signal.Signal) []aggregate.Signal {
	out := make([]aggregate.Signal, len(in))
	for i, s := range in {
		out[i] = aggregate.Signal{
			SchemaVersion: s.SchemaVersion,
			SensorID:      s.SensorID,
			UseCaseID:     s.UseCaseID,
			Angle:         s.Angle,
			EmittedAt:     s.EmittedAt,
			Verdict:       s.Verdict,
			Confidence:    s.Confidence,
			Evidence:      aggregate.Evidence(s.Evidence),
			HealHint:      aggregate.ConvertHealHint(s.HealHint),
		}
	}
	return out
}
