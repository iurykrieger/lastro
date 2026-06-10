// Shared-service orchestration for the validate path (issue #34).
//
// A `kind: observational` + `scope: core` sensor (e.g. run-dev) is a shared
// service: it must not be scheduled as a run-to-completion DAG node (a dev
// server never exits, so the scheduler would block forever and the server
// would leak). Instead its lifetime is reference-counted by
// servicemgr.Manager — started on the first consumer's Acquire, stopped on
// the last Release — mirroring cmd/harness's use-case runner.
package skillruntime

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/lifecycle"
	"github.com/iurykrieger/lastro/internal/runtime/servicemgr"
	"github.com/iurykrieger/lastro/internal/runtime/sigstream"
	"github.com/iurykrieger/lastro/internal/sensor"
)

// serviceReadyTimeout bounds how long a consumer blocks waiting for a
// freshly started service to emit its "ready" observation.
const serviceReadyTimeout = 60 * time.Second

// serviceHolder is the settable indirection between the executor (built
// once at boot) and the validate run's service manager (built per run).
// Outside a validate run Lookup always misses, so uses-steps inline-expand
// exactly as before.
type serviceHolder struct {
	mu  sync.RWMutex
	mgr *servicemgr.Manager
}

func (h *serviceHolder) set(m *servicemgr.Manager) {
	h.mu.Lock()
	h.mgr = m
	h.mu.Unlock()
}

func (h *serviceHolder) clear() { h.set(nil) }

// Lookup satisfies executor.Options.ServiceAttach.
func (h *serviceHolder) Lookup(_ context.Context, serviceID string) (servicemgr.Attachment, bool) {
	h.mu.RLock()
	mgr := h.mgr
	h.mu.RUnlock()
	if mgr == nil {
		return servicemgr.Attachment{}, false
	}
	return mgr.Lookup(serviceID)
}

// lifecycleService adapts *lifecycle.Lifecycle to servicemgr.ServiceLifecycle.
type lifecycleService struct {
	lc      *lifecycle.Lifecycle
	mu      sync.Mutex
	handles map[string]*lifecycle.Handle // serviceID -> running handle
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

// classifyServices partitions gathered sensors into shared services (core +
// observational) and regular scheduled sensors.
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

// serviceDepsOf maps each regular sensor id to the service ids it attaches
// to (via a depends_on edge or a step `uses:` that names a known service).
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

// stripServiceEdges returns copies of sensors with every depends_on edge
// that points at a shared service removed. Service ordering is handled by
// the ref-counted Acquire/Release around each consumer's run; leaving the
// edge in would make the scheduler reject the consumer as having a dangling
// dependency (the service is not a scheduled node).
func stripServiceEdges(sensors []sensor.Sensor, isService map[string]bool) []sensor.Sensor {
	out := make([]sensor.Sensor, len(sensors))
	for i, s := range sensors {
		out[i] = s
		if len(s.DependsOn) == 0 {
			continue
		}
		filtered := make([]string, 0, len(s.DependsOn))
		for _, dep := range s.DependsOn {
			if !isService[dep] {
				filtered = append(filtered, dep)
			}
		}
		out[i].DependsOn = filtered
	}
	return out
}

// RunUseCaseSensors runs every gathered sensor for a use case, managing
// shared observational core services (run-dev and friends) out-of-band:
// services are excluded from the scheduled DAG, started on the first
// consumer's Acquire (blocking until ready), fed to attaching consumers as
// live signal streams, and torn down when the last consumer detaches — with
// a deferred Shutdown sweep guaranteeing teardown on error or cancellation.
//
// It returns the per-sensor aggregates plus the scheduled (non-service)
// sensor set, which is what per-use-case aggregation must grade.
func RunUseCaseSensors(
	ctx context.Context,
	b *Booted,
	gathered []sensor.Sensor,
	parallelism int,
) ([]aggregate.AggregateSignal, []sensor.Sensor, error) {
	if parallelism < 1 {
		parallelism = runtime.NumCPU()
	}

	plain := func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		return b.Lifecycle.RunSensor(ctx, s.ID, nil)
	}

	services, regular := classifyServices(gathered)
	if len(services) == 0 {
		aggs, err := RunAll(ctx, gathered, plain, parallelism)
		return aggs, gathered, err
	}

	isService := make(map[string]bool, len(services))
	for _, s := range services {
		isService[s.ID] = true
	}
	deps := serviceDepsOf(regular, isService)
	scheduled := stripServiceEdges(regular, isService)

	mgr := servicemgr.New(
		&lifecycleService{lc: b.Lifecycle, handles: map[string]*lifecycle.Handle{}},
		servicemgr.Options{Ready: readyByObservation, ReadyTimeout: serviceReadyTimeout},
	)
	b.services.set(mgr)
	defer b.services.clear()
	// Finally sweep: no service may outlive the run, whatever path exits it.
	defer func() { _ = mgr.Shutdown(context.WithoutCancel(ctx)) }()

	runner := func(ctx context.Context, s sensor.Sensor) (aggregate.AggregateSignal, error) {
		acquired := make([]string, 0, len(deps[s.ID]))
		defer func() {
			// Detach with an uncancellable context so the last consumer's
			// Release still stops the service after ctx is cancelled.
			for _, id := range acquired {
				_ = mgr.Release(context.WithoutCancel(ctx), id)
			}
		}()
		for _, svc := range deps[s.ID] {
			if _, err := mgr.Acquire(ctx, svc, nil); err != nil {
				return inconclusiveFromServiceError(s, svc, err), nil
			}
			acquired = append(acquired, svc)
		}
		return b.Lifecycle.RunSensor(ctx, s.ID, nil)
	}

	aggs, err := RunAll(ctx, scheduled, runner, parallelism)
	return aggs, scheduled, err
}

// inconclusiveFromServiceError synthesizes an inconclusive AggregateSignal
// for a consumer whose shared service failed to start or become ready,
// matching the spec's "sensor execution errors bubble up as inconclusive".
func inconclusiveFromServiceError(s sensor.Sensor, serviceID string, err error) aggregate.AggregateSignal {
	now := time.Now().UTC()
	return aggregate.AggregateSignal{
		SchemaVersion:     "1.0.0",
		Type:              aggregate.TypeAggregate,
		SensorID:          s.ID,
		UseCaseID:         s.UseCaseID,
		Angle:             s.Angle,
		StartedAt:         now,
		EndedAt:           now,
		Verdict:           enums.VerdictInconclusive,
		Confidence:        0,
		TerminationReason: enums.TerminationError,
		Rollup: aggregate.RollupCounts{
			InconclusiveCount: 1,
		},
		HealHint: &aggregate.HealHint{
			Summary:   fmt.Sprintf("shared service %s failed to start: %v", serviceID, err),
			Rationale: fmt.Sprintf("sensor %s attaches to %s; the service did not become ready, so the sensor did not run", s.ID, serviceID),
		},
	}
}
