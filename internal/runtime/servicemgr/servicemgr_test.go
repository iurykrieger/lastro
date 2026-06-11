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

func TestLookup_DoesNotChangeRefCount(t *testing.T) {
	fl := newFakeLifecycle(t.TempDir())
	m := New(fl, Options{Ready: alwaysReady, ReadyTimeout: time.Second})
	ctx := context.Background()

	_, _ = m.Acquire(ctx, "run-dev", nil)
	if _, ok := m.Lookup("run-dev"); !ok {
		t.Fatal("lookup miss after acquire")
	}
	_ = m.Release(ctx, "run-dev")
	// assert the fake recorded exactly ONE stop for run-dev (lookup added no ref)
	if fl.stops["run-dev"] != 1 {
		t.Fatalf("stops = %d, want 1 (lookup must not change ref-count)", fl.stops["run-dev"])
	}
}

func TestShutdown_StopsEverythingRegardlessOfRefs(t *testing.T) {
	fl := newFakeLifecycle(t.TempDir())
	m := New(fl, Options{Ready: alwaysReady})
	ctx := context.Background()

	// run-dev held by two consumers, database-query by one.
	for i := 0; i < 2; i++ {
		if _, err := m.Acquire(ctx, "run-dev", nil); err != nil {
			t.Fatalf("acquire run-dev: %v", err)
		}
	}
	if _, err := m.Acquire(ctx, "database-query", nil); err != nil {
		t.Fatalf("acquire database-query: %v", err)
	}

	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if fl.stops["run-dev"] != 1 || fl.stops["database-query"] != 1 {
		t.Fatalf("stops = %+v, want one per service", fl.stops)
	}

	// Idempotent: a second sweep and late Releases are no-ops.
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
	if err := m.Release(ctx, "run-dev"); err != nil {
		t.Fatalf("late release: %v", err)
	}
	if fl.stops["run-dev"] != 1 {
		t.Fatalf("late release re-stopped: stops = %+v", fl.stops)
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
