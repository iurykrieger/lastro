package lifecycle

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	ossignal "os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/runtime/executor"
	"github.com/iurykrieger/lastro/internal/runtime/process"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/signal"
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
	RuntimeRoot string                // typically <repo>/.harness/runtime
	NewRunID    func() string         // optional; defaults to ULID
	Now         func() time.Time      // optional; defaults to time.Now
	GracePeriod time.Duration         // optional; defaults to 5s
	Version     string                // harness version recorded in Handles
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
		SensorLookup:  base.SensorLookup,
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
		SensorLookup:  l.opts.Executor.OptionsRef().SensorLookup,
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
	case <-doneCh:
		// The run goroutine finished before any step spawned: exec.Run
		// returned an error (e.g. missing use case, template/setup failure)
		// without ever invoking OnStepStart. Surface that real error rather
		// than the misleading spawn timeout below.
		if entry.err != nil {
			return nil, fmt.Errorf("lifecycle: sensor run ended before child spawn: %w", entry.err)
		}
		return nil, fmt.Errorf("lifecycle: sensor run ended before child spawn")
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

// GenerateRunID returns a fresh run id using the configured NewRunID
// function. Exposed so a detached-watcher launcher can mint the id in the
// parent process and pass it to the spawned watcher.
func (l *Lifecycle) GenerateRunID() string { return l.opts.NewRunID() }

// RunDirFor returns the canonical run directory for (sensorID, runID)
// under the configured RuntimeRoot.
func (l *Lifecycle) RunDirFor(sensorID, runID string) string {
	return runDirPath(l.opts.RuntimeRoot, sensorID, runID)
}

// FindRunning returns the registry entry for (sensorID, runID), if any.
func (l *Lifecycle) FindRunning(sensorID, runID string) (Handle, bool, error) {
	return l.registry.Find(sensorID, runID)
}

// RunWatcher executes an observational sensor synchronously in the CURRENT
// process and blocks until it completes. It is the body a detached watcher
// process (spawned by the /start-sensor skill) runs: it registers itself in
// running_sensors.json with its own PID/PGID, runs the sensor to completion
// writing raw.log / signals.jsonl / aggregate.json, de-registers on exit,
// and stops gracefully on SIGINT/SIGTERM so a cross-process StopSensor can
// terminate it. The watcher is expected to be spawned as its own process-
// group leader (PGID == PID), so SignalGroup(-PGID) reaches it.
func (l *Lifecycle) RunWatcher(ctx context.Context, sensorID, runID, runDir string, expectedObs []string) error {
	s, ok := l.opts.Sensors.Lookup(sensorID)
	if !ok {
		return ErrSensorNotFound
	}
	if s.Kind != enums.KindObservational {
		return ErrAssertionSensor
	}
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return fmt.Errorf("lifecycle: mkdir runDir: %w", err)
	}

	// Drop registry entries whose watcher process is no longer alive.
	_, _ = l.pruneDead()

	pid := os.Getpid()
	h := Handle{
		SensorID:             sensorID,
		RunID:                runID,
		RunDir:               runDir,
		PID:                  pid,
		PGID:                 pid, // spawned as its own process-group leader
		StartedAt:            l.opts.Now(),
		ExpectedObservations: expectedObs,
		HarnessPID:           pid,
		HarnessVersion:       l.opts.Version,
		GOOS:                 runtime.GOOS,
	}
	if err := l.registry.Append(h); err != nil {
		return fmt.Errorf("lifecycle: registry append: %w", err)
	}
	defer func() { _ = l.registry.Remove(sensorID, runID) }()

	// Translate an external signal (cross-process StopSensor) or ctx
	// cancellation into a stop channel the executor honors. The executor
	// then tears down the running step's process group itself.
	stopCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	ossignal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer ossignal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
		case <-ctx.Done():
		}
		close(stopCh)
	}()

	agg, runErr := l.opts.Executor.Run(ctx, s, runDir, expectedObs, stopCh)
	if werr := writeAggregateJSON(filepath.Join(runDir, "aggregate.json"), agg); werr != nil && runErr == nil {
		runErr = werr
	}
	return runErr
}

