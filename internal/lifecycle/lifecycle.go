package lifecycle

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/runtime/executor"
	"github.com/iurykrieger/lastro/internal/runtime/process"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/oklog/ulid/v2"
)

// SensorStore is the seam Lifecycle binds against for sensor lookup.
// The concrete *sensor.Store satisfies it via its LookupSensor method.
// Tests may use a stub.
type SensorStore interface {
	// Lookup returns the sensor with the given id and ok=true if found.
	Lookup(id string) (sensor.Sensor, bool)
}

// sensorStoreAdapter wraps *sensor.Store to satisfy SensorStore.
// sensor.Store's method is LookupSensor; this adapter renames it to Lookup.
type sensorStoreAdapter struct{ s *sensor.Store }

func (a sensorStoreAdapter) Lookup(id string) (sensor.Sensor, bool) {
	return a.s.LookupSensor(id)
}

// WrapSensorStore adapts a *sensor.Store to the SensorStore interface
// consumed by Lifecycle.Options.Sensors.
func WrapSensorStore(s *sensor.Store) SensorStore { return sensorStoreAdapter{s: s} }

// Options wires a Lifecycle. All fields are read-only after New.
type Options struct {
	Sensors     SensorStore
	Executor    *executor.Executor
	RuntimeRoot string               // typically <repo>/.harness/runtime
	NewRunID    func() string        // optional; defaults to ULID
	Now         func() time.Time     // optional; defaults to time.Now
	GracePeriod time.Duration        // optional; defaults to 5s
	Version     string               // harness version recorded in Handles
	Signaler    process.GroupSignaler // optional; defaults to process.Default()
}

// Lifecycle is the single entry point for starting, stopping, and running
// sensors. Construct once; RunSensor may be called concurrently.
type Lifecycle struct {
	opts     Options
	registry *registry

	mu       sync.Mutex
	inflight map[runKey]*runEntry
}

type runKey struct{ SensorID, RunID string }

type runEntry struct {
	handle *Handle
	stopCh chan struct{}
	doneCh chan struct{}
	agg    aggregate.AggregateSignal
	err    error
}

