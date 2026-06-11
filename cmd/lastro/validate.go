package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	aggregator "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
	"github.com/oklog/ulid/v2"
)

type persistedVerdict struct {
	UseCaseVerdict aggregator.UseCaseVerdict `json:"use_case_verdict"`
	UseCaseRunID   string                    `json:"use_case_run_id"`
	SensorRuns     []sensorRun               `json:"sensor_runs"`
}

type sensorRun struct {
	SensorID string `json:"sensor_id"`
	Verdict  string `json:"verdict"`
}

func runValidateUseCase(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	if len(args) < 2 {
		skillio.EmitError(stderr, "bad-argv", "expected use-case-id as first argument", nil)
		return skillio.ExitScriptError
	}
	useCaseID := args[1]

	repoRoot, err := skillio.FindRepoRoot(cwd)
	if err != nil {
		skillio.EmitError(stderr, "repo-root-not-found", err.Error(), nil)
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
		skillio.EmitError(stderr, "no-archetype", "use case has empty archetype_scope", map[string]any{"use_case_id": useCaseID})
		return skillio.ExitScriptError
	}
	archetype := uc.ArchetypeScope[0]

	sensors := b.Sensors.GatherForUseCase(useCaseID)
	pol := skillruntime.LoadPolicies(filepath.Join(skillio.HarnessDir(repoRoot), "policy"), useCaseID)

	// Shared observational core services (e.g. run-dev) are managed
	// out-of-band — started on first attach, stopped on last detach — so
	// the scheduler never waits on a server that runs forever (issue #34).
	aggs, scheduled, err := skillruntime.RunUseCaseSensors(context.Background(), b, sensors, runtime.NumCPU())
	if err != nil {
		skillio.EmitError(stderr, "scheduler-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	for _, a := range aggs {
		if err := skillio.EmitJSON(stdout, a); err != nil {
			skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
			return skillio.ExitScriptError
		}
	}

	verdict, err := aggregator.UseCase(uc, archetype, aggs, scheduled, pol)
	if err != nil {
		skillio.EmitError(stderr, "aggregate-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	ucRunID := validateNewULID()
	pv := persistedVerdict{
		UseCaseVerdict: verdict,
		UseCaseRunID:   ucRunID,
		SensorRuns:     make([]sensorRun, 0, len(aggs)),
	}
	for _, a := range aggs {
		pv.SensorRuns = append(pv.SensorRuns, sensorRun{SensorID: a.SensorID, Verdict: string(a.Verdict)})
	}
	verdictPath := filepath.Join(b.RuntimeRoot, "use-cases", useCaseID, ucRunID, "verdict.json")
	if err := validateWriteVerdict(verdictPath, pv); err != nil {
		skillio.EmitError(stderr, "persist-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	if err := skillio.EmitJSON(stdout, pv); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}
	return skillruntime.WorstExitCode(verdict.Verdict, aggs)
}

func validateWriteVerdict(path string, v persistedVerdict) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func validateNewULID() string {
	ms := ulid.Timestamp(time.Now())
	id, _ := ulid.New(ms, rand.Reader)
	return id.String()
}
