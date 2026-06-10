# Shared observational services with signal attach — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `run-dev` (and any `core` + `observational` sensor) a single shared, reference-counted service per use-case run that other sensors attach to via its live signal stream, eliminating the duplicate `next dev` / `.next/dev/lock` collision behind issue #33.

**Architecture:** A new ref-counted `servicemgr` orchestrates the existing `lifecycle.StartSensor`/`StopSensor` watcher machinery. The use-case runner classifies `core`+`observational` attach targets as *services* (kept out of the run-to-completion wavefront) and acquires/releases them around each attaching sensor. The executor gains an *attach* step path: when a step's `uses:` (or a sensor's `depends_on:`) names an observational target, it tails that service's `signals.jsonl` and applies the consumer's own matchers instead of re-spawning the primitive. Consumers terminate on completeness (all expected keys seen) or a bounded observation window.

**Tech Stack:** Go 1.x; existing packages `internal/lifecycle`, `internal/runtime/executor`, `internal/sensor`, `internal/aggregate`, `internal/signal`, `internal/enums`; `cmd/harness`. Tests use the standard `testing` package (table-driven, `_test.go` siblings — project convention).

**Spec:** `docs/superpowers/specs/2026-06-08-shared-run-dev-attach-design.md`

---

## Concrete decisions this plan locks in (refining the spec)

- **Consumer attach matching:** the attaching sensor applies its own `signal_matches` regexes to the **`matched_line` evidence string** of each signal the service emits. A match synthesizes the consumer's own signal (re-attributed to the consumer's id/angle). `signal_matches` with `expected: true` feed completeness. This is how "watch specific logs emitted from run-dev" works: `run-dev` emits each server log line as a signal carrying `matched_line`; the consumer greps that.
- **Service "ready":** `run-dev` emits an observation key `ready`. `servicemgr.Acquire` blocks until a signal with `observation_key == "ready"` appears in the service stream, or a readiness timeout elapses.
- **Default observation window:** `defaultObserveWindow = 30 * time.Second`, overridable per sensor via an optional `observe_window` duration string.
- **Attach surface:** both step-level `uses:` and sensor-level `depends_on:` targeting a `core`+`observational` sensor are treated as attach.

---

## File structure

**New files**
- `internal/runtime/sigstream/follow.go` — tails a `signals.jsonl` path, decoding lines into `signal.Signal`, delivering to a callback until done/stop/ctx. Sibling test `follow_test.go`.
- `internal/runtime/servicemgr/servicemgr.go` — ref-counted shared-service manager over a small lifecycle seam. Sibling test `servicemgr_test.go`.
- `internal/runtime/executor/attach.go` — the attach-step path (`execAttachStep`) + consumer matching/termination. Sibling test `attach_test.go`.

**Modified files**
- `internal/sensor/types.go` — add optional `ObserveWindow` field; add `IsService()` / attach-target helpers.
- `internal/sensor/gather.go` — keep gathering services (closure unchanged) but expose classification helper.
- `internal/runtime/executor/executor.go` — add `ServiceAttach` seam to `Options`; thread attachment into step args.
- `internal/runtime/executor/compose.go` — `execTopStep` dispatches to attach when target is observational.
- `internal/lifecycle/lifecycle.go` — second-spawn guard in `StartSensor`/`RunWatcher`; add `ErrServiceAlreadyRunning`.
- `internal/lifecycle/errors.go` — new sentinel error.
- `cmd/harness/usecase_runner.go` — classify services vs regular sensors; acquire/release wiring.
- `cmd/harness/validate_runner.go` — build the `servicemgr` and inject the attach seam.
- `schemas/sensor.yaml` — document `observe_window`; note attach semantics.
- `schemas/examples/sensor/core-run-dev.yaml` — `kind: observational`, `output_type: stream`, emit `ready` + `log-line` signals.

**Follow-on (Phase 6, separate review):** generator scripts under `skills/create-sensors` and `skills/create-core-sensors`; `skills/validate-use-case/SKILL.md`.

---

## Phase 1 — Signal-stream follow reader

A small, pure-ish reader the attach step and `servicemgr` readiness probe both use. No process management here.

### Task 1: `sigstream.Follow` tails a signals.jsonl and decodes signals

**Files:**
- Create: `internal/runtime/sigstream/follow.go`
- Test: `internal/runtime/sigstream/follow_test.go`

- [ ] **Step 1: Write the failing test**

```go
package sigstream

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func TestFollow_DeliversExistingSignalsThenStopsOnDone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signals.jsonl")
	writeLines(t,
		path,
		`{"schema_version":"1.0.0","sensor_id":"run-dev","angle":"environment","emitted_at":"2026-06-09T00:00:00Z","verdict":"pass","confidence":1,"evidence":{"observation_key":"ready","matched_line":"ready - started server"}}`,
	)

	var seen []string
	err := Follow(context.Background(), path, 10*time.Millisecond, nil, func(s Decoded) (done bool) {
		seen = append(seen, s.ObservationKey)
		return s.ObservationKey == "ready" // satisfied on ready
	})
	if err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if len(seen) != 1 || seen[0] != "ready" {
		t.Fatalf("seen = %v, want [ready]", seen)
	}
}

func TestFollow_StopsWhenStopChannelClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signals.jsonl") // never created
	stop := make(chan struct{})
	close(stop)
	if err := Follow(context.Background(), path, 10*time.Millisecond, stop, func(Decoded) bool { return false }); err != nil {
		t.Fatalf("Follow on closed stop: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/sigstream/ -run TestFollow -v`
Expected: FAIL — `undefined: Follow` / `undefined: Decoded`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package sigstream tails a sensor's signals.jsonl file, decoding each
// JSON line into a lightweight Decoded record and delivering it to a
// callback. It is the read side of "attach to a running observational
// service": the writer is the service's watcher (internal/runtime/executor),
// which flushes one JSON signal per line.
package sigstream

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// Decoded is the subset of a signal line the attach machinery needs.
// Evidence is kept raw so consumers can read matched_line or named groups.
type Decoded struct {
	ObservationKey string
	MatchedLine    string
	Raw            map[string]any
}

// Follow tails the JSONL file at path. For every newly appended line that
// decodes as a JSON object it calls onSignal. Follow returns nil when:
//   - onSignal returns true (satisfied), or
//   - stop is closed, or
//   - ctx is done.
// A not-yet-created file is treated as empty and retried every poll interval.
func Follow(ctx context.Context, path string, poll time.Duration, stop <-chan struct{}, onSignal func(Decoded) (done bool)) error {
	if poll <= 0 {
		poll = 50 * time.Millisecond
	}
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stop:
			return nil
		default:
		}

		n, done, err := drain(path, &offset, onSignal)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if n == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-stop:
				return nil
			case <-time.After(poll):
			}
		}
	}
}

// drain reads any new whole lines from path starting at *offset, advances
// *offset past consumed bytes, and invokes onSignal for each decoded object.
// Returns the count of lines consumed and whether onSignal asked to stop.
func drain(path string, offset *int64, onSignal func(Decoded) bool) (int, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, err
	}
	defer f.Close()

	if _, err := f.Seek(*offset, io.SeekStart); err != nil {
		return 0, false, err
	}
	r := bufio.NewReader(f)
	count := 0
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			*offset += int64(len(line))
			count++
			if d, ok := decode(line); ok {
				if onSignal(d) {
					return count, true, nil
				}
			}
		}
		if err != nil { // io.EOF or partial trailing line: stop this pass
			break
		}
	}
	return count, false, nil
}