// New creates a Lifecycle wired with the given options. Nil optional fields
// are filled with safe defaults.
func New(opts Options) *Lifecycle {
	if opts.NewRunID == nil {
		opts.NewRunID = defaultRunID
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.GracePeriod == 0 {
		opts.GracePeriod = 5 * time.Second
	}
	if opts.Signaler == nil {
		opts.Signaler = process.Default()
	}
	return &Lifecycle{
		opts:     opts,
		registry: newRegistry(opts.RuntimeRoot),
		inflight: map[runKey]*runEntry{},
	}
}

func defaultRunID() string {
	ms := ulid.Timestamp(time.Now())
	id, _ := ulid.New(ms, rand.Reader)
	return id.String()
}

// RunSensor synchronously runs the sensor identified by sensorID. It is safe
// to call concurrently. For assertion sensors pass nil expectedObs; for
// observational sensors pass the expected observation keys.
func (l *Lifecycle) RunSensor(
	ctx context.Context,
	sensorID string,
	expectedObs []string,
) (aggregate.AggregateSignal, error) {
	s, ok := l.opts.Sensors.Lookup(sensorID)
	if !ok {
		return aggregate.AggregateSignal{}, ErrSensorNotFound
	}

	runID := l.opts.NewRunID()
	runDir := runDirPath(l.opts.RuntimeRoot, sensorID, runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return aggregate.AggregateSignal{}, fmt.Errorf("lifecycle: mkdir runDir: %w", err)
	}

	// Prune stale registry entries before adding ours.
	_, _ = l.pruneDead()

	key := runKey{SensorID: sensorID, RunID: runID}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	entry := &runEntry{
		stopCh: stopCh,
		doneCh: doneCh,
	}

	l.mu.Lock()
	l.inflight[key] = entry
	l.mu.Unlock()

	defer func() {
		l.mu.Lock()
		delete(l.inflight, key)
		l.mu.Unlock()
		_ = l.registry.Remove(sensorID, runID)
		close(doneCh)
	}()

	registered := false
	onStart := func(stepIdx, pid, pgid int) {
		h := &Handle{
			SensorID:             sensorID,
			RunID:                runID,
			RunDir:               runDir,
			PID:                  pid,
			PGID:                 pgid,
			StartedAt:            l.opts.Now(),
			ExpectedObservations: expectedObs,
			HarnessPID:           os.Getpid(),
			HarnessVersion:       l.opts.Version,
			GOOS:                 runtime.GOOS,
		}
		entry.handle = h
		if !registered {
			_ = l.registry.Append(*h)
			registered = true
		} else {
			_ = l.registry.UpdatePID(sensorID, runID, pid, pgid)
		}
	}

	// Build a per-run executor that shares all dependencies from the parent
	// executor but installs a fresh OnStepStart hook for this run.
	base := l.opts.Executor.OptionsRef()
	exec := executor.New(executor.Options{
		RepoRoot:      base.RepoRoot,
		Resolver:      base.Resolver,
		FixtureStore:  base.FixtureStore,
		UseCaseLookup: base.UseCaseLookup,
		Now:           base.Now,
		Shell:         base.Shell,
		GroupSignaler: l.opts.Signaler,
		OnStepStart:   onStart,
	})

	agg, err := exec.Run(ctx, s, runDir, expectedObs, stopCh)
	if err != nil {
		return aggregate.AggregateSignal{}, err
	}

	if encErr := writeAggregateJSON(filepath.Join(runDir, "aggregate.json"), agg); encErr != nil {
		// Non-fatal: the caller gets the aggregate regardless.
		_ = encErr
	}
	return agg, nil
}

// StartSensor spawns an observational sensor and returns a Handle
// immediately. Returns ErrAssertionSensor if called on a kind:assertion
// sensor. The watcher subprocess is detached from ctx (only StopSensor
// or an OS signal can terminate it) so the caller process is free to
// exit.
func (l *Lifecycle) StartSensor(
	ctx context.Context,
	sensorID string,
	expectedObs []string,
) (*Handle, error) {
	s, ok := l.opts.Sensors.Lookup(sensorID)
	if !ok {
		return nil, ErrSensorNotFound
	}
	if s.Kind != enums.KindObservational {
		return nil, ErrAssertionSensor
	}

	runID := l.opts.NewRunID()
	runDir := runDirPath(l.opts.RuntimeRoot, sensorID, runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return nil, fmt.Errorf("lifecycle: mkdir runDir: %w", err)
	}

	_, _ = l.pruneDead()

	key := runKey{SensorID: sensorID, RunID: runID}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	entry := &runEntry{stopCh: stopCh, doneCh: doneCh}

	l.mu.Lock()
	l.inflight[key] = entry
	l.mu.Unlock()

	// Buffered to avoid blocking the executor's hook goroutine.
	startedCh := make(chan struct{}, 1)
	startErrCh := make(chan error, 1)

	registered := false
	onStart := func(stepIdx, pid, pgid int) {
		h := &Handle{
			SensorID: sensorID, RunID: runID, RunDir: runDir,
			PID: pid, PGID: pgid, StartedAt: l.opts.Now(),
			ExpectedObservations: expectedObs,
			HarnessPID:           os.Getpid(),
			HarnessVersion:       l.opts.Version,
			GOOS:                 runtime.GOOS,
		}
		entry.handle = h
		if !registered {
			if err := l.registry.Append(*h); err != nil {
				select {
				case startErrCh <- err:
				default:
				}
				return
			}
			registered = true
			select {
			case startedCh <- struct{}{}:
			default:
			}
		} else {
			_ = l.registry.UpdatePID(sensorID, runID, pid, pgid)
		}
	}

	// Detached context: the spawning caller's ctx must NOT cancel the
	// watcher. Inherit only Values, not cancellation.
	detached := context.WithoutCancel(ctx)

	exec := executor.New(executor.Options{
		RepoRoot:      l.opts.Executor.OptionsRef().RepoRoot,
		Resolver:      l.opts.Executor.OptionsRef().Resolver,
		FixtureStore:  l.opts.Executor.OptionsRef().FixtureStore,
		UseCaseLookup: l.opts.Executor.OptionsRef().UseCaseLookup,
		Now:           l.opts.Executor.OptionsRef().Now,
		Shell:         l.opts.Executor.OptionsRef().Shell,
		GroupSignaler: l.opts.Signaler,
		OnStepStart:   onStart,
	})

	go func() {
		defer close(doneCh)
		defer func() {
			l.mu.Lock()
			delete(l.inflight, key)
			l.mu.Unlock()
			_ = l.registry.Remove(sensorID, runID)
		}()

		agg, err := exec.Run(detached, s, runDir, expectedObs, stopCh)
		entry.agg = agg
		entry.err = err
		_ = writeAggregateJSON(filepath.Join(runDir, "aggregate.json"), agg)
	}()

	// Wait until either the first step has spawned (registry entry
	// written) or an early error fires.
	select {
	case <-startedCh:
	case err := <-startErrCh:
		// Best effort: signal the watcher to terminate.
		close(stopCh)
		<-doneCh
		return nil, err
	case <-time.After(10 * time.Second):
		close(stopCh)
		<-doneCh
		return nil, fmt.Errorf("lifecycle: StartSensor timed out waiting for child spawn")
	case <-ctx.Done():
		close(stopCh)
		<-doneCh
		return nil, ctx.Err()
	}

	hCopy := *entry.handle
	return &hCopy, nil
}

// StopSensor is a temporary stub; Task 17 replaces this with the full
// cross-process implementation. The stub closes the stop channel and waits
// for the run goroutine to exit, then returns whatever aggregate the
// goroutine produced.
func (l *Lifecycle) StopSensor(ctx context.Context, h *Handle) (aggregate.AggregateSignal, error) {
	if h == nil {
		return aggregate.AggregateSignal{}, fmt.Errorf("lifecycle: nil Handle")
	}
	l.mu.Lock()
	entry, ok := l.inflight[runKey{SensorID: h.SensorID, RunID: h.RunID}]
	l.mu.Unlock()
	if !ok {
		return aggregate.AggregateSignal{}, ErrSensorNotFound
	}
	select {
	case <-entry.stopCh:
	default:
		close(entry.stopCh)
	}
	select {
	case <-entry.doneCh:
	case <-ctx.Done():
		return aggregate.AggregateSignal{}, ctx.Err()
	}
	return entry.agg, entry.err
}

// pruneDead removes registry entries whose PIDs are no longer alive.
func (l *Lifecycle) pruneDead() (int, error) {
	return l.registry.Prune(func(h Handle) bool {
		return l.opts.Signaler.IsAlive(h.PID, h.StartedAt)
	})
}

// writeAggregateJSON serializes the aggregate atomically (temp + rename).
func writeAggregateJSON(path string, agg aggregate.AggregateSignal) error {
	tmp := path + ".tmp"
	data, err := jsonEncode(agg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
