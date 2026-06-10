// Package lifecycle resolves sensor IDs, persists in-flight watchers to
// a central registry, and exposes the Run/Start/Stop entry points that
// B5 (skill wrappers) and B6 (CLI) call. It is the only public surface
// for sensor execution outside the runtime package.
package lifecycle

import "errors"

var (
	ErrSensorNotFound  = errors.New("lifecycle: sensor id not in store")
	ErrAssertionSensor = errors.New("lifecycle: StartSensor called on kind:assertion sensor")
	ErrSensorOrphaned  = errors.New("lifecycle: registry entry's PID is dead")
	ErrSensorReplaced  = errors.New("lifecycle: PID is alive but started_at disagrees (PID recycled)")
	ErrRegistryBusy    = errors.New("lifecycle: could not acquire registry lock within timeout")

	// ErrServiceAlreadyRunning is returned by StartSensor when a live registry
	// entry already exists for the same sensor id. Prevents a second
	// host-exclusive service (e.g. run-dev) from racing the first on a shared
	// resource such as .next/dev/lock.
	ErrServiceAlreadyRunning = errors.New("lifecycle: a live instance of this sensor is already running")
)