// StopSensor terminates the sensor identified by h. In-process fast
// path: closes the stop channel and waits for the run goroutine. Cross-
// process path: signals the recorded PID via process.GroupSignaler.
func (l *Lifecycle) StopSensor(ctx context.Context, h *Handle) (aggregate.AggregateSignal, error) {
	if h == nil {
		return aggregate.AggregateSignal{}, fmt.Errorf("lifecycle: nil Handle")
	}
	key := runKey{SensorID: h.SensorID, RunID: h.RunID}

	// Fast path: same process owns the run.
	l.mu.Lock()
	entry, inflight := l.inflight[key]
	l.mu.Unlock()
	if inflight {
		select {
		case <-entry.stopCh: // already closed
		default:
			close(entry.stopCh)
		}
		select {
		case <-entry.doneCh:
		case <-ctx.Done():
			return aggregate.AggregateSignal{}, ctx.Err()
		}
		if entry.err != nil {
			return aggregate.AggregateSignal{}, entry.err
		}
		return entry.agg, nil
	}

	// Cross-process path: locate via registry.
	regEntry, ok, err := l.registry.Find(h.SensorID, h.RunID)
	if err != nil {
		return aggregate.AggregateSignal{}, err
	}
	if !ok {
		// Already terminated: try to read aggregate.json.
		if agg, ok := readAggregateJSON(filepath.Join(h.RunDir, "aggregate.json")); ok {
			return agg, nil
		}
		return aggregate.AggregateSignal{}, ErrSensorNotFound
	}

	// Liveness + start-time check.
	if !l.opts.Signaler.IsAlive(regEntry.PID, regEntry.StartedAt) {
		_ = l.registry.Remove(h.SensorID, h.RunID)
		return aggregate.AggregateSignal{}, ErrSensorOrphaned
	}

	// SIGTERM the group.
	if err := l.opts.Signaler.SignalGroup(regEntry.PID, regEntry.PGID, process.SignalTerm); err != nil {
		return aggregate.AggregateSignal{}, fmt.Errorf("lifecycle: SignalGroup SIGTERM: %w", err)
	}

	// Poll for aggregate.json up to gracePeriod, then SIGKILL.
	aggPath := filepath.Join(h.RunDir, "aggregate.json")
	deadline := time.Now().Add(l.opts.GracePeriod)
	for time.Now().Before(deadline) {
		if agg, ok := readAggregateJSON(aggPath); ok {
			return agg, nil
		}
		select {
		case <-ctx.Done():
			return aggregate.AggregateSignal{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	_ = l.opts.Signaler.SignalGroup(regEntry.PID, regEntry.PGID, process.SignalKill)

	// Wait a little more for aggregate.json after KILL.
	hardDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(hardDeadline) {
		if agg, ok := readAggregateJSON(aggPath); ok {
			return agg, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Last-resort orphan recovery: synthesize aggregate from signals.jsonl.
	return l.synthesizeOrphanAggregate(h, regEntry)
}

// synthesizeOrphanAggregate is the recovery path when the host process
// of a watcher died before writing aggregate.json. It reads any decoded
// signals from <runDir>/signals.jsonl and runs aggregate.Rollup with
// termination_reason=stopped.
func (l *Lifecycle) synthesizeOrphanAggregate(h *Handle, entry Handle) (aggregate.AggregateSignal, error) {
	sigs, err := readSignalsJSONL(filepath.Join(h.RunDir, "signals.jsonl"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return aggregate.AggregateSignal{}, err
	}
	s, ok := l.opts.Sensors.Lookup(h.SensorID)
	if !ok {
		return aggregate.AggregateSignal{}, ErrSensorNotFound
	}
	agg, err := aggregate.Rollup(aggregate.RollupInput{
		Signals:              sigs,
		SensorID:             s.ID,
		UseCaseID:            s.UseCaseID,
		Angle:                s.Angle,
		Kind:                 s.Kind,
		OutputType:           s.OutputType,
		StartedAt:            entry.StartedAt,
		EndedAt:              l.opts.Now(),
		TerminationReason:    enums.TerminationStopped,
		ExpectedObservations: h.ExpectedObservations,
		ObservedKeys:         observationKeysFromSignals(sigs),
	})
	if err != nil {
		return aggregate.AggregateSignal{}, err
	}
	_ = writeAggregateJSON(filepath.Join(h.RunDir, "aggregate.json"), agg)
	_ = l.registry.Remove(h.SensorID, h.RunID)
	return agg, nil
}

// ListRunning returns a snapshot of all in-flight registry entries.
func (l *Lifecycle) ListRunning() ([]Handle, error) {
	return l.registry.List()
}

// LoadHandle reconstructs a Handle from the registry for cross-process
// callers (e.g., `harness stop-sensor`).
func (l *Lifecycle) LoadHandle(sensorID, runID string) (*Handle, error) {
	h, ok, err := l.registry.Find(sensorID, runID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrSensorNotFound
	}
	return &h, nil
}

func readAggregateJSON(path string) (aggregate.AggregateSignal, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return aggregate.AggregateSignal{}, false
	}
	var agg aggregate.AggregateSignal
	if err := jsonDecode(b, &agg); err != nil {
		return aggregate.AggregateSignal{}, false
	}
	return agg, true
}

func readSignalsJSONL(path string) ([]aggregate.Signal, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []aggregate.Signal
	for _, line := range splitLines(b) {
		if len(line) == 0 {
			continue
		}
		sig, err := signal.DecodeLine(line)
		if err != nil {
			continue
		}
		out = append(out, aggregate.Signal{
			SchemaVersion: sig.SchemaVersion, SensorID: sig.SensorID, UseCaseID: sig.UseCaseID,
			Angle: sig.Angle, EmittedAt: sig.EmittedAt, Verdict: sig.Verdict, Confidence: sig.Confidence,
			Evidence: aggregate.Evidence(sig.Evidence), HealHint: aggregate.ConvertHealHint(sig.HealHint),
		})
	}
	return out, nil
}

func observationKeysFromSignals(sigs []aggregate.Signal) []string {
	var out []string
	for _, s := range sigs {
		if k, ok := s.Evidence["observation_key"].(string); ok && k != "" {
			out = append(out, k)
		}
	}
	return out
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			out = append(out, b[start:i])
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
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
