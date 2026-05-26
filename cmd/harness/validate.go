package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/lifecycle"
	aggregator "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/internal/runtime/executor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// ErrUnimplemented stays defined for stubs in other subcommands; kept
// here so callers still importing it continue to compile.
var ErrUnimplemented = errors.New("harness: subcommand not yet implemented")

// newValidateCmd returns the validate subcommand wired with real
// lifecycle + aggregator integration.
func newValidateCmd(ctx context.Context, cfg *Config) *cobra.Command {
	var (
		useCaseIDs []string
		all        bool
	)
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate one or more use cases",
		Long: `validate runs the sensors associated with one or more use cases
and prints an aggregated verdict per use case.

Exactly one of --use-case (repeatable) or --all must be supplied.

Exit codes:
  0 - all selected use cases passed
  1 - at least one use case failed
  2 - at least one use case was inconclusive and none failed
  64 - usage error (bad flags or missing input files)
  70 - internal error`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(useCaseIDs) > 0) {
				return &UsageError{Msg: "supply exactly one of --use-case or --all"}
			}
			return runValidate(ctx, cfg, useCaseIDs, all, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringSliceVar(&useCaseIDs, "use-case", nil, "use case id (repeatable; conflicts with --all)")
	cmd.Flags().BoolVar(&all, "all", false, "validate every use case (conflicts with --use-case)")
	return cmd
}

func runValidate(ctx context.Context, cfg *Config, useCaseIDs []string, all bool, out io.Writer) error {
	return runValidateWith(ctx, cfg, useCaseIDs, all, out, defaultRunnerFactory)
}

// runnerFactory builds a SensorRunner over loaded artifacts. The seam
// lets tests inject a fake runner without touching the real lifecycle.
type runnerFactory func(arts *HarnessArtifacts, repoRoot string) (SensorRunner, func(), error)

// defaultRunnerFactory builds the real lifecycle + executor.
func defaultRunnerFactory(arts *HarnessArtifacts, repoRoot string) (SensorRunner, func(), error) {
	// Build the (sensor.ID -> *usecase.UseCase) lookup closure the
	// executor uses to resolve fixture/entry-point references at
	// step-execution time.
	sensorIndex := make(map[string]string, len(arts.Sensors.All()))
	for _, s := range arts.Sensors.All() {
		sensorIndex[s.ID] = s.UseCaseID
	}
	useCaseLookup := func(sensorID string) (*usecase.UseCase, bool) {
		ucID, ok := sensorIndex[sensorID]
		if !ok {
			return nil, false
		}
		uc, ok := arts.UseCases[ucID]
		return uc, ok
	}

	// Flatten every use case's entry points into the template Resolver
	// map so step.Run interpolations like `{{entry_points.X.spec.path}}`
	// resolve at exec time.
	entryPoints := map[string]entrypoint.EntryPoint{}
	for _, uc := range arts.UseCases {
		for _, ep := range uc.EntryPoints {
			entryPoints[ep.ID] = ep
		}
	}

	resolver := &template.Resolver{
		Fixtures:    arts.Fixtures,
		EntryPoints: entryPoints,
	}

	exec := executor.New(executor.Options{
		RepoRoot:      repoRoot,
		Resolver:      resolver,
		FixtureStore:  arts.Fixtures,
		UseCaseLookup: useCaseLookup,
		Now:           time.Now,
	})

	lc := lifecycle.New(lifecycle.Options{
		Sensors:     lifecycle.WrapSensorStore(arts.Sensors),
		Executor:    exec,
		RuntimeRoot: arts.RuntimeRoot,
		Version:     HarnessVersion,
	})

	cleanup := func() {}
	return lc, cleanup, nil
}

// runValidateWith is the testable seam. Production calls it with
// defaultRunnerFactory; tests pass a closure returning a fake runner.
func runValidateWith(
	ctx context.Context,
	cfg *Config,
	useCaseIDs []string,
	all bool,
	out io.Writer,
	makeRunner runnerFactory,
) error {
	startedAt := time.Now().UTC()
	runID := newRunID(startedAt)

	repoRoot, err := resolveRepoRoot(cfg)
	if err != nil {
		return &UsageError{Msg: err.Error()}
	}
	harnessDir, err := resolveHarnessDir(repoRoot)
	if err != nil {
		return &UsageError{Msg: err.Error()}
	}
	policyPath := resolvePolicyPath(cfg, harnessDir)

	arts, err := LoadHarnessArtifacts(harnessDir, policyPath)
	if err != nil {
		return &UsageError{Msg: err.Error()}
	}

	// Resolve which use case IDs to run.
	ids := useCaseIDs
	if all {
		ids = make([]string, 0, len(arts.UseCases))
		for id := range arts.UseCases {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}
	// Verify every requested ID exists before doing any work.
	for _, id := range ids {
		if _, ok := arts.UseCases[id]; !ok {
			return &UsageError{Msg: fmt.Sprintf("unknown use case %q", id)}
		}
	}

	runner, cleanup, err := makeRunner(arts, repoRoot)
	if err != nil {
		return fmt.Errorf("init runner: %w", err)
	}
	defer cleanup()

	// Bounded parallel evaluation over use cases.
	sem := make(chan struct{}, cfg.EffectiveConcurrency())
	results := make([]UseCaseRunResult, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		i, id := i, id
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r, err := RunUseCase(ctx, runner, arts, id)
			if err != nil {
				// Surface as an inconclusive synthesized verdict.
				r = synthesizeFailedRun(id, err)
			}
			results[i] = r
		}()
	}
	wg.Wait()

	endedAt := time.Now().UTC()

	// Render the wrapper.
	wrapper := &RunResult{
		CLISchemaVersion: CLISchemaVersion,
		RunID:            runID,
		Command:          "validate",
		Args:             os.Args[1:],
		StartedAt:        startedAt,
		EndedAt:          endedAt,
		DurationMs:       endedAt.Sub(startedAt).Milliseconds(),
		HarnessVersion:   HarnessVersion,
		Result:           buildValidateResult(results),
	}
	if err := Render(out, wrapper, cfg.Output, renderValidateText); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	return aggregateExitDecision(results)
}

