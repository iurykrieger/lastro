package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/lifecycle"
	aggregator "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/internal/runtime/executor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

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
	// map so step.Run interpolations like `${{entry_points.X.spec.path}}`
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
