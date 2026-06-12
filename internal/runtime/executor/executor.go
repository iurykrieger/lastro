package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/runtime/process"
	"github.com/iurykrieger/lastro/internal/runtime/servicemgr"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/signal"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// observationSignalSchemaVersion is the schema_version stamped on synthesized
// observation signals (must match schemas/signal.yaml's accepted shape).
const observationSignalSchemaVersion = "1.0.0"

// mergeKeys returns the union of a and b preserving first-seen order.
func mergeKeys(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(append([]string{}, a...), b...) {
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Options is the dependency wiring an Executor needs. All fields are
// read-only after New; concurrent Run calls are safe.
type Options struct {
	RepoRoot      string
	Resolver      *template.Resolver
	FixtureStore  fixture.FixtureStore
	UseCaseLookup func(sensorID string) (*usecase.UseCase, bool)
	// SensorLookup resolves a core primitive sensor by id. Required only
	// when a sensor contains uses-steps; run-step-only sensors never call
	// it. Wired from the same *sensor.Store the loader builds.
	SensorLookup func(id string) (sensor.Sensor, bool)
	// ServiceAttach resolves a running shared service (a core + observational
	// sensor) to its live Attachment. Returns false when the target is not a
	// managed service, in which case a uses-step expands inline as before.
	// Wired from the use-case runner's *servicemgr.Manager; nil outside the
	// validate flow (uses-steps then always expand).
	ServiceAttach func(ctx context.Context, serviceID string) (servicemgr.Attachment, bool)
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
	// Core-scoped sensors (environment primitives like run-dev) are not bound
	// to a use case, so a missing lookup is only an error for use-case sensors.
	// Steps that reference no fixtures tolerate a nil UseCase (see fixturebinder.Bind).
	uc, ok := e.opts.UseCaseLookup(s.ID)
	if !ok && s.UseCaseID != "" {
		return aggregate.AggregateSignal{}, fmt.Errorf("executor: use case lookup failed for sensor %q", s.ID)
	}

	// Compile signal_matches into matchers. Expected (pass-only) keys feed
	// completeness.
	matchers, expectedKeys, err := compileMatchers(s.SignalMatches)
	if err != nil {
		return aggregate.AggregateSignal{}, err
	}
	expectedObs = mergeKeys(expectedObs, expectedKeys)
	var obs *signalConfig
	if len(expectedObs) > 0 || len(matchers) > 0 {
		obs = &signalConfig{
			SchemaVersion: observationSignalSchemaVersion,
			SensorID:      s.ID,
			UseCaseID:     s.UseCaseID,
			Angle:         s.Angle,
			Now:           e.opts.Now,
			Matchers:      matchers,
		}
		if err := probeValidate(obs, matchers); err != nil {
			return aggregate.AggregateSignal{}, err
		}
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

	// stepOutputs maps a top-level step id to its re-exported outputs.
	// For run-steps the outputs are the parsed HARNESS_OUTPUT map; for
	// uses-steps they are the primitive's declared Outputs resolved
	// against its inner steps' outputs.
	stepOutputs := map[string]map[string]string{}
	// globalIdx is a monotonic counter across every physical step
	// execution (run-steps and the inner steps of expanded uses-steps).
	// It feeds StepIdx (signal attribution) and the unique stepout tag.
	globalIdx := 0

	for _, step := range s.Steps {
		// Build the step-output env from outputs collected by prior
		// top-level steps. Computed fresh per top-level step so the inner
		// steps of a uses-step also see earlier consumer-step outputs.
		stepOutEnv := map[string]string{}
		for sid, kv := range stepOutputs {
			for name, val := range kv {
				stepOutEnv[stepOutEnvName(sid, name)] = val
			}
		}

		res := e.execTopStep(ctx, topStepArgs{
			Sensor:      s,
			Step:        step,
			GlobalIdx:   &globalIdx,
			RunDir:      runDir,
			UseCase:     uc,
			ExpectedObs: expectedObs,
			Obs:         obs,
			RawLog:      rl,
			SignalsW:    sw,
			Stop:        stop,
			StepOutEnv:  stepOutEnv,
		})

		// Store re-exported outputs for use by subsequent steps.
		stepOutputs[step.ID] = res.Outputs
		allSignals = append(allSignals, toAggregateSignals(res.Signals)...)
		observedKeys = append(observedKeys, res.ObservationKeys...)

		if res.TermReason != enums.TerminationCompleted {
			termReason = res.TermReason
			stepErr = res.StepErr
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
