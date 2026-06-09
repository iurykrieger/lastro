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
