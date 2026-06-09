package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
)

// countingRunner exposes how many RunSensor calls are in-flight at
// the same time. The validate path expects bounded parallelism at
// the *use-case* level; this also bounds total sensor concurrency
// when there is one sensor per use case.
type countingRunner struct {
	mu        sync.Mutex
	inflight  atomic.Int64
	maxSeen   atomic.Int64
}

func (c *countingRunner) RunSensor(ctx context.Context, sensorID string, expectedObs []string) (aggregate.AggregateSignal, error) {
	n := c.inflight.Add(1)
	if max := c.maxSeen.Load(); n > max {
		c.maxSeen.Store(n)
	}
	defer c.inflight.Add(-1)
	time.Sleep(50 * time.Millisecond)
	return aggregate.AggregateSignal{
		SchemaVersion: "1.0.0", SensorID: sensorID,
		Verdict: enums.VerdictPass, Rollup: aggregate.RollupCounts{TotalSignals: 1, PassCount: 1},
	}, nil
}

func TestValidate_ConcurrencyBound(t *testing.T) {
	harnessDir, err := filepath.Abs(passingTreeRel)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(harnessDir); err != nil {
		t.Skipf("fixture tree missing: %v", err)
	}

	runner := &countingRunner{}
	makeRunner := func(arts *HarnessArtifacts, repoRoot string) (SensorRunner, ServiceManager, func(), error) {
		return runner, &fakeServiceMgr{}, func() {}, nil
	}

	cfg := &Config{Output: "json", Concurrency: 1, RepoRoot: filepath.Dir(filepath.Dir(harnessDir))}
	out := &bytes.Buffer{}
	if err := runValidateWith(context.Background(), cfg, nil, true, out, makeRunner); err != nil {
		t.Fatalf("runValidateWith: %v", err)
	}

	if max := runner.maxSeen.Load(); max > 1 {
		t.Errorf("maxSeen in-flight = %d, want 1 with --concurrency 1", max)
	}
}
