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
