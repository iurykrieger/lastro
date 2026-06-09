package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
)

// slowRunner blocks until the context is canceled; verifies that
// cancellation propagates through the use-case scheduler.
type slowRunner struct{}

func (slowRunner) RunSensor(ctx context.Context, sensorID string, expectedObs []string) (aggregate.AggregateSignal, error) {
	select {
	case <-time.After(10 * time.Second):
		return aggregate.AggregateSignal{
			SchemaVersion: "1.0.0", SensorID: sensorID,
			Verdict: enums.VerdictPass, Rollup: aggregate.RollupCounts{TotalSignals: 1, PassCount: 1},
		}, nil
	case <-ctx.Done():
		return aggregate.AggregateSignal{}, ctx.Err()
	}
}

func TestValidate_CancellationViaContext(t *testing.T) {
	harnessDir, err := filepath.Abs(passingTreeRel)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(harnessDir); err != nil {
		t.Skipf("fixture tree missing: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cfg := &Config{Output: "json", RepoRoot: filepath.Dir(filepath.Dir(harnessDir))}
	out := &bytes.Buffer{}

	// Cancel after 100ms so the slowRunner returns ctx.Err.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	makeRunner := func(arts *HarnessArtifacts, repoRoot string) (SensorRunner, ServiceManager, func(), error) {
		return slowRunner{}, &fakeServiceMgr{}, func() {}, nil
	}
	err = runValidateWith(ctx, cfg, nil, true, out, makeRunner)
	// The result is allowed to surface as VerdictInconclusiveError
	// (cancellation demoted via inconclusiveFromError) or as
	// context.Canceled directly. Both map to expected exit codes.
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, VerdictInconclusiveError) {
		t.Errorf("err = %v, want context.Canceled or VerdictInconclusiveError", err)
	}
}

func TestExitCodeFor_SIGTERMTag(t *testing.T) {
	ctx := SetCancelSignal(context.Background(), "SIGTERM")
	got := exitCodeFor(context.Canceled, ctx)
	if got != ExitSignalTerm {
		t.Errorf("exitCodeFor(SIGTERM) = %d, want %d", got, ExitSignalTerm)
	}
}