func newRunID(t time.Time) string {
	return ulid.MustNew(ulid.Timestamp(t), nil).String()
}

// aggregateExitDecision returns VerdictFailError if any use case
// failed, VerdictInconclusiveError if any was inconclusive (and none
// failed), or nil for all-pass.
func aggregateExitDecision(results []UseCaseRunResult) error {
	hasFail := false
	hasInconclusive := false
	for _, r := range results {
		switch r.Verdict.Verdict {
		case enums.VerdictFail:
			hasFail = true
		case enums.VerdictInconclusive:
			hasInconclusive = true
		}
	}
	switch {
	case hasFail:
		return VerdictFailError
	case hasInconclusive:
		return VerdictInconclusiveError
	}
	return nil
}

// synthesizeFailedRun materializes a UseCaseRunResult when the runner
// itself errored before producing any sensor signals. The verdict is
// explicitly inconclusive so aggregateExitDecision routes the CLI to
// exit code 2 rather than treating the zero value as a pass.
func synthesizeFailedRun(id string, err error) UseCaseRunResult {
	return UseCaseRunResult{
		Verdict: aggregator.UseCaseVerdict{
			UseCaseID:  id,
			Verdict:    enums.VerdictInconclusive,
			Confidence: 0,
		},
	}
}

// buildValidateResult shapes the spec §6.2 "result" object from raw
// per-use-case run results.
func buildValidateResult(results []UseCaseRunResult) any {
	type sensorJSON struct {
		SensorID    string                `json:"sensor_id"`
		Angle       enums.ValidationAngle `json:"angle"`
		Verdict     enums.Verdict         `json:"verdict"`
		Confidence  float64               `json:"confidence"`
		StartedAt   time.Time             `json:"started_at"`
		EndedAt     time.Time             `json:"ended_at"`
		Rollup      any                   `json:"rollup"`
		RuntimePath string                `json:"runtime_path,omitempty"`
		HealHint    any                   `json:"heal_hint,omitempty"`
	}
	type verdictJSON struct {
		UseCaseID           string                  `json:"use_case_id"`
		Archetype           enums.Archetype         `json:"archetype"`
		Verdict             enums.Verdict           `json:"verdict"`
		Confidence          float64                 `json:"confidence"`
		ObligatorySatisfied bool                    `json:"obligatory_satisfied"`
		EvaluatedAngles     []enums.ValidationAngle `json:"evaluated_angles"`
		FailingAngles       []enums.ValidationAngle `json:"failing_angles"`
		WarningAngles       []enums.ValidationAngle `json:"warning_angles"`
		HealHints           any                     `json:"heal_hints"`
		Sensors             []sensorJSON            `json:"sensors"`
	}
	type summary struct {
		TotalUseCases     int `json:"total_use_cases"`
		PassCount         int `json:"pass_count"`
		FailCount         int `json:"fail_count"`
		InconclusiveCount int `json:"inconclusive_count"`
	}

	verdicts := make([]verdictJSON, 0, len(results))
	s := summary{TotalUseCases: len(results)}
	for _, r := range results {
		v := r.Verdict
		sensors := make([]sensorJSON, 0, len(r.Sensors))
		for _, sig := range r.Sensors {
			sensors = append(sensors, sensorJSON{
				SensorID:   sig.SensorID,
				Angle:      sig.Angle,
				Verdict:    sig.Verdict,
				Confidence: sig.Confidence,
				StartedAt:  sig.StartedAt,
				EndedAt:    sig.EndedAt,
				Rollup:     sig.Rollup,
				HealHint:   sig.HealHint,
			})
		}
		verdicts = append(verdicts, verdictJSON{
			UseCaseID:           v.UseCaseID,
			Archetype:           v.Archetype,
			Verdict:             v.Verdict,
			Confidence:          v.Confidence,
			ObligatorySatisfied: v.ObligatorySatisfied,
			EvaluatedAngles:     v.EvaluatedAngles,
			FailingAngles:       v.FailingAngles,
			WarningAngles:       v.WarningAngles,
			HealHints:           v.HealHints,
			Sensors:             sensors,
		})
		switch v.Verdict {
		case enums.VerdictPass:
			s.PassCount++
		case enums.VerdictFail:
			s.FailCount++
		case enums.VerdictInconclusive:
			s.InconclusiveCount++
		}
	}
	return map[string]any{
		"verdicts": verdicts,
		"summary":  s,
	}
}

// renderValidateText is the text-mode renderer. Task 19 replaces this
// placeholder with the full glyph-aware implementation.
func renderValidateText(w io.Writer, r *RunResult) error {
	_, err := fmt.Fprintln(w, "validate complete (text renderer pending Task 19)")
	return err
}
