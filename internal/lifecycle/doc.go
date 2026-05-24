// Package lifecycle resolves sensor IDs, persists in-flight watchers to
// a central registry, and exposes the Run/Start/Stop entry points that
// B5 (skill wrappers) and B6 (CLI) call. It is the only public surface
// for sensor execution outside the runtime package.
//
// Deps anchored here for future registry and run-ID generation:
//   - github.com/rogpeppe/go-internal/lockedfile  — advisory file locking for running_sensors.json
//   - github.com/oklog/ulid/v2                    — monotonic ULID generation for RunID
package lifecycle

import (
	_ "github.com/oklog/ulid/v2"
	_ "github.com/rogpeppe/go-internal/lockedfile"
)