func decode(line []byte) (Decoded, bool) {
	var m map[string]any
	if err := json.Unmarshal(line, &m); err != nil {
		return Decoded{}, false
	}
	d := Decoded{Raw: m}
	if ev, ok := m["evidence"].(map[string]any); ok {
		if k, ok := ev["observation_key"].(string); ok {
			d.ObservationKey = k
		}
		if ml, ok := ev["matched_line"].(string); ok {
			d.MatchedLine = ml
		}
	}
	return d, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime/sigstream/ -run TestFollow -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/sigstream/
git commit -m "feat(runtime): add sigstream.Follow to tail a service signal stream"
```

### Task 2: `Follow` picks up lines appended after the first pass

**Files:**
- Test: `internal/runtime/sigstream/follow_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestFollow_PicksUpAppendedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signals.jsonl")
	writeLines(t, path, `{"evidence":{"observation_key":"log-line","matched_line":"booting"}}`)

	go func() {
		time.Sleep(30 * time.Millisecond)
		writeLines(t, path, `{"evidence":{"observation_key":"ready","matched_line":"ready - started"}}`)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var keys []string
	err := Follow(ctx, path, 10*time.Millisecond, nil, func(s Decoded) bool {
		keys = append(keys, s.ObservationKey)
		return s.ObservationKey == "ready"
	})
	if err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if len(keys) != 2 || keys[1] != "ready" {
		t.Fatalf("keys = %v, want [log-line ready]", keys)
	}
}
```

- [ ] **Step 2: Run test to verify it fails or passes**

Run: `go test ./internal/runtime/sigstream/ -run TestFollow_PicksUpAppendedLines -v`
Expected: PASS (the implementation already polls). If it FAILS, fix `drain`'s offset handling before continuing.

- [ ] **Step 3: Commit (test only — no impl change expected)**

```bash
git add internal/runtime/sigstream/follow_test.go
git commit -m "test(runtime): cover sigstream.Follow incremental tail"
```

---

## Phase 2 — Reference-counted service manager

`servicemgr` owns shared-service lifetime over a minimal lifecycle seam (so it is unit-testable without spawning processes).

### Task 3: Define the lifecycle seam and a fake for tests

**Files:**
- Create: `internal/runtime/servicemgr/servicemgr.go`
- Test: `internal/runtime/servicemgr/servicemgr_test.go`

- [ ] **Step 1: Write the failing test (fake seam + construction)**

```go
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
	mu       sync.Mutex
	starts   map[string]int
	stops    map[string]int
	runDir   string
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/servicemgr/ -run TestAcquire_StartsServiceOnce -v`
Expected: FAIL — `undefined: New`, `Options`, `Started`, `ServiceLifecycle`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package servicemgr manages reference-counted shared observational
// services for one use-case run. A service (a core + observational sensor
// such as run-dev) is started on the first Acquire, kept alive while at
// least one consumer is attached, and stopped on the last Release.
package servicemgr

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Started is what the lifecycle seam returns when a service is launched.
type Started struct {
	RunID       string
	SignalsPath string
}

// ServiceLifecycle is the slice of *lifecycle.Lifecycle servicemgr needs.
// Production wires the real lifecycle (see cmd/harness); tests inject a fake.
type ServiceLifecycle interface {
	StartService(ctx context.Context, serviceID string, expectedObs []string) (Started, error)
	StopService(ctx context.Context, serviceID, runID string) error
}

// ReadyFunc blocks until the service identified by signalsPath has emitted a
// "ready" observation, or returns an error on timeout.
type ReadyFunc func(ctx context.Context, signalsPath string, timeout time.Duration) error

// Options configures readiness probing.
type Options struct {
	Ready        ReadyFunc
	ReadyTimeout time.Duration
}

// Attachment is the handle a consumer holds onto a running service.
type Attachment struct {
	ServiceID   string
	RunID       string
	SignalsPath string
}

// Manager is the ref-counted service manager. One per use-case run.
type Manager struct {
	lc   ServiceLifecycle
	opts Options

	mu       sync.Mutex
	services map[string]*entry
}

type entry struct {
	runID       string
	signalsPath string
	refs        int
}

// New builds a Manager over the given lifecycle seam.
func New(lc ServiceLifecycle, opts Options) *Manager {
	if opts.ReadyTimeout <= 0 {
		opts.ReadyTimeout = 30 * time.Second
	}
	return &Manager{lc: lc, opts: opts, services: map[string]*entry{}}
}

// Acquire ensures serviceID is running (starting it on the first caller),
// increments its ref-count, and returns an Attachment carrying the live
// signals.jsonl path. It blocks until the service is ready.
func (m *Manager) Acquire(ctx context.Context, serviceID string, expectedObs []string) (Attachment, error) {
	m.mu.Lock()
	if e, ok := m.services[serviceID]; ok {
		e.refs++
		att := Attachment{ServiceID: serviceID, RunID: e.runID, SignalsPath: e.signalsPath}
		m.mu.Unlock()
		return att, nil
	}
	m.mu.Unlock()

	// Start outside the lock (StartService blocks on watcher spawn). Concurrent
	// first-acquirers are reconciled below under the lock.
	started, err := m.lc.StartService(ctx, serviceID, expectedObs)
	if err != nil {
		return Attachment{}, fmt.Errorf("servicemgr: start %q: %w", serviceID, err)
	}

	m.mu.Lock()
	if e, ok := m.services[serviceID]; ok {
		// Lost the race: another goroutine already started it. Stop our extra.
		e.refs++
		att := Attachment{ServiceID: serviceID, RunID: e.runID, SignalsPath: e.signalsPath}
		m.mu.Unlock()
		_ = m.lc.StopService(ctx, serviceID, started.RunID)
		return att, nil
	}
	m.services[serviceID] = &entry{runID: started.RunID, signalsPath: started.SignalsPath, refs: 1}
	m.mu.Unlock()

	if m.opts.Ready != nil {
		if err := m.opts.Ready(ctx, started.SignalsPath, m.opts.ReadyTimeout); err != nil {
			_ = m.Release(ctx, serviceID)
			return Attachment{}, fmt.Errorf("servicemgr: %q not ready: %w", serviceID, err)
		}
	}
	return Attachment{ServiceID: serviceID, RunID: started.RunID, SignalsPath: started.SignalsPath}, nil
}

// Release decrements serviceID's ref-count, stopping the service when it
// reaches zero. Releasing an unknown service is a no-op.
func (m *Manager) Release(ctx context.Context, serviceID string) error {
	m.mu.Lock()
	e, ok := m.services[serviceID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	e.refs--
	if e.refs > 0 {
		m.mu.Unlock()
		return nil
	}
	runID := e.runID
	delete(m.services, serviceID)
	m.mu.Unlock()
	return m.lc.StopService(ctx, serviceID, runID)
}
```

> Note: the lost-race branch double-starts then stops the extra. Real `StartService` (Phase 5 guard) is idempotent-safe because the second-spawn guard makes a concurrent real start fail fast; the fake models the common single-process path. The mutex already serializes Acquire, so the race window only opens if `StartService` is intentionally called outside the lock — keep it inside for production simplicity (see Step 5).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime/servicemgr/ -run TestAcquire_StartsServiceOnce -v`
Expected: PASS.

- [ ] **Step 5: Simplify — start under the lock (kills the race branch)**

Replace the body of `Acquire` so `StartService` runs while holding `m.mu` (services are few and start latency is dominated by the watcher, which is acceptable to serialize per run):

```go
func (m *Manager) Acquire(ctx context.Context, serviceID string, expectedObs []string) (Attachment, error) {
	m.mu.Lock()
	if e, ok := m.services[serviceID]; ok {
		e.refs++
		att := Attachment{ServiceID: serviceID, RunID: e.runID, SignalsPath: e.signalsPath}
		m.mu.Unlock()
		return att, nil
	}
	started, err := m.lc.StartService(ctx, serviceID, expectedObs)
	if err != nil {
		m.mu.Unlock()
		return Attachment{}, fmt.Errorf("servicemgr: start %q: %w", serviceID, err)
	}
	m.services[serviceID] = &entry{runID: started.RunID, signalsPath: started.SignalsPath, refs: 1}
	m.mu.Unlock()

	if m.opts.Ready != nil {
		if err := m.opts.Ready(ctx, started.SignalsPath, m.opts.ReadyTimeout); err != nil {
			_ = m.Release(ctx, serviceID)
			return Attachment{}, fmt.Errorf("servicemgr: %q not ready: %w", serviceID, err)
		}
	}
	return Attachment{ServiceID: serviceID, RunID: started.RunID, SignalsPath: started.SignalsPath}, nil
}
```

Run: `go test ./internal/runtime/servicemgr/ -run TestAcquire_StartsServiceOnce -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/servicemgr/
git commit -m "feat(runtime): ref-counted servicemgr for shared observational services"
```

### Task 4: Release stops the service only on the last detach

**Files:**
- Test: `internal/runtime/servicemgr/servicemgr_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestRelease_StopsOnlyOnLastDetach(t *testing.T) {
	fl := newFakeLifecycle(t.TempDir())
	m := New(fl, Options{Ready: alwaysReady})
	ctx := context.Background()

	_, _ = m.Acquire(ctx, "run-dev", nil)
	_, _ = m.Acquire(ctx, "run-dev", nil)

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
	_, _ = m.Acquire(ctx, "run-dev", nil)
	if fl.starts["run-dev"] != 2 {
		t.Fatalf("starts = %d, want 2 after re-acquire", fl.starts["run-dev"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails or passes**

Run: `go test ./internal/runtime/servicemgr/ -run TestRelease_StopsOnlyOnLastDetach -v`
Expected: PASS (logic implemented in Task 3). If FAIL, fix `Release` ref accounting.

- [ ] **Step 3: Commit**

```bash
git add internal/runtime/servicemgr/servicemgr_test.go
git commit -m "test(runtime): cover servicemgr ref-counted release"
```

---

## Phase 3 — Executor attach path

When a step's `uses:` (or the runner's depends_on attach) targets an observational service, the executor must tail the service stream and apply the consumer's matchers instead of expanding the primitive.

### Task 5: `execAttachStep` consumes a service stream and emits consumer signals

**Files:**
- Create: `internal/runtime/executor/attach.go`
- Test: `internal/runtime/executor/attach_test.go`

- [ ] **Step 1: Write the failing test**

```go
package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/runtime/servicemgr"
	"github.com/iurykrieger/lastro/internal/sensor"
)

func TestExecAttachStep_MatchesServiceLinesAndCompletes(t *testing.T) {
	dir := t.TempDir()
	svcSignals := filepath.Join(dir, "svc-signals.jsonl")
	// run-dev emits each server log line as a signal carrying matched_line.
	if err := os.WriteFile(svcSignals, []byte(
		`{"evidence":{"observation_key":"log-line","matched_line":"GET /health 200"}}`+"\n"+
			`{"evidence":{"observation_key":"log-line","matched_line":"compiled successfully"}}`+"\n",
	), 0o600); err != nil {
		t.Fatalf("seed svc signals: %v", err)
	}

	consumer := sensor.Sensor{
		ID:         "logs-sensor",
		UseCaseID:  "uc-x",
		Angle:      enums.ValidationAngle("logs"),
		Kind:       enums.KindObservational,
		SignalMatches: []sensor.SignalMatch{
			{Key: "compiled", Pattern: "compiled successfully", Verdict: enums.VerdictPass, Expected: true},
		},
	}

	att := servicemgr.Attachment{ServiceID: "run-dev", SignalsPath: svcSignals}
	got := execAttachStep(context.Background(), attachArgs{
		Consumer:      consumer,
		Attachment:    att,
		ExpectedKeys:  []string{"compiled"},
		ObserveWindow: 2 * time.Second,
		Now:           func() time.Time { return time.Unix(0, 0).UTC() },
	})

	if got.TermReason != enums.TerminationCompleted {
		t.Fatalf("term = %q, want completed", got.TermReason)
	}
	if len(got.ObservationKeys) == 0 || got.ObservationKeys[len(got.ObservationKeys)-1] != "compiled" {
		t.Fatalf("observation keys = %v, want to include compiled", got.ObservationKeys)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/executor/ -run TestExecAttachStep -v`
Expected: FAIL — `undefined: execAttachStep`, `attachArgs`.

- [ ] **Step 3: Write minimal implementation**

```go
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/runtime/servicemgr"
	"github.com/iurykrieger/lastro/internal/runtime/sigstream"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/signal"
)

// attachArgs is the input to one attach step: a consumer sensor watching a
// running service's signal stream.
type attachArgs struct {
	Consumer      sensor.Sensor
	Attachment    servicemgr.Attachment
	ExpectedKeys  []string
	ObserveWindow time.Duration
	Now           func() time.Time
	SignalsW      *jsonlWriter // optional; nil in unit tests
	Stop          <-chan struct{}
}

// attachResult mirrors topStepResult's fields the caller needs.
type attachResult struct {
	Signals         []signal.Signal
	ObservationKeys []string
	TermReason      enums.TerminationReason
	StepErr         error
}

// execAttachStep tails the service's signal stream, applies the consumer's
// signal_matches to each service signal's matched_line, emits the consumer's
// own signals, and terminates when every expected key has been observed
// (completed) or the observation window elapses (timeout).
func execAttachStep(ctx context.Context, a attachArgs) attachResult {
	now := a.Now
	if now == nil {
		now = time.Now
	}

	type cm struct {
		key     string
		re      *regexp.Regexp
		verdict enums.Verdict
		conf    float64
		hint    *signal.HealHint
	}
	matchers := make([]cm, 0, len(a.Consumer.SignalMatches))
	for _, sm := range a.Consumer.SignalMatches {
		re, err := regexp.Compile(sm.Pattern)
		if err != nil {
			return attachResult{TermReason: enums.TerminationError, StepErr: fmt.Errorf("attach: bad pattern %q: %w", sm.Key, err)}
		}
		verdict := sm.Verdict
		if verdict == "" {
			verdict = enums.VerdictPass
		}
		conf := 1.0
		if sm.Confidence != nil {
			conf = *sm.Confidence
		}
		var hint *signal.HealHint
		if sm.HealHint != nil {
			hint = &signal.HealHint{Summary: sm.HealHint.Summary, Rationale: sm.HealHint.Rationale}
		}
		matchers = append(matchers, cm{key: sm.Key, re: re, verdict: verdict, conf: conf, hint: hint})
	}

	remaining := map[string]struct{}{}
	for _, k := range a.ExpectedKeys {
		remaining[k] = struct{}{}
	}

	var res attachResult
	window := a.ObserveWindow
	if window <= 0 {
		window = defaultObserveWindow
	}
	wctx, cancel := context.WithTimeout(ctx, window)
	defer cancel()

	err := sigstream.Follow(wctx, a.Attachment.SignalsPath, 0, a.Stop, func(d sigstream.Decoded) bool {
		for _, m := range matchers {
			if !m.re.MatchString(d.MatchedLine) {
				continue
			}
			sig := signal.Signal{
				SchemaVersion: observationSignalSchemaVersion,
				SensorID:      a.Consumer.ID,
				UseCaseID:     a.Consumer.UseCaseID,
				Angle:         a.Consumer.Angle,
				EmittedAt:     now(),
				Verdict:       m.verdict,
				Confidence:    m.conf,
				Evidence:      signal.Evidence{"observation_key": m.key, "matched_line": d.MatchedLine},
				HealHint:      m.hint,
			}
			res.Signals = append(res.Signals, sig)
			res.ObservationKeys = append(res.ObservationKeys, m.key)
			if a.SignalsW != nil {
				if b, err := json.Marshal(sig); err == nil {
					_ = a.SignalsW.WriteLine(b)
				}
			}
			delete(remaining, m.key)
		}
		return len(remaining) == 0 // satisfied-on-completeness
	})

	switch {
	case err == nil:
		res.TermReason = enums.TerminationCompleted
	case err == context.DeadlineExceeded:
		// Window elapsed without full completeness; not an error — the rollup
		// turns missing expected keys into the verdict.
		res.TermReason = enums.TerminationCompleted
	case err == context.Canceled:
		res.TermReason = enums.TerminationStopped
	default:
		res.TermReason = enums.TerminationError
		res.StepErr = err
	}
	return res
}

// defaultObserveWindow bounds how long an attaching observational consumer
// watches a service stream before rolling up on completeness.
const defaultObserveWindow = 30 * time.Second
```

> `context.DeadlineExceeded` maps to `TerminationCompleted` (not timeout) so the consumer's rollup derives its verdict from observation completeness, per the spec's satisfied-on-completeness model. If a future requirement wants window-exhaustion to read as `timeout`, change this one switch.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime/executor/ -run TestExecAttachStep -v`
Expected: PASS.

- [ ] **Step 5: Add the window-elapse test**

```go
func TestExecAttachStep_WindowElapsesWithoutAllKeys(t *testing.T) {
	dir := t.TempDir()
	svc := filepath.Join(dir, "svc.jsonl")
	if err := os.WriteFile(svc, []byte(`{"evidence":{"observation_key":"log-line","matched_line":"only this line"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	consumer := sensor.Sensor{
		ID: "logs", Angle: enums.ValidationAngle("logs"), Kind: enums.KindObservational,
		SignalMatches: []sensor.SignalMatch{{Key: "never", Pattern: "this never appears", Verdict: enums.VerdictPass, Expected: true}},
	}
	got := execAttachStep(context.Background(), attachArgs{
		Consumer: consumer, Attachment: servicemgr.Attachment{SignalsPath: svc},
		ExpectedKeys: []string{"never"}, ObserveWindow: 150 * time.Millisecond,
		Now: func() time.Time { return time.Unix(0, 0).UTC() },
	})
	if got.TermReason != enums.TerminationCompleted {
		t.Fatalf("term = %q, want completed (verdict comes from completeness)", got.TermReason)
	}
	if len(got.ObservationKeys) != 0 {
		t.Fatalf("keys = %v, want none", got.ObservationKeys)
	}
}
```

Run: `go test ./internal/runtime/executor/ -run TestExecAttachStep_WindowElapsesWithoutAllKeys -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/runtime/executor/attach.go internal/runtime/executor/attach_test.go
git commit -m "feat(executor): attach-step path consuming a shared service signal stream"
```

### Task 6: Dispatch to attach when a uses-step targets an observational sensor

**Files:**
- Modify: `internal/runtime/executor/executor.go` (Options + threading)
- Modify: `internal/runtime/executor/compose.go:51-56` (`execTopStep` dispatch)
- Test: `internal/runtime/executor/attach_test.go`

- [ ] **Step 1: Write the failing test (dispatch via a fake ServiceAttach)**

```go
func TestExecTopStep_DispatchesToAttachForObservableTarget(t *testing.T) {
	dir := t.TempDir()
	svc := filepath.Join(dir, "svc.jsonl")
	if err := os.WriteFile(svc, []byte(`{"evidence":{"observation_key":"log-line","matched_line":"ready - started server"}}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	e := New(Options{
		Now: func() time.Time { return time.Unix(0, 0).UTC() },
		SensorLookup: func(id string) (sensor.Sensor, bool) {
			if id == "run-dev" {
				return sensor.Sensor{ID: "run-dev", Scope: enums.ScopeCore, Kind: enums.KindObservational}, true
			}
			return sensor.Sensor{}, false
		},
		ServiceAttach: func(_ context.Context, serviceID string) (servicemgr.Attachment, bool) {
			return servicemgr.Attachment{ServiceID: serviceID, SignalsPath: svc}, true
		},
	})

	consumer := sensor.Sensor{
		ID: "logs", UseCaseID: "uc", Angle: enums.ValidationAngle("logs"), Kind: enums.KindObservational,
		SignalMatches: []sensor.SignalMatch{{Key: "ready", Pattern: "ready - started", Verdict: enums.VerdictPass, Expected: true}},
	}
	idx := 0
	res := e.execTopStep(context.Background(), topStepArgs{
		Sensor:    consumer,
		Step:      sensor.Step{ID: "watch", Uses: "run-dev"},
		GlobalIdx: &idx,
		RunDir:    dir,
	})
	if res.TermReason != enums.TerminationCompleted {
		t.Fatalf("term = %q, want completed", res.TermReason)
	}
	if len(res.ObservationKeys) != 1 || res.ObservationKeys[0] != "ready" {
		t.Fatalf("keys = %v, want [ready]", res.ObservationKeys)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/executor/ -run TestExecTopStep_DispatchesToAttachForObservableTarget -v`
Expected: FAIL — `Options` has no `ServiceAttach` field; `execTopStep` does not dispatch to attach.

- [ ] **Step 3: Add the `ServiceAttach` seam to Options**

In `internal/runtime/executor/executor.go`, add to the `Options` struct (after `SensorLookup`):

```go
	// ServiceAttach resolves a running shared service (a core + observational
	// sensor) to its live Attachment. Returns false when the target is not a
	// managed service, in which case a uses-step expands inline as before.
	// Wired from the use-case runner's *servicemgr.Manager; nil outside the
	// validate flow (uses-steps then always expand).
	ServiceAttach func(ctx context.Context, serviceID string) (servicemgr.Attachment, bool)
```

Add the import `"github.com/iurykrieger/lastro/internal/runtime/servicemgr"` to `executor.go`.

- [ ] **Step 4: Dispatch in `execTopStep`**

In `internal/runtime/executor/compose.go`, replace `execTopStep` (lines 51-56):

```go
func (e *Executor) execTopStep(ctx context.Context, a topStepArgs) topStepResult {
	if a.Step.Uses != "" {
		if e.opts.SensorLookup != nil {
			if prim, ok := e.opts.SensorLookup(a.Step.Uses); ok && prim.Kind == enums.KindObservational {
				return e.attachToService(ctx, a, prim)
			}
		}
		return e.execUsesStep(ctx, a)
	}
	return e.execRunStep(ctx, a)
}

// attachToService runs the attach-step path for a uses-step whose target is a
// shared observational service. Falls back to inline expansion if no service
// is registered (ServiceAttach nil or returns false) so non-validate callers
// keep working.
func (e *Executor) attachToService(ctx context.Context, a topStepArgs, prim sensor.Sensor) topStepResult {
	if e.opts.ServiceAttach == nil {
		return e.execUsesStep(ctx, a)
	}
	att, ok := e.opts.ServiceAttach(ctx, prim.ID)
	if !ok {
		return e.execUsesStep(ctx, a)
	}
	*a.GlobalIdx++
	r := execAttachStep(ctx, attachArgs{
		Consumer:      a.Sensor,
		Attachment:    att,
		ExpectedKeys:  expectedKeysOf(a.Sensor),
		ObserveWindow: observeWindowOf(a.Sensor),
		Now:           e.opts.Now,
		SignalsW:      a.SignalsW,
		Stop:          a.Stop,
	})
	return topStepResult{
		Signals:         r.Signals,
		ObservationKeys: r.ObservationKeys,
		Outputs:         map[string]string{},
		TermReason:      r.TermReason,
		StepErr:         r.StepErr,
	}
}

// expectedKeysOf returns the consumer's expected observation keys (pass
// matchers flagged expected:true).
func expectedKeysOf(s sensor.Sensor) []string {
	var keys []string
	for _, sm := range s.SignalMatches {
		if sm.Expected {
			keys = append(keys, sm.Key)
		}
	}
	return keys
}

// observeWindowOf returns the sensor's ObserveWindow (parsed) or 0 for default.
func observeWindowOf(s sensor.Sensor) time.Duration {
	if s.ObserveWindow == "" {
		return 0
	}
	d, err := time.ParseDuration(s.ObserveWindow)
	if err != nil {
		return 0
	}
	return d
}
```

Add imports to `compose.go`: `"time"` and (already present) `enums`, `sensor`.

- [ ] **Step 5: Add the `ObserveWindow` field to the Sensor type**

In `internal/sensor/types.go`, add to `Sensor` (after `OutputType`):

```go
	// ObserveWindow optionally bounds how long an attaching observational
	// sensor watches a shared service's signal stream before rolling up on
	// completeness. A Go duration string ("45s"); empty means runtime default.
	ObserveWindow string `json:"observe_window,omitempty"`
```

- [ ] **Step 6: Run the dispatch test**

Run: `go test ./internal/runtime/executor/ -run TestExecTopStep_DispatchesToAttachForObservableTarget -v`
Expected: PASS.

- [ ] **Step 7: Run the whole executor package to catch regressions**

Run: `go test ./internal/runtime/executor/...`
Expected: PASS (existing uses-step expansion tests still pass — non-observational targets unaffected).

- [ ] **Step 8: Commit**

```bash
git add internal/runtime/executor/ internal/sensor/types.go
git commit -m "feat(executor): dispatch uses-step to attach for observational targets"
```

---

## Phase 4 — Use-case runner classification & wiring

### Task 7: Classify services and exclude them from the wavefront node set

**Files:**
- Modify: `cmd/harness/usecase_runner.go`
- Test: `cmd/harness/usecase_runner_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestClassifyServices_SplitsCoreObservationalTargets(t *testing.T) {
	sensors := []sensor.Sensor{
		{ID: "run-dev", Scope: enums.ScopeCore, Kind: enums.KindObservational},
		{ID: "logs", UseCaseID: "uc", Kind: enums.KindObservational, Steps: []sensor.Step{{ID: "watch", Uses: "run-dev"}}},
		{ID: "unit", UseCaseID: "uc", Kind: enums.KindAssertion, Steps: []sensor.Step{{ID: "t", Run: "go test ./..."}}},
	}
	services, regular := classifyServices(sensors)
	if len(services) != 1 || services[0].ID != "run-dev" {
		t.Fatalf("services = %v, want [run-dev]", services)
	}
	ids := []string{}
	for _, s := range regular {
		ids = append(ids, s.ID)
	}
	if len(ids) != 2 {
		t.Fatalf("regular = %v, want logs+unit", ids)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/harness/ -run TestClassifyServices_SplitsCoreObservationalTargets -v`
Expected: FAIL — `undefined: classifyServices`.

- [ ] **Step 3: Write minimal implementation**

Add to `cmd/harness/usecase_runner.go`:

```go
// classifyServices partitions gathered sensors into shared services (core +
// observational sensors that other sensors attach to) and regular sensors.
// Services are managed by servicemgr — started on first attach, reaped on
// last detach — and are NOT scheduled as run-to-completion wavefront nodes.
func classifyServices(sensors []sensor.Sensor) (services, regular []sensor.Sensor) {
	for _, s := range sensors {
		if s.Scope == enums.ScopeCore && s.Kind == enums.KindObservational {
			services = append(services, s)
			continue
		}
		regular = append(regular, s)
	}
	return services, regular
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/harness/ -run TestClassifyServices_SplitsCoreObservationalTargets -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/harness/usecase_runner.go cmd/harness/usecase_runner_test.go
git commit -m "feat(harness): classify shared services out of the wavefront node set"
```

### Task 8: Acquire/release a service around an attaching sensor's run

**Files:**
- Modify: `cmd/harness/usecase_runner.go` (`RunUseCase`, `runUseCaseSensors`)
- Test: `cmd/harness/usecase_runner_test.go`

- [ ] **Step 1: Write the failing test (fake manager records acquire/release)**

```go
type fakeServiceMgr struct {
	mu       sync.Mutex
	acquired []string
	released []string
}

func (f *fakeServiceMgr) Acquire(_ context.Context, id string, _ []string) (servicemgr.Attachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquired = append(f.acquired, id)
	return servicemgr.Attachment{ServiceID: id, SignalsPath: "/tmp/" + id + ".jsonl"}, nil
}

func (f *fakeServiceMgr) Release(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, id)
	return nil
}

func TestRunUseCaseSensors_AcquiresAndReleasesServices(t *testing.T) {
	owned := []sensor.Sensor{
		{ID: "logs", UseCaseID: "uc", Kind: enums.KindObservational, Steps: []sensor.Step{{ID: "watch", Uses: "run-dev"}}},
	}
	fsm := &fakeServiceMgr{}
	runner := fakeRunner{} // existing test helper returning a pass aggregate

	_, err := runUseCaseSensors(context.Background(), runner, owned, nil, fsm, serviceDepsOf(owned, map[string]bool{"run-dev": true}))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(fsm.acquired) != 1 || fsm.acquired[0] != "run-dev" {
		t.Fatalf("acquired = %v, want [run-dev]", fsm.acquired)
	}
	if len(fsm.released) != 1 || fsm.released[0] != "run-dev" {
		t.Fatalf("released = %v, want [run-dev]", fsm.released)
	}
}
```

> If `fakeRunner` does not yet exist in the test file, add a minimal one:
> ```go
> type fakeRunner struct{}
> func (fakeRunner) RunSensor(_ context.Context, sensorID string, _ []string) (aggregate.AggregateSignal, error) {
> 	return aggregate.AggregateSignal{SchemaVersion: "1.0.0", Type: aggregate.TypeAggregate, SensorID: sensorID, Verdict: enums.VerdictPass, Confidence: 1}, nil
> }
> ```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/harness/ -run TestRunUseCaseSensors_AcquiresAndReleasesServices -v`
Expected: FAIL — `runUseCaseSensors` has the old signature; `serviceDepsOf` undefined; `ServiceManager` interface undefined.

- [ ] **Step 3: Define the manager seam and dependency mapping**

Add to `cmd/harness/usecase_runner.go`:

```go
// ServiceManager is the slice of *servicemgr.Manager the runner needs.
type ServiceManager interface {
	Acquire(ctx context.Context, serviceID string, expectedObs []string) (servicemgr.Attachment, error)
	Release(ctx context.Context, serviceID string) error
}

// serviceDepsOf maps each regular sensor id to the service ids it attaches to
// (via a uses-step or a depends_on edge that names a known service).
func serviceDepsOf(sensors []sensor.Sensor, isService map[string]bool) map[string][]string {
	deps := map[string][]string{}
	for _, s := range sensors {
		seen := map[string]bool{}
		add := func(id string) {
			if isService[id] && !seen[id] {
				seen[id] = true
				deps[s.ID] = append(deps[s.ID], id)
			}
		}
		for _, d := range s.DependsOn {
			add(d)
		}
		for _, st := range s.Steps {
			if st.Uses != "" {
				add(st.Uses)
			}
		}
	}
	return deps
}
```

Add imports: `"github.com/iurykrieger/lastro/internal/runtime/servicemgr"`.

- [ ] **Step 4: Thread acquire/release through `runUseCaseSensors`**

Change `runUseCaseSensors`'s signature and body to acquire each sensor's services before `RunSensor` and release them after (deferred inside the goroutine so release is guaranteed):

```go
func runUseCaseSensors(
	ctx context.Context,
	runner SensorRunner,
	sensors []sensor.Sensor,
	observationKeys map[string][]string,
	mgr ServiceManager,
	serviceDeps map[string][]string,
) ([]aggregate.AggregateSignal, error) {
	sorted, err := sensor.ResolveExecutionOrder(sensors)
	if err != nil {
		return nil, fmt.Errorf("topo sort: %w", err)
	}
	layers := wavefronts(sorted)

	results := make([]aggregate.AggregateSignal, 0, len(sorted))
	resultsMu := sync.Mutex{}

	for _, layer := range layers {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		var wg sync.WaitGroup
		for _, s := range layer {
			wg.Add(1)
			go func(s sensor.Sensor) {
				defer wg.Done()
				// Acquire the services this sensor attaches to; release on exit.
				acquired := make([]string, 0, len(serviceDeps[s.ID]))
				defer func() {
					for _, id := range acquired {
						_ = mgr.Release(ctx, id)
					}
				}()
				for _, svc := range serviceDeps[s.ID] {
					if _, err := mgr.Acquire(ctx, svc, nil); err != nil {
						resultsMu.Lock()
						results = append(results, inconclusiveFromError(s, err))
						resultsMu.Unlock()
						return
					}
					acquired = append(acquired, svc)
				}
				agg, err := runner.RunSensor(ctx, s.ID, observationKeys[s.ID])
				if err != nil {
					agg = inconclusiveFromError(s, err)
				}
				resultsMu.Lock()
				results = append(results, agg)
				resultsMu.Unlock()
			}(s)
		}
		wg.Wait()
	}

	sort.Slice(results, func(i, j int) bool { return results[i].SensorID < results[j].SensorID })
	return results, nil
}
```

- [ ] **Step 5: Update `RunUseCase` to classify and pass the manager**

In `RunUseCase`, after `owned := arts.Sensors.GatherForUseCase(useCaseID)` and the empty check, replace the `runUseCaseSensors` call site:

```go
	services, regular := classifyServices(owned)
	isService := make(map[string]bool, len(services))
	for _, s := range services {
		isService[s.ID] = true
	}
	deps := serviceDepsOf(regular, isService)

	signals, err := runUseCaseSensors(ctx, runner, regular, nil, mgr, deps)
	if err != nil {
		return UseCaseRunResult{}, err
	}
```

Change `RunUseCase`'s signature to accept the manager: add `mgr ServiceManager` as a parameter (after `runner SensorRunner`). Update the aggregator call to pass `regular` (the scheduled set) where it currently passes `owned` for the `sensors []sensor.Sensor` argument of `aggregateUseCase`.

> Services are excluded from `regular`, so the per-use-case aggregator no longer sees `run-dev` as a use-case sensor to verdict — correct, because services are infrastructure, not validated angles. Confirm `aggregator.UseCase` tolerates the service's absence (it keys off the use case's policy angles, which never include `environment` core services).

- [ ] **Step 6: Run the test**

Run: `go test ./cmd/harness/ -run TestRunUseCaseSensors_AcquiresAndReleasesServices -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/harness/usecase_runner.go cmd/harness/usecase_runner_test.go
git commit -m "feat(harness): acquire/release shared services around attaching sensors"
```

### Task 9: Wire the real servicemgr + attach seam in `validate_runner.go`

**Files:**
- Modify: `cmd/harness/validate_runner.go` (`defaultRunnerFactory`, `RunUseCase` call site)
- Modify: `cmd/harness/validate_runner.go` add a `lifecycleService` adapter
- Test: `cmd/harness/validate_test.go` (smoke: factory builds without error)

- [ ] **Step 1: Write the failing test**

```go
func TestDefaultRunnerFactory_BuildsServiceManager(t *testing.T) {
	arts := &HarnessArtifacts{
		Sensors:     emptySensorStore(t), // existing helper or sensor.NewStore(nil)
		UseCases:    map[string]*usecase.UseCase{},
		RuntimeRoot: t.TempDir(),
	}
	runner, mgr, cleanup, err := defaultRunnerFactory(arts, t.TempDir())
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	defer cleanup()
	if runner == nil || mgr == nil {
		t.Fatalf("factory returned nil runner/mgr")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/harness/ -run TestDefaultRunnerFactory_BuildsServiceManager -v`
Expected: FAIL — factory returns 3 values, not 4; `mgr` unknown.

- [ ] **Step 3: Add a lifecycle adapter implementing servicemgr.ServiceLifecycle**

In `cmd/harness/validate_runner.go`:

```go
// lifecycleService adapts *lifecycle.Lifecycle to servicemgr.ServiceLifecycle.
type lifecycleService struct {
	lc          *lifecycle.Lifecycle
	mu          sync.Mutex
	handles     map[string]*lifecycle.Handle // serviceID -> running handle
}

func (a *lifecycleService) StartService(ctx context.Context, serviceID string, expectedObs []string) (servicemgr.Started, error) {
	h, err := a.lc.StartSensor(ctx, serviceID, expectedObs)
	if err != nil {
		return servicemgr.Started{}, err
	}
	a.mu.Lock()
	a.handles[serviceID] = h
	a.mu.Unlock()
	return servicemgr.Started{RunID: h.RunID, SignalsPath: filepath.Join(h.RunDir, "signals.jsonl")}, nil
}

func (a *lifecycleService) StopService(ctx context.Context, serviceID, runID string) error {
	a.mu.Lock()
	h := a.handles[serviceID]
	delete(a.handles, serviceID)
	a.mu.Unlock()
	if h == nil {
		return nil
	}
	_, err := a.lc.StopSensor(ctx, h)
	return err
}

// readyByObservation blocks until a "ready" observation_key appears in the
// service's signal stream, or timeout elapses.
func readyByObservation(ctx context.Context, signalsPath string, timeout time.Duration) error {
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return sigstream.Follow(wctx, signalsPath, 0, nil, func(d sigstream.Decoded) bool {
		return d.ObservationKey == "ready"
	})
}
```

Add imports: `"path/filepath"`, `"github.com/iurykrieger/lastro/internal/runtime/servicemgr"`, `"github.com/iurykrieger/lastro/internal/runtime/sigstream"`.

- [ ] **Step 4: Change `defaultRunnerFactory` to also build the manager and attach seam**

Change the `runnerFactory` type and `defaultRunnerFactory` to return `(SensorRunner, ServiceManager, func(), error)`. Build the manager and inject `ServiceAttach`:

```go
type runnerFactory func(arts *HarnessArtifacts, repoRoot string) (SensorRunner, ServiceManager, func(), error)

func defaultRunnerFactory(arts *HarnessArtifacts, repoRoot string) (SensorRunner, ServiceManager, func(), error) {
	// ... existing sensorIndex/useCaseLookup/entryPoints/resolver setup unchanged ...

	lc := lifecycle.New(lifecycle.Options{
		Sensors:     lifecycle.WrapSensorStore(arts.Sensors),
		Executor:    nil, // set below after we can reference the manager
		RuntimeRoot: arts.RuntimeRoot,
		Version:     HarnessVersion,
	})
	svc := &lifecycleService{lc: lc, handles: map[string]*lifecycle.Handle{}}
	mgr := servicemgr.New(svc, servicemgr.Options{Ready: readyByObservation, ReadyTimeout: 60 * time.Second})

	exec := executor.New(executor.Options{
		RepoRoot:      repoRoot,
		Resolver:      resolver,
		FixtureStore:  arts.Fixtures,
		UseCaseLookup: useCaseLookup,
		SensorLookup:  arts.Sensors.LookupSensor,
		Now:           time.Now,
		ServiceAttach: func(ctx context.Context, serviceID string) (servicemgr.Attachment, bool) {
			att, err := mgr.Acquire(ctx, serviceID, nil)
			if err != nil {
				return servicemgr.Attachment{}, false
			}
			return att, true
		},
	})
	// Rebuild lc with the executor now that it exists (lifecycle copies Options).
	lc = lifecycle.New(lifecycle.Options{
		Sensors:     lifecycle.WrapSensorStore(arts.Sensors),
		Executor:    exec,
		RuntimeRoot: arts.RuntimeRoot,
		Version:     HarnessVersion,
	})
	svc.lc = lc

	cleanup := func() {}
	return lc, mgr, cleanup, nil
}
```

> **Important sequencing:** the executor's `ServiceAttach` calls `mgr.Acquire`, which already increments the ref-count for the service. The runner's `runUseCaseSensors` ALSO acquires (Task 8). To avoid double ref-counting, the runner's acquire is what manages lifetime; the executor seam must NOT re-acquire. Fix in Step 5.

- [ ] **Step 5: Make the executor seam read the already-acquired attachment (no second Acquire)**

The runner acquires before running the sensor; the executor only needs the attachment, not another ref. Add a read-only lookup to the manager and use it in the seam:

In `internal/runtime/servicemgr/servicemgr.go` add:

```go
// Lookup returns the current attachment for a running service without
// changing its ref-count. Used by the executor attach seam, which runs only
// after the runner has already Acquired the service.
func (m *Manager) Lookup(serviceID string) (Attachment, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.services[serviceID]
	if !ok {
		return Attachment{}, false
	}
	return Attachment{ServiceID: serviceID, RunID: e.runID, SignalsPath: e.signalsPath}, true
}
```

Add a unit test in `servicemgr_test.go`:

```go
func TestLookup_DoesNotChangeRefCount(t *testing.T) {
	fl := newFakeLifecycle(t.TempDir())
	m := New(fl, Options{Ready: alwaysReady})
	ctx := context.Background()
	_, _ = m.Acquire(ctx, "run-dev", nil)
	if _, ok := m.Lookup("run-dev"); !ok {
		t.Fatal("lookup miss after acquire")
	}
	_ = m.Release(ctx, "run-dev")
	if fl.stops["run-dev"] != 1 {
		t.Fatalf("stops = %d, want 1 (lookup must not add a ref)", fl.stops["run-dev"])
	}
}
```

Then change the `ServiceAttach` closure in Step 4 to use `mgr.Lookup` instead of `mgr.Acquire`:

```go
		ServiceAttach: func(_ context.Context, serviceID string) (servicemgr.Attachment, bool) {
			return mgr.Lookup(serviceID)
		},
```

- [ ] **Step 6: Update the `RunUseCase` and `runValidateWith` call sites**

`runValidateWith` calls `makeRunner` (now returns 4 values) and `RunUseCase` (now takes `mgr`). Update both:

```go
	runner, mgr, cleanup, err := makeRunner(arts, repoRoot)
	if err != nil {
		return fmt.Errorf("init runner: %w", err)
	}
	defer cleanup()
	// ... inside the per-use-case goroutine:
			r, err := RunUseCase(ctx, runner, mgr, arts, id)
```

Update any other `defaultRunnerFactory`/`RunUseCase` callers and the test fake `runnerFactory` closures to the new arity.

- [ ] **Step 7: Run the factory test + the full cmd/harness package**

Run: `go test ./cmd/harness/...`
Expected: PASS (fix any call-site arity mismatches the compiler flags).

- [ ] **Step 8: Commit**

```bash
git add cmd/harness/ internal/runtime/servicemgr/
git commit -m "feat(harness): wire servicemgr + attach seam into the validate runner"
```

---

## Phase 5 — Second-spawn guard, run-dev artifact, and #33 regression

### Task 10: Lifecycle second-spawn guard for live services

**Files:**
- Modify: `internal/lifecycle/errors.go`
- Modify: `internal/lifecycle/lifecycle.go` (`StartSensor`)
- Test: `internal/lifecycle/lifecycle_test.go` (or the existing handles/start test file)

- [ ] **Step 1: Write the failing test**

```go
func TestStartSensor_RejectsSecondLiveInstanceOfSameSensor(t *testing.T) {
	lc := newTestLifecycle(t) // existing helper constructing a Lifecycle over a temp runtime root + a fake executor that blocks until stopped
	ctx := context.Background()

	h1, err := lc.StartSensor(ctx, "run-dev", []string{"ready"})
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer func() { _, _ = lc.StopSensor(ctx, h1) }()

	_, err = lc.StartSensor(ctx, "run-dev", []string{"ready"})
	if !errors.Is(err, ErrServiceAlreadyRunning) {
		t.Fatalf("second start err = %v, want ErrServiceAlreadyRunning", err)
	}
}
```

> If `newTestLifecycle` does not exist, model it on the existing `internal/lifecycle/*_test.go` setup (they already construct a Lifecycle with a fake executor). Reuse that helper rather than inventing a new one.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lifecycle/ -run TestStartSensor_RejectsSecondLiveInstanceOfSameSensor -v`
Expected: FAIL — `undefined: ErrServiceAlreadyRunning`; no guard.

- [ ] **Step 3: Add the sentinel error**

In `internal/lifecycle/errors.go`, add:

```go
	// ErrServiceAlreadyRunning is returned by StartSensor when a live registry
	// entry already exists for the same sensor id. Prevents a second
	// host-exclusive service (e.g. run-dev) from racing the first on a shared
	// resource such as .next/dev/lock.
	ErrServiceAlreadyRunning = errors.New("lifecycle: a live instance of this sensor is already running")
```

- [ ] **Step 4: Add the guard in `StartSensor`**

In `internal/lifecycle/lifecycle.go`, inside `StartSensor`, immediately after `_, _ = l.pruneDead()` (around line 221) and before minting the run key:

```go
	// Second-spawn guard: refuse to start a sensor that already has a live
	// registry entry. Shared observational services are host-exclusive
	// (e.g. run-dev holds .next/dev/lock); a duplicate would crash on the lock.
	if existing, err := l.registry.List(); err == nil {
		for _, h := range existing {
			if h.SensorID == sensorID {
				return nil, fmt.Errorf("%w: %s (run %s)", ErrServiceAlreadyRunning, sensorID, h.RunID)
			}
		}
	}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/lifecycle/ -run TestStartSensor_RejectsSecondLiveInstanceOfSameSensor -v`
Expected: PASS.

- [ ] **Step 6: Run the lifecycle package**

Run: `go test ./internal/lifecycle/...`
Expected: PASS. If any existing test starts the same sensor id twice without stopping, update it to stop between starts (the guard is intended).

- [ ] **Step 7: Commit**

```bash
git add internal/lifecycle/
git commit -m "feat(lifecycle): guard against a second live instance of a sensor"
```

### Task 11: Convert the run-dev example to an observational service

**Files:**
- Modify: `schemas/examples/sensor/core-run-dev.yaml`
- Test: `schemas/schemas_test.go` (the example must still validate against `sensor.yaml`)

- [ ] **Step 1: Update the example**

Replace `schemas/examples/sensor/core-run-dev.yaml` with:

```yaml
schema_version: 1.0.0
id: run-dev
scope: core
angle: environment
kind: observational
nature: computational
output_type: stream
uses: []
signal_matches:
  # Readiness: servicemgr.Acquire blocks until this key appears.
  - key: ready
    pattern: "ready|started server|compiled successfully"
    verdict: pass
    expected: true
  # Firehose: emit every server log line so attaching sensors (e.g. logs)
  # can grep matched_line for their own patterns.
  - key: log-line
    pattern: ".+"
    verdict: pass
steps:
  - id: boot
    run: "make dev"
```

> `make dev` runs the long-lived server; the watcher tails its output, the `ready` matcher fires when the server is serving, and `log-line` re-emits each line. No `wait-ready` curl step is needed — readiness is now signal-driven.

- [ ] **Step 2: Run the schema example test**

Run: `go test ./schemas/ -run Example -v` (or the test that loads `schemas/examples/**`)
Expected: PASS — the example validates against `sensor.yaml`. If the schema rejects `observe_window` or the new shape, proceed to Step 3.

- [ ] **Step 3: Allow `observe_window` in the schema**

In `schemas/sensor.yaml`, under `properties:`, add:

```yaml
  observe_window:
    type: string
    description: |
      Optional Go duration ("45s") bounding how long an attaching
      observational sensor watches a shared service's signal stream before
      rolling up on completeness. Absent → runtime default.
    pattern: "^[0-9]+(ns|us|µs|ms|s|m|h)$"
```

Run: `go test ./schemas/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add schemas/examples/sensor/core-run-dev.yaml schemas/sensor.yaml
git commit -m "feat(schemas): run-dev becomes an observational service; add observe_window"
```

### Task 12: End-to-end regression test for #33

**Files:**
- Test: `examples/shared_service_test.go` (new; mirror the style of `examples/integration_test.go`)

- [ ] **Step 1: Write the failing integration test**

```go
package examples

import (
	"context"
	"testing"

	// import the same harness wiring helpers integration_test.go uses
)

// TestSharedRunDev_SingleInstance_NoLockCollision reproduces issue #33:
// a use case with a run-dev observational service and a logs sensor that
// attaches via `uses: run-dev`. Expectation: run-dev starts exactly once,
// the logs sensor produces signals from run-dev's stream, and the use-case
// verdict is NOT the crash-induced inconclusive.
func TestSharedRunDev_SingleInstance_NoLockCollision(t *testing.T) {
	// Arrange: build a temp .harness tree with:
	//   - stack-manifest.yaml (archetype web/next)
	//   - a use case uc-x
	//   - sensors/run-dev.yaml  (kind: observational, scope: core; boot step
	//     echoes a "ready - started server" line then a few "GET /x 200" lines
	//     to a script that stays alive until stopped)
	//   - sensors/logs.yaml (use-case, kind: observational, angle: logs,
	//     step: {uses: run-dev}, signal_matches: [{key: served, pattern: "GET /",
	//     expected: true}])
	// Use a fake "server" command (a shell loop) so the test needs no Next.js.
	root := buildSharedServiceFixture(t) // helper writing the YAML tree

	// Act
	verdict := runValidateUseCaseForTest(t, context.Background(), root, "uc-x")

	// Assert
	if verdict.Verdict == enumsVerdictInconclusive {
		t.Fatalf("verdict inconclusive — service collision regression (#33)")
	}
	if startCount := countServiceStarts(t, root, "run-dev"); startCount != 1 {
		t.Fatalf("run-dev started %d times, want exactly 1", startCount)
	}
}
```

> Use the existing test harness entry points from `examples/integration_test.go` for `runValidateUseCaseForTest` / fixture building. If those helpers are unexported in another package, add a thin exported test seam in `cmd/harness` or run the CLI `validate` path in-process as `integration_test.go` already does. `buildSharedServiceFixture` and `countServiceStarts` (counts `run-dev` run dirs under `.harness/runtime/run-dev/`) are new helpers in this test file.

- [ ] **Step 2: Run test to verify it fails (or errors) before the fix is fully wired**

Run: `go test ./examples/ -run TestSharedRunDev_SingleInstance_NoLockCollision -v`
Expected: FAIL initially (e.g. two run-dev run dirs, or inconclusive) — proving the test exercises the bug. Once Phases 1-5 are in place it should PASS.

- [ ] **Step 3: Make it pass**

Iterate on wiring until the test passes: exactly one `run-dev` run dir, logs sensor emits a `served` signal, verdict not inconclusive.

Run: `go test ./examples/ -run TestSharedRunDev_SingleInstance_NoLockCollision -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add examples/shared_service_test.go
git commit -m "test(examples): regression for #33 shared run-dev single instance"
```

### Task 13: Full suite + race detector

- [ ] **Step 1: Run the whole suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 2: Run the concurrency-sensitive packages under -race**

Run: `go test -race ./internal/runtime/servicemgr/ ./internal/runtime/executor/ ./cmd/harness/ ./internal/lifecycle/`
Expected: PASS, no data races.

- [ ] **Step 3: Commit any fixes**

```bash
git add -A
git commit -m "test: full suite + race pass for shared-service attach"
```

---

## Phase 6 — Follow-on (separate review; do after Phases 1-5 land)

These deliver the spec's "follow-on" scope so freshly generated sensors use the attach shape. Keep as a distinct review because they touch inferential skill scripts.

### Task 14: Generators emit run-dev as observational and logs as attach-shaped

**Files:**
- Modify: `skills/create-core-sensors/scripts/main.go` (emit `run-dev` with `kind: observational`, `output_type: stream`, `ready` + `log-line` matchers)
- Modify: `skills/create-sensors/scripts/main.go` (emit `logs`-angle sensors as `uses: <service>` + `with:` patterns, no inline server boot)
- Test: the sibling `main_test.go` files already referenced run-dev — update expectations to the new shape.

- [ ] **Step 1: Update `create-core-sensors` test expectations to the observational shape**, run to fail, implement, run to pass, commit. (Mirror Task 11's YAML shape in the generator output.)

- [ ] **Step 2: Update `create-sensors` test for logs-angle attach output**, run to fail, implement, run to pass, commit.

> These tasks are intentionally lighter-grained: the generator code and its tests already exist (`skills/*/scripts/main_test.go` reference `run-dev`); the change is updating the emitted YAML shape and the golden expectations. Follow the existing test structure in those files.

### Task 15: Document observational/service handling in the validate-use-case skill

**Files:**
- Modify: `skills/validate-use-case/SKILL.md`

- [ ] **Step 1:** Add a short "Shared observational services" subsection: core+observational sensors (e.g. `run-dev`) start once, are reference-counted, and attaching sensors consume their signal stream. Keep the skill file ≤ 200 lines (project rule 4). Commit.

---

## Self-review notes

- **Spec coverage:** §2 model → Tasks 5-9; §3.1 servicemgr → Tasks 3-4, 9; §3.2 runner → Tasks 7-9; §3.3 executor attach → Tasks 5-6; §5 schema/run-dev → Task 11; §6 second-spawn guard → Task 10, readiness → Task 9 (`readyByObservation`) + Task 11 (`ready` key), window → Tasks 5-6; §7 testing → every task is TDD + Task 12 regression + Task 13 race; §9 follow-on → Phase 6.
- **Type consistency:** `servicemgr.Attachment{ServiceID, RunID, SignalsPath}`, `servicemgr.Started{RunID, SignalsPath}`, `Manager.Acquire/Release/Lookup`, `executor.Options.ServiceAttach func(ctx, string) (servicemgr.Attachment, bool)`, `sensor.Sensor.ObserveWindow string`, `sigstream.Decoded{ObservationKey, MatchedLine, Raw}` / `sigstream.Follow(ctx, path, poll, stop, cb)` are used identically across all tasks.
- **Open risk flagged for the implementer:** Task 9 Step 4 rebuilds `lifecycle.New` twice to break the executor↔manager construction cycle; if `lifecycle.Options` gains a setter or the executor seam is made lazy, simplify. The aggregator must tolerate services being absent from the scheduled set (Task 8 Step 5 note) — verify against `internal/runtime/aggregator/usecase`.
