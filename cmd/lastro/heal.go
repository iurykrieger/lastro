package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	aggregator "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/internal/runtime/healloop"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
)

const healDefaultMaxIterations = 3

type healState struct {
	UseCaseID     string        `json:"use_case_id"`
	Iteration     int           `json:"iteration"`
	MaxIterations int           `json:"max_iterations"`
	History       []healAttempt `json:"history"`
}

type healAttempt struct {
	AppliedAt time.Time `json:"applied_at"`
	Rationale string    `json:"rationale"`
	Verdict   string    `json:"verdict"`
}

type healEnvelope struct {
	Status        string                    `json:"status"`
	Iteration     int                       `json:"iteration"`
	MaxIterations int                       `json:"max_iterations"`
	Verdict       aggregator.UseCaseVerdict `json:"verdict"`
	AppliedFiles  []string                  `json:"applied_files"`
	Rationale     string                    `json:"rationale,omitempty"`
}

func runHeal(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected use-case-id as first argument", nil)
		return skillio.ExitScriptError
	}
	useCaseID := args[1]

	plan, err := healDecodePlan(stdin)
	if err != nil {
		skillio.EmitError(stderr, "bad-edit-plan", err.Error(), nil)
		return skillio.ExitScriptError
	}
	if err := healValidatePaths(plan); err != nil {
		skillio.EmitError(stderr, "bad-edit-plan", err.Error(), nil)
		return skillio.ExitScriptError
	}

	repoRoot, err := skillio.FindRepoRoot(cwd)
	if err != nil {
		skillio.EmitError(stderr, "repo-root-not-found", err.Error(), nil)
		return skillio.ExitScriptError
	}

	statePath := filepath.Join(skillio.HarnessDir(repoRoot), "runtime", "heal-state.json")
	state := healLoadState(statePath, useCaseID)
	if state.Iteration >= state.MaxIterations {
		skillio.EmitError(stderr, "heal-exhausted", "heal iteration cap reached; no edit applied", map[string]any{
			"iteration":      state.Iteration,
			"max_iterations": state.MaxIterations,
			"use_case_id":    useCaseID,
		})
		return skillio.ExitScriptError
	}

	b, err := skillruntime.BootLifecycle(repoRoot)
	if err != nil {
		skillio.EmitError(stderr, "boot-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	defer func() { _ = b.Cleanup() }()

	uc, ok := b.UseCases[useCaseID]
	if !ok {
		skillio.EmitError(stderr, "use-case-not-found", fmt.Sprintf("no use case %q", useCaseID), map[string]any{"use_case_id": useCaseID})
		return skillio.ExitScriptError
	}
	if len(uc.ArchetypeScope) == 0 {
		skillio.EmitError(stderr, "no-archetype", "use case has empty archetype_scope", nil)
		return skillio.ExitScriptError
	}
	archetype := uc.ArchetypeScope[0]

	sensors := b.Sensors.ForUseCase(useCaseID)
	pol := skillruntime.LoadPolicies(filepath.Join(skillio.HarnessDir(repoRoot), "policy"), useCaseID)

	snap, err := healSnapshot(repoRoot, plan)
	if err != nil {
		skillio.EmitError(stderr, "snapshot-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	if err := healApplyPlan(repoRoot, plan); err != nil {
		_ = healRestore(snap)
		skillio.EmitError(stderr, "apply-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	ctx := context.Background()
	runner := skillruntime.SensorRunner(func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		return b.Lifecycle.RunSensor(ctx, s.ID, nil)
	})
	aggs, err := skillruntime.RunAll(ctx, sensors, runner, runtime.NumCPU())
	if err != nil {
		_ = healRestore(snap)
		skillio.EmitError(stderr, "scheduler-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	for _, a := range aggs {
		_ = skillio.EmitJSON(stdout, a)
	}

	verdict, aggErr := aggregator.UseCase(uc, archetype, aggs, sensors, pol)
	if aggErr != nil {
		_ = healRestore(snap)
		skillio.EmitError(stderr, "aggregate-failed", aggErr.Error(), nil)
		return skillio.ExitScriptError
	}

	envelope := healEnvelope{
		Iteration:     state.Iteration + 1,
		MaxIterations: state.MaxIterations,
		Verdict:       verdict,
		Rationale:     plan.Rationale,
	}
	for _, f := range plan.Files {
		envelope.AppliedFiles = append(envelope.AppliedFiles, f.Path)
	}

	worst := skillruntime.WorstAggregateVerdict(aggs)
	healed := verdict.Verdict == enums.VerdictPass && worst == enums.VerdictPass
	if healed {
		envelope.Status = "healed"
		state.Iteration = 0
		state.History = nil
	} else {
		envelope.Status = "reverted"
		envelope.AppliedFiles = nil
		if err := healRestore(snap); err != nil {
			skillio.EmitError(stderr, "revert-failed", err.Error(), nil)
			return skillio.ExitScriptError
		}
		state.Iteration++
		state.History = append(state.History, healAttempt{
			AppliedAt: time.Now().UTC(),
			Rationale: plan.Rationale,
			Verdict:   string(worst),
		})
	}
	state.UseCaseID = useCaseID
	if err := healSaveState(statePath, state); err != nil {
		skillio.EmitError(stderr, "persist-state-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	_ = skillio.EmitJSON(stdout, envelope)

	if healed {
		return skillio.ExitPass
	}
	if worst == enums.VerdictInconclusive {
		return skillio.ExitInconclusive
	}
	return skillio.ExitFail
}

func healDecodePlan(r io.Reader) (healloop.EditPlan, error) {
	if r == nil {
		return healloop.EditPlan{}, errors.New("stdin is nil")
	}
	var plan healloop.EditPlan
	dec := json.NewDecoder(r)
	if err := dec.Decode(&plan); err != nil {
		return healloop.EditPlan{}, err
	}
	if len(plan.Files) == 0 {
		return healloop.EditPlan{}, errors.New("edit plan has no files")
	}
	for _, f := range plan.Files {
		switch f.Op {
		case healloop.OpWrite, healloop.OpDelete:
		case "":
			return healloop.EditPlan{}, fmt.Errorf("edit file %q has no op", f.Path)
		default:
			return healloop.EditPlan{}, fmt.Errorf("edit file %q has unknown op %q", f.Path, f.Op)
		}
	}
	return plan, nil
}

func healValidatePaths(plan healloop.EditPlan) error {
	for _, f := range plan.Files {
		if f.Path == "" {
			return errors.New("edit file has empty path")
		}
		if filepath.IsAbs(f.Path) {
			return fmt.Errorf("edit file %q must be repo-root-relative", f.Path)
		}
		clean := filepath.ToSlash(filepath.Clean(f.Path))
		if strings.HasPrefix(clean, "../") || clean == ".." {
			return fmt.Errorf("edit file %q escapes repo root", f.Path)
		}
	}
	return nil
}

type healFileSnapshot struct {
	Path          string
	Existed       bool
	OriginalBytes []byte
}

func healSnapshot(repoRoot string, plan healloop.EditPlan) ([]healFileSnapshot, error) {
	snaps := make([]healFileSnapshot, 0, len(plan.Files))
	for _, f := range plan.Files {
		abs := filepath.Join(repoRoot, f.Path)
		bs, err := os.ReadFile(abs)
		if err == nil {
			snaps = append(snaps, healFileSnapshot{Path: abs, Existed: true, OriginalBytes: bs})
		} else if os.IsNotExist(err) {
			snaps = append(snaps, healFileSnapshot{Path: abs, Existed: false})
		} else {
			return nil, fmt.Errorf("snapshot %s: %w", f.Path, err)
		}
	}
	return snaps, nil
}

func healApplyPlan(repoRoot string, plan healloop.EditPlan) error {
	for _, f := range plan.Files {
		abs := filepath.Join(repoRoot, f.Path)
		switch f.Op {
		case healloop.OpWrite:
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(f.Path), err)
			}
			if err := os.WriteFile(abs, []byte(f.Content), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", f.Path, err)
			}
		case healloop.OpDelete:
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("delete %s: %w", f.Path, err)
			}
		}
	}
	return nil
}

func healRestore(snaps []healFileSnapshot) error {
	var firstErr error
	for _, s := range snaps {
		if s.Existed {
			if err := os.WriteFile(s.Path, s.OriginalBytes, 0o644); err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func healLoadState(path, useCaseID string) healState {
	bs, err := os.ReadFile(path)
	if err != nil {
		return healState{UseCaseID: useCaseID, MaxIterations: healDefaultMaxIterations}
	}
	var s healState
	if err := json.Unmarshal(bs, &s); err != nil {
		return healState{UseCaseID: useCaseID, MaxIterations: healDefaultMaxIterations}
	}
	if s.MaxIterations == 0 {
		s.MaxIterations = healDefaultMaxIterations
	}
	if s.UseCaseID == "" {
		s.UseCaseID = useCaseID
	}
	return s
}

func healSaveState(path string, s healState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	bs, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bs, 0o644)
}
