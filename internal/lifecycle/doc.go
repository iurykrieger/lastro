// Package lifecycle resolves sensor IDs, persists in-flight watchers to
// a central registry, and exposes the Run/Start/Stop entry points that
// B5 (skill wrappers) and B6 (CLI) call. It is the only public surface
// for sensor execution outside the runtime package.
package lifecycle
