// Package process abstracts process-group creation, signaling, and
// liveness checks behind a small interface so internal/runtime/executor
// and internal/lifecycle can stay GOOS-agnostic.
//
// The POSIX implementation uses Setpgid + kill(-pgid, sig); the Windows
// implementation uses CREATE_NEW_PROCESS_GROUP + GenerateConsoleCtrlEvent
// with a TerminateProcess fallback. See process_posix.go and
// process_windows.go.
package process

import (
	"os/exec"
	"time"
)

// Signal is a portable signal enum. Mapping to OS-native signals lives in
// the per-GOOS implementations because Windows has no Unix signals.
type Signal int

const (
	// SignalTerm is the graceful-termination signal: SIGTERM on POSIX,
	// CTRL_BREAK_EVENT to the console process group on Windows.
	SignalTerm Signal = iota
	// SignalKill is the hard-kill signal: SIGKILL on POSIX,
	// TerminateProcess on Windows.
	SignalKill
)

// GroupSignaler is the interface both executor and lifecycle consume. The
// production implementation is returned by Default(); tests can provide
// a stub.
type GroupSignaler interface {
	// Spawn mutates cmd.SysProcAttr so cmd.Start() places the child in
	// its own process group (POSIX) or console process group (Windows).
	// Spawn does not call Start itself.
	Spawn(cmd *exec.Cmd) error

	// GroupID returns the process group id of an already-Started cmd.
	// On Windows the returned value is the Pid (CTRL_BREAK_EVENT is
	// dispatched against the Pid that is the group root).
	GroupID(cmd *exec.Cmd) (int, error)

	// SignalGroup sends sig to the entire process group rooted at
	// (pid, pgid). Either argument may be ignored depending on platform.
	SignalGroup(pid, pgid int, sig Signal) error

	// IsAlive returns true if pid is alive AND its start time matches
	// startedAt within tolerance. The start-time check is the PID-
	// recycling defense; on platforms where start time isn't cheaply
	// available, IsAlive reports liveness only (best effort).
	IsAlive(pid int, startedAt time.Time) bool
}

// Default returns the GOOS-appropriate GroupSignaler. The concrete value
// is built by per-GOOS files via the defaultSignaler() helper.
func Default() GroupSignaler { return defaultSignaler() }
