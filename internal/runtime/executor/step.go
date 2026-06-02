package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/runtime/fixturebinder"
	"github.com/iurykrieger/lastro/internal/runtime/process"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/signal"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// stepArgs is the per-step input handed to runStep by Run.
type stepArgs struct {
	Step        sensor.Step
	StepIdx     int
	RunDir      string
	UseCase     *usecase.UseCase
	Store       fixture.FixtureStore
	Resolver    *template.Resolver
	Signaler    process.GroupSignaler
	Shell       []string
	ExpectedObs []string // nil for assertion sensors
	RawLog      *rawLog
	SignalsW    *jsonlWriter
	Stop        <-chan struct{}
	OnStart     func(stepIdx, pid, pgid int)
	// InputEnv and StepOutEnv carry compiled env vars for ${{inputs.X}} and
	// ${{step_outputs.X}} references respectively. Populated by tasks 12/13;
	// ranging over a nil map is a no-op, so callers may leave them unset.
	InputEnv   map[string]string
	StepOutEnv map[string]string
	// OutTag uniquely identifies this step's HARNESS_OUTPUT scratch file.
	// It must be unique across all physical step executions in a run
	// (composed primitive inner steps share consumer step ids, so the bare
	// Step.ID is not unique). When empty, runStep falls back to Step.ID.
	OutTag string
}

// stepOutcome reports the per-step result.
type stepOutcome struct {
	Signals           []signal.Signal
	ObservationKeys   []string
	ExitErr           error // nil if process exited 0
	StoppedExternally bool  // closed via stop channel
	CtxErr            error // ctx.Err() at exit, if any
	Outputs           map[string]string // parsed from $HARNESS_OUTPUT after step exits
}

func runStep(ctx context.Context, a stepArgs) (stepOutcome, error) {
	// 1. Parse + compile Step.Run to env-ref form (fixtures/inputs/step-outputs
	//    become "${HARNESS_*}" references; values are never inlined).
	segs, err := template.Parse(a.Step.Run)
	if err != nil {
		return stepOutcome{}, &TemplateError{Step: a.StepIdx, Cause: err}
	}
	resolved, refs, err := template.Compile(segs)
	if err != nil {
		return stepOutcome{}, &TemplateError{Step: a.StepIdx, Cause: err}
	}

	// 2. Bind fixtures referenced by the compiled run.
	binder := &fixturebinder.Binder{ScratchDir: filepath.Join(a.RunDir, "scratch")}
	if err := os.MkdirAll(binder.ScratchDir, 0o700); err != nil {
		return stepOutcome{}, fmt.Errorf("executor: mkdir scratch: %w", err)
	}
	binding, err := binder.Bind(refs.Fixtures, a.UseCase, a.Store)
	if err != nil {
		return stepOutcome{}, err
	}

	// 3. Build environment.
	// NOTE: entry-point env vars (refs.EntryPoints) are not yet wired here;
	// that is deferred to a future task. The Resolver field on stepArgs is
	// kept for callers that set it, but is no longer called in runStep.
	env := os.Environ()
	for k, v := range binding.Env {
		env = append(env, k+"="+v)
	}
	for k, v := range a.InputEnv {
		env = append(env, k+"="+v)
	}
	for k, v := range a.StepOutEnv {
		env = append(env, k+"="+v)
	}
	outTag := a.OutTag
	if outTag == "" {
		outTag = a.Step.ID
	}
	outPath := filepath.Join(binder.ScratchDir, "stepout-"+outTag)
	if err := os.WriteFile(outPath, nil, 0o600); err != nil {
		return stepOutcome{}, fmt.Errorf("executor: create stepout: %w", err)
	}
	env = append(env,
		"HARNESS_RUN_DIR="+a.RunDir,
		"HARNESS_SCRATCH_DIR="+binder.ScratchDir,
		"HARNESS_OUTPUT="+outPath,
	)

	// 4. Build the command.
	argv := shellArgv(a.Shell, resolved)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = env
	if err := a.Signaler.Spawn(cmd); err != nil {
		return stepOutcome{}, &SpawnError{Step: a.StepIdx, Cause: err}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return stepOutcome{}, &SpawnError{Step: a.StepIdx, Cause: err}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return stepOutcome{}, &SpawnError{Step: a.StepIdx, Cause: err}
	}

	// 5. Spawn.
	if err := cmd.Start(); err != nil {
		return stepOutcome{}, &SpawnError{Step: a.StepIdx, Cause: err}
	}
	pgid, _ := a.Signaler.GroupID(cmd)
	if a.OnStart != nil {
		a.OnStart(a.StepIdx, cmd.Process.Pid, pgid)
	}

	// 6. Concurrent reader goroutines.
	type pumpResult struct {
		out pumpOutput
		err error
	}
	stdoutDone := make(chan pumpResult, 1)
	stderrDone := make(chan error, 1)
	go func() {
		out, err := pumpStdout(stdout, a.StepIdx, a.RawLog, a.SignalsW, a.ExpectedObs != nil)
		stdoutDone <- pumpResult{out: out, err: err}
	}()
	go func() {
		stderrDone <- pumpStderr(stderr, a.StepIdx, a.RawLog)
	}()

	// 7. Stop watcher: kill the process group on stop or ctx cancel.
	stoppedExternally := false
	watcherDone := make(chan struct{})
	exitCh := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-exitCh:
			return
		case <-a.Stop:
			stoppedExternally = true
		case <-ctx.Done():
		}
		_ = a.Signaler.SignalGroup(cmd.Process.Pid, pgid, process.SignalTerm)
		// Hard-kill after 2 seconds if still alive.
		t := time.NewTimer(2 * time.Second)
		defer t.Stop()
		select {
		case <-exitCh:
		case <-t.C:
			_ = a.Signaler.SignalGroup(cmd.Process.Pid, pgid, process.SignalKill)
			<-exitCh
		}
	}()

	// 8. Wait for the process to exit.
	waitErr := cmd.Wait()
	close(exitCh)
	<-watcherDone

	// 9. Drain the readers.
	stdoutRes := <-stdoutDone
	stderrErr := <-stderrDone
	_ = stderrErr // best-effort; raw.log already captured what made it through

	// 10. Note non-zero exit in raw.log for forensics.
	if waitErr != nil && len(stdoutRes.out.Signals) > 0 {
		a.RawLog.WriteAnnotated(a.StepIdx, "exit-nonzero", []byte(strconv.Quote(waitErr.Error())))
	}

	// 11. Parse step outputs written to $HARNESS_OUTPUT.
	stepOut, _ := parseStepOutputFile(outPath)

	return stepOutcome{
		Signals:           stdoutRes.out.Signals,
		ObservationKeys:   stdoutRes.out.ObservationKeys,
		ExitErr:           waitErr,
		StoppedExternally: stoppedExternally,
		CtxErr:            ctx.Err(),
		Outputs:           stepOut,
	}, errors.Join(stdoutRes.err) // stderr errors are non-fatal
}

// envBytes is only used in tests to verify env-var building is stable.
func envBytes(env []string) []byte {
	var b bytes.Buffer
	for _, e := range env {
		b.WriteString(e)
		b.WriteByte('\n')
	}
	return b.Bytes()
}
