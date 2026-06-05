// Command validate-use-case backs the /validate-use-case skill.
// See skills/validate-use-case/skill.md.
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

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/policy"
	aggregator "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/lib/skillio"
	"github.com/iurykrieger/lastro/lib/skillruntime"
	"github.com/oklog/ulid/v2"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		skillio.EmitError(os.Stderr, "cwd-failed", err.Error(), nil)
		os.Exit(skillio.ExitScriptError)
	}
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr, cwd))
}

// persistedVerdict adds run-id + per-sensor traceability around the
// aggregator's UseCaseVerdict. Written to verdict.json and emitted as
// the final stdout line.
type persistedVerdict struct {
	UseCaseVerdict aggregator.UseCaseVerdict `json:"use_case_verdict"`
	UseCaseRunID   string                    `json:"use_case_run_id"`
	SensorRuns     []sensorRun               `json:"sensor_runs"`
}

type sensorRun struct {
	SensorID string `json:"sensor_id"`
	Verdict  string `json:"verdict"`
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
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
		skillio.EmitError(stderr, "use-case-not-found", fmt.Sprintf("no use case %q in .harness/use-cases/", useCaseID), map[string]any{"use_case_id": useCaseID})
		return skillio.ExitScriptError
	}
	if len(uc.ArchetypeScope) == 0 {
		skillio.EmitError(stderr, "no-archetype", "use case has empty archetype_scope", map[string]any{"use_case_id": useCaseID})
		return skillio.ExitScriptError
	}
	archetype := uc.ArchetypeScope[0]

	sensors := b.Sensors.GatherForUseCase(useCaseID)

	pol := loadPolicies(filepath.Join(skillio.HarnessDir(repoRoot), "policy"), useCaseID)

	ctx := context.Background()
	runner := func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		return b.Lifecycle.RunSensor(ctx, s.ID, nil)
	}
	aggs, err := skillruntime.RunAll(ctx, sensors, runner, runtime.NumCPU())
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

	verdict, err := aggregator.UseCase(uc, archetype, aggs, sensors, pol)
	if err != nil {
		skillio.EmitError(stderr, "aggregate-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	ucRunID := newULID()
	persisted := persistedVerdict{
		UseCaseVerdict: verdict,
		UseCaseRunID:   ucRunID,
		SensorRuns:     make([]sensorRun, 0, len(aggs)),
	}
	for _, a := range aggs {
		persisted.SensorRuns = append(persisted.SensorRuns, sensorRun{SensorID: a.SensorID, Verdict: string(a.Verdict)})
	}
	if err := writeVerdict(filepath.Join(b.RuntimeRoot, "use-cases", useCaseID, ucRunID, "verdict.json"), persisted); err != nil {
		skillio.EmitError(stderr, "persist-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	if err := skillio.EmitJSON(stdout, persisted); err != nil {
		skillio.EmitError(stderr, "emit-failed", err.Error(), nil)
		return skillio.ExitScriptError
	}

	// The UseCaseVerdict reflects only policy-evaluated angles; any sensor
	// that ran and produced verdict=fail should not be silently ignored even
	// if no policy entry covers its angle. Promote to the worst observed
	// sensor verdict when the use-case verdict would otherwise be better.
	return worstExitCode(verdict.Verdict, aggs)
}

// worstExitCode returns the exit code for the given use-case verdict, but
// promotes to ExitFail (1) if any AggregateSignal in aggs has verdict=fail.
// This prevents a "no obligatory angles" policy from hiding real sensor failures.
func worstExitCode(ucVerdict enums.Verdict, aggs []aggregate.AggregateSignal) int {
	ucCode := skillio.ExitCodeForVerdict(ucVerdict)
	for _, a := range aggs {
		if a.Verdict == enums.VerdictFail {
			return skillio.ExitFail
		}
		if a.Verdict == enums.VerdictInconclusive && ucCode == skillio.ExitPass {
			ucCode = skillio.ExitInconclusive
		}
	}
	return ucCode
}

func loadPolicies(policyDir, useCaseID string) *policy.EffectivePolicy {
	global := loadOne(filepath.Join(policyDir, "global.yaml"))
	local := loadOne(filepath.Join(policyDir, "local", useCaseID+".yaml"))
	return policy.Resolve(global, local)
}

func loadOne(path string) *policy.ValidationPolicy {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	p, err := policy.Load(f)
	if err != nil {
		return nil
	}
	return p
}

func writeVerdict(path string, v persistedVerdict) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func newULID() string {
	ms := ulid.Timestamp(time.Now())
	id, _ := ulid.New(ms, rand.Reader)
	return id.String()
}
