package servicemgr

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeLifecycle implements ServiceLifecycle for tests. It records starts and
// stops and serves a canned signals path per service.
type fakeLifecycle struct {
	mu     sync.Mutex
	starts map[string]int
	stops  map[string]int
	runDir string
}

func newFakeLifecycle(runDir string) *fakeLifecycle {
	return &fakeLifecycle{starts: map[string]int{}, stops: map[string]int{}, runDir: runDir}
}

func (f *fakeLifecycle) StartService(_ context.Context, serviceID string, expectedObs []string) (Started, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts[serviceID]++
	return Started{RunID: "run-" + serviceID, SignalsPath: f.runDir + "/" + serviceID + "/signals.jsonl"}, nil
}

func (f *fakeLifecycle) StopService(_ context.Context, serviceID, runID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops[serviceID]++
	return nil
}

// awaitReady is injected so the test does not need a real ready signal.
func alwaysReady(context.Context, string, time.Duration) error { return nil }

func TestAcquire_StartsServiceOnce(t *testing.T) {
	fl := newFakeLifecycle(t.TempDir())
	m := New(fl, Options{Ready: alwaysReady, ReadyTimeout: time.Second})

	a1, err := m.Acquire(context.Background(), "run-dev", nil)
	if err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	a2, err := m.Acquire(context.Background(), "run-dev", nil)
	if err != nil {
		t.Fatalf("acquire 2: %v", err)
	}
	if fl.starts["run-dev"] != 1 {
		t.Fatalf("starts = %d, want 1", fl.starts["run-dev"])
	}
	if a1.SignalsPath != a2.SignalsPath {
		t.Fatalf("attachments disagree on signals path: %q vs %q", a1.SignalsPath, a2.SignalsPath)
	}
}

func TestRelease_StopsOnlyOnLastDetach(t *testing.T) {
	fl := newFakeLifecycle(t.TempDir())
	m := New(fl, Options{Ready: alwaysReady})
	ctx := context.Background()

	if _, err := m.Acquire(ctx, "run-dev", nil); err != nil {
		t.Fatalf("setup acquire: %v", err)
	}
	if _, err := m.Acquire(ctx, "run-dev", nil); err != nil {
		t.Fatalf("setup acquire: %v", err)
	}

	if err := m.Release(ctx, "run-dev"); err != nil {
		t.Fatalf("release 1: %v", err)
	}
	if fl.stops["run-dev"] != 0 {
		t.Fatalf("stopped too early: stops = %d", fl.stops["run-dev"])
	}
	if err := m.Release(ctx, "run-dev"); err != nil {
		t.Fatalf("release 2: %v", err)
	}
	if fl.stops["run-dev"] != 1 {
		t.Fatalf("stops = %d, want 1", fl.stops["run-dev"])
	}
	// A subsequent Acquire must start a fresh instance.
	if _, err := m.Acquire(ctx, "run-dev", nil); err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	if fl.starts["run-dev"] != 2 {
		t.Fatalf("starts = %d, want 2 after re-acquire", fl.starts["run-dev"])
	}
}
