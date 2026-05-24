# B2 — Executor & Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement B2 — the executor + lifecycle runtime spine — per [the B2 spec](../specs/2026-05-24-b2-executor-lifecycle-design.md).

**Architecture:** Three new packages under `internal/runtime/` (`process`, `executor`) and `internal/lifecycle/`. Plus one five-line public-helper promotion in `internal/signal`. Executor owns mechanism (spawn/stream/rollup); Lifecycle owns registry (ids, sidecars, cross-process stop). Cross-platform process-group handling lives in a leaf `process` package both consume.

**Tech Stack:** Go 1.24+, two new third-party dependencies:
- `github.com/rogpeppe/go-internal/lockedfile` — file locking for the central registry
- `github.com/oklog/ulid/v2` — run-id generation
- `golang.org/x/sys` — already a transitive dep; pull explicitly for Windows process-group APIs

**Spec reference shorthand:** §N below refers to sections of [`2026-05-24-b2-executor-lifecycle-design.md`](../specs/2026-05-24-b2-executor-lifecycle-design.md).

**Branching:** Work on `feat/b2-executor-lifecycle` branched from `origin/main`. The B2 chunk doc mandates this branch; do not stack on `feat/b1-*`.

---

## File map

**Modify (Phase 1 — `signal.DecodeLine` promotion):**
- `internal/signal/parser.go` — rename `decodeAndValidateLine` to public `DecodeLine` and update the one internal caller.
- `internal/signal/parser_test.go` — add a focused unit test for `DecodeLine` (table-driven: valid pass, valid fail-with-heal-hint, bad JSON, schema-violation).

**Create (Phase 2 — `internal/runtime/process/`):**
- `internal/runtime/process/process.go` — `GroupSignaler` interface, `Signal` enum, `Default()` factory.
- `internal/runtime/process/process_posix.go` — POSIX impl (`//go:build !windows`).
- `internal/runtime/process/process_windows.go` — Windows impl (`//go:build windows`).
- `internal/runtime/process/process_posix_test.go` — POSIX-only behavioral tests.
- `internal/runtime/process/process_windows_test.go` — Windows-only behavioral tests.

**Create (Phase 3 — `internal/runtime/executor/` + shared test fake):**
- `internal/runtime/executor/errors.go` — typed errors.
- `internal/runtime/executor/rawlog.go` — line-annotated, mutex-serialized writer.
- `internal/runtime/executor/rawlog_test.go` — race-detector test.
- `internal/runtime/executor/command.go` — shell-argv per GOOS.
- `internal/runtime/executor/command_test.go` — argv construction matrix.
- `internal/runtime/executor/signals.go` — stdout pump + observation-key extraction.
- `internal/runtime/executor/signals_test.go` — pump behavior.
- `internal/testutil/fakesensor/main.go` — shared CLI fake used by executor + lifecycle tests.
- `internal/runtime/executor/step.go` — single-step exec.
- `internal/runtime/executor/step_test.go` — step-level behavior.
- `internal/runtime/executor/crash.go` — `synthesizeCrashHint`.
- `internal/runtime/executor/crash_test.go` — hint synthesis cases.
- `internal/runtime/executor/executor.go` — `Executor`, `Options`, `Run`.
- `internal/runtime/executor/run_test.go` — end-to-end + cancellation/timeout.
- `internal/runtime/executor/testdata/golden/assertion_three_step.json` — golden aggregate.

**Create (Phase 4 — `internal/lifecycle/`):**
- `internal/lifecycle/errors.go` — typed errors.
- `internal/lifecycle/handle.go` — `Handle` struct + JSON.
- `internal/lifecycle/handle_test.go` — JSON round-trip.
- `internal/lifecycle/runtime_dir.go` — path helpers.
- `internal/lifecycle/runtime_dir_test.go` — path tests.
- `internal/lifecycle/registry.go` — `running_sensors.json` CRUD under `lockedfile`.
- `internal/lifecycle/registry_test.go` — concurrent writers, stale-pruning, lock contention.
- `internal/lifecycle/lifecycle.go` — `Lifecycle`, `RunSensor`, `StartSensor`, `StopSensor`, `ListRunning`, `LoadHandle`.
- `internal/lifecycle/lifecycle_test.go` — Run/Start/Stop end-to-end including subtest-fork cross-process round-trip.
- `internal/lifecycle/testdata/sensors/` — minimal sensor YAMLs the tests load.

**Modify (final polish):**
- `go.mod`, `go.sum` — pinned versions of new deps.

---

# Phase 1 — `signal.DecodeLine` promotion

## Task 1: Promote `decodeAndValidateLine` to public `signal.DecodeLine`

**Files:**
- Modify: `internal/signal/parser.go`
- Modify: `internal/signal/parser_test.go`

- [ ] **Step 1: Branch from main**

```bash
git fetch origin
git checkout -b feat/b2-executor-lifecycle origin/main
```

- [ ] **Step 2: Write the failing test**

Append to `internal/signal/parser_test.go`:

```go
func TestDecodeLine_Valid(t *testing.T) {
	line := []byte(`{"schema_version":"1.0.0","sensor_id":"s","use_case_id":"uc","angle":"build","emitted_at":"2026-05-24T10:00:00Z","verdict":"pass","confidence":1,"evidence":{}}`)
	sig, err := DecodeLine(line)
	if err != nil {
		t.Fatalf("DecodeLine: %v", err)
	}
	if sig.SensorID != "s" {
		t.Errorf("SensorID = %q, want %q", sig.SensorID, "s")
	}
	if sig.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want %q", sig.Verdict, enums.VerdictPass)
	}
}

func TestDecodeLine_BadJSON(t *testing.T) {
	_, err := DecodeLine([]byte(`{not json`))
	if err == nil {
		t.Fatalf("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode line") {
		t.Errorf("error = %v, want substring 'decode line'", err)
	}
}

func TestDecodeLine_SchemaViolation(t *testing.T) {
	// Missing required `verdict`.
	line := []byte(`{"schema_version":"1.0.0","sensor_id":"s","use_case_id":"uc","angle":"build","emitted_at":"2026-05-24T10:00:00Z","confidence":1,"evidence":{}}`)
	_, err := DecodeLine(line)
	if err == nil {
		t.Fatalf("expected schema error, got nil")
	}
	if !strings.Contains(err.Error(), "schema") {
		t.Errorf("error = %v, want substring 'schema'", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/signal/ -run TestDecodeLine -v
```

Expected: FAIL with `undefined: DecodeLine`.

- [ ] **Step 4: Promote the helper**

In `internal/signal/parser.go`, rename the function and update its single caller (`ParseSignals`):

```go
// DecodeLine runs the per-line three-phase pipeline used by ParseSignals
// for one JSON Lines record: JSON decode → schema validation → typed
// decode. Returns the zero Signal and a wrapped error on any phase
// failure. Exposed so streaming consumers (e.g., the executor) can
// interleave decoding with their own per-line bookkeeping.
func DecodeLine(line []byte) (Signal, error) {
	var instance any
	if err := json.Unmarshal(line, &instance); err != nil {
		return Signal{}, fmt.Errorf("signal: decode line: %w", err)
	}
	s, err := compiledSchema()
	if err != nil {
		return Signal{}, err
	}
	if err := s.Validate(instance); err != nil {
		return Signal{}, fmt.Errorf("signal: schema: %w", err)
	}
	var sig Signal
	if err := json.Unmarshal(line, &sig); err != nil {
		return Signal{}, fmt.Errorf("signal: decode typed: %w", err)
	}
	return sig, nil
}
```

Inside `ParseSignals`, replace `decodeAndValidateLine(line)` with `DecodeLine(line)`. Delete the old private function entirely.

- [ ] **Step 5: Run tests to verify all pass**

```bash
go test ./internal/signal/ -v -race
```

Expected: all tests pass (existing `ParseSignals` tests continue to work; new `DecodeLine` tests pass).

- [ ] **Step 6: Commit**

```bash
git add internal/signal/parser.go internal/signal/parser_test.go
git commit -m "$(cat <<'EOF'
feat(signal): expose DecodeLine for line-by-line streaming consumers

Promotes the existing private decodeAndValidateLine helper so B2's
executor can interleave per-line decoding with its raw.log/signals.jsonl
writes. No behavior change for ParseSignals.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

# Phase 2 — `internal/runtime/process` (GroupSignaler abstraction)

## Task 2: Create the process package interface and Signal enum

**Files:**
- Create: `internal/runtime/process/process.go`

- [ ] **Step 1: Create the directory**

```bash
mkdir -p internal/runtime/process
```

- [ ] **Step 2: Write `process.go`**

```go
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
```

- [ ] **Step 3: Verify it builds (no test yet)**

```bash
go build ./internal/runtime/process/
```

Expected: FAIL with `undefined: defaultSignaler`. (Per-GOOS files come next.)

---

## Task 3: POSIX implementation

**Files:**
- Create: `internal/runtime/process/process_posix.go`
- Create: `internal/runtime/process/process_posix_test.go`

- [ ] **Step 1: Write `process_posix.go`**

```go
//go:build !windows

package process

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type posixSignaler struct{}

func defaultSignaler() GroupSignaler { return posixSignaler{} }

func (posixSignaler) Spawn(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	return nil
}

func (posixSignaler) GroupID(cmd *exec.Cmd) (int, error) {
	if cmd.Process == nil {
		return 0, fmt.Errorf("process: GroupID called before Start")
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return 0, fmt.Errorf("process: Getpgid(%d): %w", cmd.Process.Pid, err)
	}
	return pgid, nil
}

func (posixSignaler) SignalGroup(pid, pgid int, sig Signal) error {
	var ss syscall.Signal
	switch sig {
	case SignalTerm:
		ss = syscall.SIGTERM
	case SignalKill:
		ss = syscall.SIGKILL
	default:
		return fmt.Errorf("process: unknown Signal %d", sig)
	}
	if pgid > 0 {
		return syscall.Kill(-pgid, ss)
	}
	return syscall.Kill(pid, ss)
}

func (posixSignaler) IsAlive(pid int, startedAt time.Time) bool {
	if pid <= 0 {
		return false
	}
	// kill(pid, 0) tests existence without actually signaling.
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	if runtime.GOOS != "linux" {
		// Best-effort: no cheap start-time read outside Linux.
		return true
	}
	return procStartTimeMatches(pid, startedAt)
}

// procStartTimeMatches reads /proc/<pid>/stat field 22 (starttime in
// clock ticks since boot) and compares it against startedAt within a
// 2-second tolerance.
func procStartTimeMatches(pid int, startedAt time.Time) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	s := string(data)
	rparen := strings.LastIndex(s, ")")
	if rparen < 0 {
		return false
	}
	fields := strings.Fields(s[rparen+1:])
	// After the closing ')', fields are: state, ppid, pgrp, session,
	// tty_nr, tpgid, flags, minflt, cminflt, majflt, cmajflt, utime,
	// stime, cutime, cstime, priority, nice, num_threads, itrealvalue,
	// starttime[19].
	if len(fields) < 20 {
		return false
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return false
	}
	boot, err := readBootTime()
	if err != nil {
		return false
	}
	const clockTicksPerSec = 100 // sysconf(_SC_CLK_TCK) on Linux is 100 in practice.
	got := boot.Add(time.Duration(ticks) * time.Second / clockTicksPerSec)
	diff := got.Sub(startedAt)
	if diff < 0 {
		diff = -diff
	}
	return diff < 2*time.Second
}

func readBootTime() (time.Time, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "btime ") {
			continue
		}
		sec, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "btime ")), 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(sec, 0), nil
	}
	return time.Time{}, fmt.Errorf("process: btime not found in /proc/stat")
}
```

- [ ] **Step 2: Write `process_posix_test.go`**

```go
//go:build !windows

package process

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestPOSIX_SpawnSetsPgidFlag(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "true")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	attr, ok := cmd.SysProcAttr.(*syscall.SysProcAttr) // ignored if direct ptr
	_ = ok
	if cmd.SysProcAttr == nil {
		t.Fatalf("SysProcAttr is nil after Spawn")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Errorf("Setpgid = false, want true")
		_ = attr
	}
}

func TestPOSIX_GroupIDBeforeStart(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "true")
	_, err := defaultSignaler().GroupID(cmd)
	if err == nil {
		t.Errorf("expected error when calling GroupID before Start")
	}
}

func TestPOSIX_GroupIDAfterStart(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 0.2")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })

	pgid, err := defaultSignaler().GroupID(cmd)
	if err != nil {
		t.Fatalf("GroupID: %v", err)
	}
	if pgid != cmd.Process.Pid {
		t.Errorf("pgid = %d, want %d (child should be group leader)", pgid, cmd.Process.Pid)
	}
}

func TestPOSIX_SignalGroupReachesGroup(t *testing.T) {
	// Spawn a shell that backgrounds a `sleep` child. SignalGroup must
	// terminate BOTH (the shell and the sleep), proving the signal
	// reached the whole group.
	cmd := exec.Command("/bin/sh", "-c", "sleep 5 & echo $! ; wait")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pgid, _ := defaultSignaler().GroupID(cmd)

	time.Sleep(50 * time.Millisecond) // let the shell fork the sleep

	if err := defaultSignaler().SignalGroup(cmd.Process.Pid, pgid, SignalTerm); err != nil {
		t.Fatalf("SignalGroup: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = defaultSignaler().SignalGroup(cmd.Process.Pid, pgid, SignalKill)
		<-done
		t.Fatalf("process group did not exit within 2s of SIGTERM")
	}
}

func TestPOSIX_IsAliveTrueForLiveProcess(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 0.5")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })

	startedAt := time.Now()
	if !defaultSignaler().IsAlive(cmd.Process.Pid, startedAt) {
		t.Errorf("IsAlive = false for live PID %d", cmd.Process.Pid)
	}
}

func TestPOSIX_IsAliveFalseForDeadPID(t *testing.T) {
	// PID 2^30 is guaranteed not allocated.
	if defaultSignaler().IsAlive(1<<30, time.Now()) {
		t.Errorf("IsAlive returned true for non-existent PID")
	}
	if defaultSignaler().IsAlive(0, time.Now()) {
		t.Errorf("IsAlive returned true for PID 0")
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/runtime/process/ -v -race
```

Expected: all pass on POSIX hosts (Linux/WSL/macOS). The "process group reaches group" test depends on Setpgid working — that's the smoke test.

- [ ] **Step 4: Verify the file compiles on Windows (cross-compile sanity)**

```bash
GOOS=windows go build ./internal/runtime/process/
```

Expected: FAIL with `undefined: defaultSignaler` because we haven't written `process_windows.go` yet. (That's the next task.) Do not commit yet.

---

## Task 4: Windows implementation

**Files:**
- Create: `internal/runtime/process/process_windows.go`
- Create: `internal/runtime/process/process_windows_test.go`

- [ ] **Step 1: Add `golang.org/x/sys` to go.mod**

```bash
go get golang.org/x/sys/windows@latest
go mod tidy
```

Expected: `go.mod` now lists `golang.org/x/sys` as a direct dependency.

- [ ] **Step 2: Write `process_windows.go`**

```go
//go:build windows

package process

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

type windowsSignaler struct{}

func defaultSignaler() GroupSignaler { return windowsSignaler{} }

func (windowsSignaler) Spawn(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
	return nil
}

func (windowsSignaler) GroupID(cmd *exec.Cmd) (int, error) {
	if cmd.Process == nil {
		return 0, fmt.Errorf("process: GroupID called before Start")
	}
	// CTRL_BREAK_EVENT is dispatched against the Pid that is the new
	// console group root; there is no separate pgid on Windows.
	return cmd.Process.Pid, nil
}

func (windowsSignaler) SignalGroup(pid, pgid int, sig Signal) error {
	switch sig {
	case SignalTerm:
		// CTRL_BREAK_EVENT propagates to the entire console process
		// group rooted at pid. CTRL_C_EVENT is intercepted by the new
		// group on creation, so Ctrl-Break is the standard choice.
		if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid)); err != nil {
			return fmt.Errorf("process: GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, %d): %w", pid, err)
		}
		return nil
	case SignalKill:
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
		if err != nil {
			return fmt.Errorf("process: OpenProcess(PROCESS_TERMINATE, %d): %w", pid, err)
		}
		defer windows.CloseHandle(h)
		if err := windows.TerminateProcess(h, 1); err != nil {
			return fmt.Errorf("process: TerminateProcess(%d): %w", pid, err)
		}
		return nil
	default:
		return fmt.Errorf("process: unknown Signal %d", sig)
	}
}

func (windowsSignaler) IsAlive(pid int, startedAt time.Time) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	// Check exit code: STILL_ACTIVE (259) means running.
	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	const STILL_ACTIVE = 259
	if exitCode != STILL_ACTIVE {
		return false
	}

	// PID-recycling defense: compare creation time.
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return false
	}
	got := time.Unix(0, creation.Nanoseconds())
	diff := got.Sub(startedAt)
	if diff < 0 {
		diff = -diff
	}
	return diff < 2*time.Second
}
```

- [ ] **Step 3: Write `process_windows_test.go`**

```go
//go:build windows

package process

import (
	"os/exec"
	"testing"
	"time"
)

func TestWindows_SpawnSetsCreationFlag(t *testing.T) {
	cmd := exec.Command("cmd", "/C", "exit /B 0")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if cmd.SysProcAttr == nil {
		t.Fatalf("SysProcAttr is nil after Spawn")
	}
	if cmd.SysProcAttr.CreationFlags&0x00000200 == 0 { // CREATE_NEW_PROCESS_GROUP
		t.Errorf("CreationFlags missing CREATE_NEW_PROCESS_GROUP: %x", cmd.SysProcAttr.CreationFlags)
	}
}

func TestWindows_GroupIDReturnsPid(t *testing.T) {
	cmd := exec.Command("cmd", "/C", "timeout /T 1 /NOBREAK")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })

	pgid, err := defaultSignaler().GroupID(cmd)
	if err != nil {
		t.Fatalf("GroupID: %v", err)
	}
	if pgid != cmd.Process.Pid {
		t.Errorf("pgid = %d, want %d", pgid, cmd.Process.Pid)
	}
}

func TestWindows_SignalGroupTerminatesProcess(t *testing.T) {
	// Use SignalKill (TerminateProcess) for a deterministic test —
	// CTRL_BREAK_EVENT requires a console-attached child that handles
	// the signal, which a pure cmd.exe invocation may not.
	cmd := exec.Command("cmd", "/C", "ping -n 30 127.0.0.1 > NUL")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)
	if err := defaultSignaler().SignalGroup(cmd.Process.Pid, cmd.Process.Pid, SignalKill); err != nil {
		t.Fatalf("SignalGroup: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("process did not exit within 2s of SignalKill")
	}
}

func TestWindows_IsAliveTrueForLiveProcess(t *testing.T) {
	cmd := exec.Command("cmd", "/C", "ping -n 2 127.0.0.1 > NUL")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })

	if !defaultSignaler().IsAlive(cmd.Process.Pid, time.Now()) {
		t.Errorf("IsAlive = false for live PID %d", cmd.Process.Pid)
	}
}

func TestWindows_IsAliveFalseForDeadPID(t *testing.T) {
	if defaultSignaler().IsAlive(1<<30, time.Now()) {
		t.Errorf("IsAlive returned true for non-existent PID")
	}
	if defaultSignaler().IsAlive(0, time.Now()) {
		t.Errorf("IsAlive returned true for PID 0")
	}
}
```

- [ ] **Step 4: Verify Windows build cross-compiles**

```bash
GOOS=windows go build ./internal/runtime/process/
```

Expected: SUCCESS, exit 0. (We can't run the Windows tests from a Linux host; that's CI's job. Cross-compile is the verification gate here.)

- [ ] **Step 5: Run POSIX tests once more**

```bash
go test ./internal/runtime/process/ -v -race
```

Expected: all POSIX tests still pass.

- [ ] **Step 6: Commit Phase 2**

```bash
git add internal/runtime/process/ go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(runtime/process): cross-platform GroupSignaler abstraction

Adds process-group spawn (Setpgid / CREATE_NEW_PROCESS_GROUP), graceful
+ hard signaling (SIGTERM/SIGKILL / CTRL_BREAK/TerminateProcess), and
PID-recycling-aware IsAlive (proc/<pid>/stat on Linux, GetProcessTimes
on Windows). Consumed by B2 executor + lifecycle.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

# Phase 3 — `internal/runtime/executor`

## Task 5: Errors and the raw-log writer

**Files:**
- Create: `internal/runtime/executor/errors.go`
- Create: `internal/runtime/executor/rawlog.go`
- Create: `internal/runtime/executor/rawlog_test.go`

- [ ] **Step 1: Create the directory**

```bash
mkdir -p internal/runtime/executor
```

- [ ] **Step 2: Write `errors.go`**

```go
// Package executor runs a single Sensor end-to-end. It owns the per-
// step process lifecycle, stdout/stderr capture, signal decoding, and
// the call to aggregate.Rollup that produces the terminal AggregateSignal.
// It knows nothing about sensor IDs, sidecars, or .harness/ layout —
// those are internal/lifecycle's concern.
package executor

import (
	"errors"
	"fmt"
)

// ErrTemplateFixtureInRun is returned when a step's `run:` string
// contains a `{{fixtures.X}}` reference. Fixtures must reach steps
// via env vars from fixturebinder, not via shell-string interpolation,
// to avoid shell-injection from arbitrary fixture content.
var ErrTemplateFixtureInRun = errors.New("executor: {{fixtures.X}} not allowed in step.run; use env vars")

// ErrStepCrashed is returned when a step's process exits non-zero and
// has emitted zero Signals. Multi-step sensors short-circuit on this.
var ErrStepCrashed = errors.New("executor: step exited non-zero without emitting signals")

// TemplateError wraps a template parse or resolve failure with the
// owning step index.
type TemplateError struct {
	Step  int
	Cause error
}

func (e *TemplateError) Error() string {
	return fmt.Sprintf("executor: template error at step %d: %v", e.Step, e.Cause)
}

func (e *TemplateError) Unwrap() error { return e.Cause }

// SpawnError wraps an exec.Cmd.Start failure with the owning step index.
type SpawnError struct {
	Step  int
	Cause error
}

func (e *SpawnError) Error() string {
	return fmt.Sprintf("executor: spawn failed at step %d: %v", e.Step, e.Cause)
}

func (e *SpawnError) Unwrap() error { return e.Cause }
```

- [ ] **Step 3: Write the failing test for `rawLog`**

Create `internal/runtime/executor/rawlog_test.go`:

```go
package executor

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRawLog_WriteAnnotated_Format(t *testing.T) {
	dir := t.TempDir()
	rl, err := newRawLog(filepath.Join(dir, "raw.log"), fixedNow(t))
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Close()

	rl.WriteAnnotated(1, "stdout", []byte(`{"ok":true}`))
	rl.WriteAnnotated(1, "stderr", []byte("oops"))
	rl.WriteAnnotated(2, "parse-error", []byte("bad json"))

	rl.Close()
	data, err := os.ReadFile(filepath.Join(dir, "raw.log"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		"[2026-05-24T10:00:00.000000000Z step-01 stdout] {\"ok\":true}",
		"[2026-05-24T10:00:00.000000000Z step-01 stderr] oops",
		"[2026-05-24T10:00:00.000000000Z step-02 parse-error] bad json",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("raw.log missing line %q\nfull output:\n%s", want, got)
		}
	}
}

func TestRawLog_ConcurrentWritersDoNotTear(t *testing.T) {
	dir := t.TempDir()
	rl, err := newRawLog(filepath.Join(dir, "raw.log"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Close()

	const writers = 8
	const linesPerWriter = 200
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			payload := strings.Repeat("x", 200)
			for i := 0; i < linesPerWriter; i++ {
				rl.WriteAnnotated(id+1, "stdout", []byte(payload))
			}
		}(w)
	}
	wg.Wait()
	rl.Close()

	data, _ := os.ReadFile(filepath.Join(dir, "raw.log"))
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if got, want := len(lines), writers*linesPerWriter; got != want {
		t.Errorf("line count = %d, want %d", got, want)
	}
	// No line should be torn — every line starts with "[".
	for i, line := range lines {
		if !strings.HasPrefix(line, "[") {
			t.Fatalf("line %d does not start with '[': %q", i, line)
		}
	}
}

// fixedNow returns a Now() that always returns 2026-05-24T10:00:00.000000000Z.
func fixedNow(t *testing.T) func() time.Time {
	t.Helper()
	ts, _ := time.Parse(time.RFC3339Nano, "2026-05-24T10:00:00.000000000Z")
	return func() time.Time { return ts }
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
go test ./internal/runtime/executor/ -run TestRawLog -v
```

Expected: FAIL with `undefined: newRawLog`.

- [ ] **Step 5: Write `rawlog.go`**

```go
package executor

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"
)

// rawLog is a line-annotated, mutex-serialized writer for the per-run
// raw.log file. Multiple goroutines (stdout reader, stderr reader) write
// concurrently; the mutex guarantees no line is torn.
type rawLog struct {
	mu  sync.Mutex
	f   *os.File
	w   *bufio.Writer
	now func() time.Time
}

func newRawLog(path string, now func() time.Time) (*rawLog, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("rawlog: open %s: %w", path, err)
	}
	if now == nil {
		now = time.Now
	}
	return &rawLog{
		f:   f,
		w:   bufio.NewWriter(f),
		now: now,
	}, nil
}

// WriteAnnotated writes one annotated line of the form
//
//	[<RFC3339Nano timestamp> step-NN <stream>] <content>\n
//
// stepIdx is 1-based and zero-padded to two digits. stream is one of
// "stdout", "stderr", "parse-error", "exit-nonzero".
func (r *rawLog) WriteAnnotated(stepIdx int, stream string, content []byte) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ts := r.now().UTC().Format(time.RFC3339Nano)
	// Pad to 9 fractional digits for stable golden tests; Go's RFC3339Nano
	// trims trailing zeros, so pad explicitly.
	ts = padNanos(ts)
	fmt.Fprintf(r.w, "[%s step-%02d %s] %s\n", ts, stepIdx, stream, content)
}

// Close flushes the buffer and closes the file. Safe to call multiple times.
func (r *rawLog) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.w.Flush()
	if cerr := r.f.Close(); err == nil {
		err = cerr
	}
	r.f = nil
	return err
}

// padNanos ensures the fractional-second portion of an RFC3339Nano
// timestamp is exactly 9 digits, so golden test outputs are byte-stable.
func padNanos(ts string) string {
	// Find the dot.
	dot := -1
	for i := 0; i < len(ts); i++ {
		if ts[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		// No fractional part — insert ".000000000" before the timezone.
		// Find the timezone suffix start (Z or +/-).
		tz := len(ts)
		for i := len(ts) - 1; i >= 0; i-- {
			if ts[i] == 'Z' || ts[i] == '+' || ts[i] == '-' {
				tz = i
				break
			}
		}
		return ts[:tz] + ".000000000" + ts[tz:]
	}
	// Already has a dot; count fractional digits.
	tz := len(ts)
	for i := dot + 1; i < len(ts); i++ {
		if ts[i] == 'Z' || ts[i] == '+' || (ts[i] == '-' && i > dot+3) {
			tz = i
			break
		}
	}
	fracLen := tz - dot - 1
	if fracLen >= 9 {
		return ts
	}
	pad := ""
	for i := fracLen; i < 9; i++ {
		pad += "0"
	}
	return ts[:tz] + pad + ts[tz:]
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/runtime/executor/ -run TestRawLog -v -race
```

Expected: both `TestRawLog_*` tests PASS, including under `-race`.

- [ ] **Step 7: Commit**

```bash
git add internal/runtime/executor/errors.go internal/runtime/executor/rawlog.go internal/runtime/executor/rawlog_test.go
git commit -m "feat(runtime/executor): add typed errors and mutex-serialized rawLog writer

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 6: Shell-argv builder

**Files:**
- Create: `internal/runtime/executor/command.go`
- Create: `internal/runtime/executor/command_test.go`

- [ ] **Step 1: Write the failing test**

```go
package executor

import (
	"runtime"
	"testing"
)

func TestShellArgv_POSIXDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only test")
	}
	argv := shellArgv(nil, "echo hi")
	if len(argv) != 3 || argv[0] != "/bin/sh" || argv[1] != "-c" || argv[2] != "echo hi" {
		t.Errorf("argv = %v, want [/bin/sh -c echo hi]", argv)
	}
}

func TestShellArgv_WindowsDefault(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	argv := shellArgv(nil, "echo hi")
	if len(argv) != 3 || argv[0] != "cmd" || argv[1] != "/C" || argv[2] != "echo hi" {
		t.Errorf("argv = %v, want [cmd /C echo hi]", argv)
	}
}

func TestShellArgv_Override(t *testing.T) {
	argv := shellArgv([]string{"bash", "-eu", "-c"}, "ls -la")
	want := []string{"bash", "-eu", "-c", "ls -la"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/runtime/executor/ -run TestShellArgv -v
```

Expected: FAIL with `undefined: shellArgv`.

- [ ] **Step 3: Write `command.go`**

```go
package executor

import "runtime"

// shellArgv returns the argv slice used to spawn a step's command via
// exec.Command. If override is non-empty, it is used verbatim with the
// resolved command appended as the last argument. Otherwise the GOOS
// default applies: /bin/sh -c on POSIX, cmd /C on Windows.
//
// /bin/sh (not /bin/bash) is the POSIX choice because some images
// (Alpine) ship only /bin/sh. Sensors that need bash should call it
// explicitly in their run: string.
func shellArgv(override []string, resolvedCmd string) []string {
	if len(override) > 0 {
		out := make([]string, 0, len(override)+1)
		out = append(out, override...)
		out = append(out, resolvedCmd)
		return out
	}
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/C", resolvedCmd}
	}
	return []string{"/bin/sh", "-c", resolvedCmd}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/runtime/executor/ -run TestShellArgv -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/executor/command.go internal/runtime/executor/command_test.go
git commit -m "feat(runtime/executor): GOOS-aware shell-argv builder

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 7: Stdout signal pump

**Files:**
- Create: `internal/runtime/executor/signals.go`
- Create: `internal/runtime/executor/signals_test.go`

- [ ] **Step 1: Write the failing test**

```go
package executor

import (
	"strings"
	"testing"
)

func TestPumpStdout_HappyPath(t *testing.T) {
	const passLine = `{"schema_version":"1.0.0","sensor_id":"s","use_case_id":"u","angle":"build","emitted_at":"2026-05-24T10:00:00Z","verdict":"pass","confidence":1,"evidence":{}}`
	const obsLine = `{"schema_version":"1.0.0","sensor_id":"s","use_case_id":"u","angle":"logs","emitted_at":"2026-05-24T10:00:00Z","verdict":"pass","confidence":1,"evidence":{"observation_key":"order-received"}}`

	stdout := strings.NewReader(passLine + "\n\n" + obsLine + "\n")
	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	signalsJSONL, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer signalsJSONL.Close()

	out, err := pumpStdout(stdout, 1, rl, signalsJSONL, true /*observational*/)
	if err != nil {
		t.Fatalf("pumpStdout: %v", err)
	}
	if got, want := len(out.Signals), 2; got != want {
		t.Errorf("len(Signals) = %d, want %d", got, want)
	}
	if got, want := out.ObservationKeys, []string{"order-received"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("ObservationKeys = %v, want %v", got, want)
	}
}

func TestPumpStdout_BadJSONLineKeepsStreaming(t *testing.T) {
	const passLine = `{"schema_version":"1.0.0","sensor_id":"s","use_case_id":"u","angle":"build","emitted_at":"2026-05-24T10:00:00Z","verdict":"pass","confidence":1,"evidence":{}}`
	stdout := strings.NewReader("not json\n" + passLine + "\n")

	dir := t.TempDir()
	rl, _ := newRawLog(dir+"/raw.log", fixedNow(t))
	defer rl.Close()
	jw, _ := newJSONLWriter(dir + "/signals.jsonl")
	defer jw.Close()

	out, err := pumpStdout(stdout, 1, rl, jw, false)
	if err != nil {
		t.Fatalf("pumpStdout: %v", err)
	}
	if got, want := len(out.Signals), 1; got != want {
		t.Errorf("len(Signals) = %d, want %d", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/runtime/executor/ -run TestPumpStdout -v
```

Expected: FAIL with `undefined: pumpStdout, newJSONLWriter`.

- [ ] **Step 3: Write `signals.go`**

```go
package executor

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/iurykrieger/lastro/internal/signal"
)

// maxStdoutLineBytes caps a single stdout line. Mirrors
// signal.maxSignalLineBytes (1 MiB).
const maxStdoutLineBytes = 1 << 20

// pumpOutput is the result of consuming a step's stdout: the successfully
// decoded signals (in arrival order) and the observation keys extracted
// from their evidence (observational sensors only; nil otherwise).
type pumpOutput struct {
	Signals         []signal.Signal
	ObservationKeys []string
}

// pumpStdout reads r line-by-line, writes each line to rl with stream
// "stdout", attempts to decode each non-empty line as a Signal, appends
// successful decodes to the returned slice, and tees the raw bytes to
// signalsJSONL. Decode failures are logged to rl with stream
// "parse-error" and skipped.
//
// If observational is true and a Signal's evidence carries the
// "observation_key" string, the value is appended to ObservationKeys.
//
// pumpStdout returns when r returns EOF or any scanner-level error. The
// scanner error (if any) is returned wrapped; a bare EOF returns nil.
func pumpStdout(r io.Reader, stepIdx int, rl *rawLog, signalsJSONL *jsonlWriter, observational bool) (pumpOutput, error) {
	out := pumpOutput{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxStdoutLineBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		// Copy because scanner reuses the underlying buffer.
		lineCopy := append([]byte(nil), line...)
		rl.WriteAnnotated(stepIdx, "stdout", lineCopy)

		trimmed := bytes.TrimSpace(lineCopy)
		if len(trimmed) == 0 {
			continue
		}
		sig, err := signal.DecodeLine(trimmed)
		if err != nil {
			rl.WriteAnnotated(stepIdx, "parse-error", []byte(err.Error()))
			continue
		}
		out.Signals = append(out.Signals, sig)
		_ = signalsJSONL.WriteLine(trimmed)
		if observational {
			if k, ok := sig.Evidence["observation_key"].(string); ok && k != "" {
				out.ObservationKeys = append(out.ObservationKeys, k)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("signals: scan stdout: %w", err)
	}
	return out, nil
}

// pumpStderr reads r line-by-line and writes each line to rl with stream
// "stderr". Returns when r returns EOF.
func pumpStderr(r io.Reader, stepIdx int, rl *rawLog) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxStdoutLineBytes)
	for scanner.Scan() {
		rl.WriteAnnotated(stepIdx, "stderr", append([]byte(nil), scanner.Bytes()...))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("signals: scan stderr: %w", err)
	}
	return nil
}

// jsonlWriter appends raw JSON lines (no annotation) to signals.jsonl.
// Not goroutine-safe; the executor uses it only from the stdout pump.
type jsonlWriter struct {
	f *os.File
	w *bufio.Writer
}

func newJSONLWriter(path string) (*jsonlWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("jsonl: open %s: %w", path, err)
	}
	return &jsonlWriter{f: f, w: bufio.NewWriter(f)}, nil
}

func (j *jsonlWriter) WriteLine(b []byte) error {
	if _, err := j.w.Write(b); err != nil {
		return err
	}
	if _, err := j.w.Write([]byte{'\n'}); err != nil {
		return err
	}
	return nil
}

func (j *jsonlWriter) Close() error {
	if j == nil || j.f == nil {
		return nil
	}
	err := j.w.Flush()
	if cerr := j.f.Close(); err == nil {
		err = cerr
	}
	j.f = nil
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/runtime/executor/ -run TestPumpStdout -v -race
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/executor/signals.go internal/runtime/executor/signals_test.go
git commit -m "feat(runtime/executor): stdout signal pump + jsonl writer

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 8: Fake sensor CLI shared by all integration tests

**Files:**
- Create: `internal/testutil/fakesensor/main.go`
- Create: `internal/testutil/fakesensor/doc.go`

- [ ] **Step 1: Create the directory**

```bash
mkdir -p internal/testutil/fakesensor
```

- [ ] **Step 2: Write `doc.go`**

```go
// Package fakesensor is the source of a cross-platform Go binary used as
// a stand-in for real sensor commands in executor + lifecycle tests.
// Test packages compile this binary from TestMain into a temp directory
// and invoke it via the sensor's run: field.
//
// Subcommands:
//
//	signal pass    [--observation-key K]   Emit one pass Signal.
//	signal fail    [--summary S]           Emit one fail Signal + heal hint.
//	stream N       [--interval D]          Emit N pass Signals, optional sleep.
//	crash          [--exit-code C] [--stderr S]
//	                                       Print S to stderr, exit C.
//	watch          [--emit K]... [--interval D]
//	                                       Emit one Signal per --emit value,
//	                                       then loop until SIGTERM / SIGKILL.
//	sleep D                                Sleep D, emit nothing.
package fakesensor
```

- [ ] **Step 3: Write `main.go`**

```go
//go:build ignore
// +build ignore

// fakesensor is a test helper compiled by executor/lifecycle test
// packages via TestMain. The build tag prevents it from being picked up
// during normal `go build ./...`.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type signalRec struct {
	SchemaVersion string         `json:"schema_version"`
	SensorID      string         `json:"sensor_id"`
	UseCaseID     string         `json:"use_case_id"`
	Angle         string         `json:"angle"`
	EmittedAt     string         `json:"emitted_at"`
	Verdict       string         `json:"verdict"`
	Confidence    float64        `json:"confidence"`
	Evidence      map[string]any `json:"evidence"`
	HealHint      *healHint      `json:"heal_hint,omitempty"`
}

type healHint struct {
	Summary   string `json:"summary"`
	Rationale string `json:"rationale"`
}

func emit(rec signalRec) {
	if rec.SchemaVersion == "" {
		rec.SchemaVersion = "1.0.0"
	}
	if rec.SensorID == "" {
		rec.SensorID = os.Getenv("HARNESS_SENSOR_ID")
		if rec.SensorID == "" {
			rec.SensorID = "fake"
		}
	}
	if rec.UseCaseID == "" {
		rec.UseCaseID = os.Getenv("HARNESS_USE_CASE_ID")
		if rec.UseCaseID == "" {
			rec.UseCaseID = "fake-uc"
		}
	}
	if rec.Angle == "" {
		rec.Angle = "build"
	}
	rec.EmittedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if rec.Confidence == 0 {
		rec.Confidence = 1.0
	}
	if rec.Evidence == nil {
		rec.Evidence = map[string]any{}
	}
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(rec)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fakesensor <subcommand> [args...]")
		os.Exit(2)
	}
	args := os.Args[1:]
	switch args[0] {
	case "signal":
		cmdSignal(args[1:])
	case "stream":
		cmdStream(args[1:])
	case "crash":
		cmdCrash(args[1:])
	case "watch":
		cmdWatch(args[1:])
	case "sleep":
		cmdSleep(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "fakesensor: unknown subcommand:", args[0])
		os.Exit(2)
	}
}

func cmdSignal(args []string) {
	if len(args) == 0 {
		os.Exit(2)
	}
	rec := signalRec{Verdict: args[0]}
	var summary, obsKey, angle string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--observation-key":
			i++
			if i < len(args) {
				obsKey = args[i]
			}
		case "--summary":
			i++
			if i < len(args) {
				summary = args[i]
			}
		case "--angle":
			i++
			if i < len(args) {
				angle = args[i]
			}
		}
	}
	if angle != "" {
		rec.Angle = angle
	}
	if obsKey != "" {
		rec.Evidence = map[string]any{"observation_key": obsKey}
	}
	if rec.Verdict == "fail" || rec.Verdict == "warn" {
		s := summary
		if s == "" {
			s = "fake failure"
		}
		rec.HealHint = &healHint{Summary: s, Rationale: "fakesensor-generated"}
	}
	emit(rec)
}

func cmdStream(args []string) {
	if len(args) == 0 {
		os.Exit(2)
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		os.Exit(2)
	}
	interval := time.Duration(0)
	for i := 1; i < len(args); i++ {
		if args[i] == "--interval" {
			i++
			if d, err := time.ParseDuration(args[i]); err == nil {
				interval = d
			}
		}
	}
	for i := 0; i < n; i++ {
		emit(signalRec{Verdict: "pass"})
		if interval > 0 {
			time.Sleep(interval)
		}
	}
}

func cmdCrash(args []string) {
	exitCode := 1
	msg := "fakesensor crashed"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--exit-code":
			i++
			if i < len(args) {
				if c, err := strconv.Atoi(args[i]); err == nil {
					exitCode = c
				}
			}
		case "--stderr":
			i++
			if i < len(args) {
				msg = args[i]
			}
		}
	}
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(exitCode)
}

func cmdWatch(args []string) {
	var emits []string
	interval := 50 * time.Millisecond
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--emit":
			i++
			if i < len(args) {
				emits = append(emits, args[i])
			}
		case "--interval":
			i++
			if i < len(args) {
				if d, err := time.ParseDuration(args[i]); err == nil {
					interval = d
				}
			}
		}
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	for _, k := range emits {
		select {
		case <-sigCh:
			return
		default:
		}
		emit(signalRec{Angle: "logs", Verdict: "pass", Evidence: map[string]any{"observation_key": k}})
		time.Sleep(interval)
	}
	// Then idle until signaled.
	<-sigCh
}

func cmdSleep(args []string) {
	if len(args) == 0 {
		os.Exit(2)
	}
	d, err := time.ParseDuration(args[0])
	if err != nil {
		os.Exit(2)
	}
	time.Sleep(d)
}
```

- [ ] **Step 4: Verify it compiles (the build tag will skip it from `go build ./...`)**

```bash
go build -o /tmp/fakesensor.bin ./internal/testutil/fakesensor/main.go
/tmp/fakesensor.bin signal pass
```

Expected: prints a single JSON Signal to stdout. Exits 0.

- [ ] **Step 5: Commit**

```bash
git add internal/testutil/fakesensor/
git commit -m "test: add fakesensor CLI for executor + lifecycle integration tests

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 9: Single-step execution

**Files:**
- Create: `internal/runtime/executor/step.go`
- Create: `internal/runtime/executor/step_test.go`

- [ ] **Step 1: Sketch the test (compile-time placeholder; full test is in Task 11 once executor.go exists)**

For now, write a single test in `step_test.go` that exercises template-fixture rejection:

```go
package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/runtime/process"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// stubStore is a minimal FixtureStore that returns nothing — enough to
// satisfy the binder when no fixtures are referenced.
type stubStore struct{}

func (stubStore) LookupFixture(id string) (fixture.Fixture, bool) { return fixture.Fixture{}, false }
func (stubStore) FixturesForUseCase(uc string) []fixture.Fixture  { return nil }

func TestRunStep_RejectsFixtureRefInRun(t *testing.T) {
	res := template.Resolver{Fixtures: stubStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}}
	uc := &usecase.UseCase{ID: "uc"}
	step := sensor.Step{ID: "s1", Run: "echo {{fixtures.foo}}"}
	dir := t.TempDir()

	_, err := runStep(context.Background(), stepArgs{
		Step:        step,
		StepIdx:     1,
		RunDir:      dir,
		UseCase:     uc,
		Store:       stubStore{},
		Resolver:    &res,
		Signaler:    process.Default(),
		Shell:       []string{"/bin/sh", "-c"},
		ExpectedObs: nil,
		RawLog:      mustRawLog(t, dir),
		SignalsW:    mustJSONL(t, dir),
		Stop:        nil,
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var te *TemplateError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TemplateError", err)
	}
	if !errors.Is(te.Cause, ErrTemplateFixtureInRun) {
		t.Errorf("inner err = %v, want ErrTemplateFixtureInRun", te.Cause)
	}
}

func mustRawLog(t *testing.T, dir string) *rawLog {
	t.Helper()
	rl, err := newRawLog(dir+"/raw.log", fixedNow(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rl.Close() })
	return rl
}

func mustJSONL(t *testing.T, dir string) *jsonlWriter {
	t.Helper()
	jw, err := newJSONLWriter(dir + "/signals.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = jw.Close() })
	return jw
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/runtime/executor/ -run TestRunStep_Rejects -v
```

Expected: FAIL with `undefined: runStep, stepArgs`.

- [ ] **Step 3: Write `step.go`**

```go
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/runtime/fixturebinder"
	"github.com/iurykrieger/lastro/internal/runtime/process"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/signal"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// stepArgs is the per-step input handed to runStep by Run.
type stepArgs struct {
	Step        sensor.Step
	StepIdx     int
	RunDir      string
	UseCase     *usecase.UseCase
	Store       fixture.FixtureStore
	Resolver    *template.Resolver
	Signaler    process.GroupSignaler
	Shell       []string
	ExpectedObs []string // nil for assertion sensors
	RawLog      *rawLog
	SignalsW    *jsonlWriter
	Stop        <-chan struct{}
	OnStart     func(stepIdx, pid, pgid int)
}

// stepOutcome reports the per-step result.
type stepOutcome struct {
	Signals           []signal.Signal
	ObservationKeys   []string
	ExitErr           error // nil if process exited 0
	StoppedExternally bool  // closed via stop channel
	CtxErr            error // ctx.Err() at exit, if any
}

func runStep(ctx context.Context, a stepArgs) (stepOutcome, error) {
	// 1. Template-parse Step.Run; reject FixtureRef segments.
	segs, err := template.Parse(a.Step.Run)
	if err != nil {
		return stepOutcome{}, &TemplateError{Step: a.StepIdx, Cause: err}
	}
	for _, s := range segs {
		if _, ok := s.(template.FixtureRef); ok {
			return stepOutcome{}, &TemplateError{Step: a.StepIdx, Cause: ErrTemplateFixtureInRun}
		}
	}
	resolved, err := a.Resolver.Resolve(segs)
	if err != nil {
		return stepOutcome{}, &TemplateError{Step: a.StepIdx, Cause: err}
	}

	// 2. Bind fixtures via fixturebinder.
	binder := &fixturebinder.Binder{ScratchDir: filepath.Join(a.RunDir, "scratch")}
	if err := os.MkdirAll(binder.ScratchDir, 0o700); err != nil {
		return stepOutcome{}, fmt.Errorf("executor: mkdir scratch: %w", err)
	}
	binding, err := binder.Bind(a.Step, a.UseCase, a.Store)
	if err != nil {
		return stepOutcome{}, err
	}

	// 3. Build environment.
	env := os.Environ()
	for k, v := range binding.Env {
		env = append(env, k+"="+v)
	}
	env = append(env,
		"HARNESS_RUN_DIR="+a.RunDir,
		"HARNESS_SCRATCH_DIR="+binder.ScratchDir,
	)

	// 4. Build the command.
	argv := shellArgv(a.Shell, resolved)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = env
	if err := a.Signaler.Spawn(cmd); err != nil {
		return stepOutcome{}, &SpawnError{Step: a.StepIdx, Cause: err}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return stepOutcome{}, &SpawnError{Step: a.StepIdx, Cause: err}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return stepOutcome{}, &SpawnError{Step: a.StepIdx, Cause: err}
	}

	// 5. Spawn.
	if err := cmd.Start(); err != nil {
		return stepOutcome{}, &SpawnError{Step: a.StepIdx, Cause: err}
	}
	pgid, _ := a.Signaler.GroupID(cmd)
	if a.OnStart != nil {
		a.OnStart(a.StepIdx, cmd.Process.Pid, pgid)
	}

	// 6. Concurrent reader goroutines.
	type pumpResult struct {
		out pumpOutput
		err error
	}
	stdoutDone := make(chan pumpResult, 1)
	stderrDone := make(chan error, 1)
	go func() {
		out, err := pumpStdout(stdout, a.StepIdx, a.RawLog, a.SignalsW, a.ExpectedObs != nil)
		stdoutDone <- pumpResult{out: out, err: err}
	}()
	go func() {
		stderrDone <- pumpStderr(stderr, a.StepIdx, a.RawLog)
	}()

	// 7. Stop watcher: kill the process group on stop or ctx cancel.
	stoppedExternally := false
	watcherDone := make(chan struct{})
	exitCh := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-exitCh:
			return
		case <-a.Stop:
			stoppedExternally = true
		case <-ctx.Done():
		}
		_ = a.Signaler.SignalGroup(cmd.Process.Pid, pgid, process.SignalTerm)
		// Hard-kill after 2 seconds if still alive.
		t := time.NewTimer(2 * time.Second)
		defer t.Stop()
		select {
		case <-exitCh:
		case <-t.C:
			_ = a.Signaler.SignalGroup(cmd.Process.Pid, pgid, process.SignalKill)
			<-exitCh
		}
	}()

	// 8. Wait for the process to exit.
	waitErr := cmd.Wait()
	close(exitCh)
	<-watcherDone

	// 9. Drain the readers.
	stdoutRes := <-stdoutDone
	stderrErr := <-stderrDone
	_ = stderrErr // best-effort; raw.log already captured what made it through

	// 10. Note non-zero exit in raw.log for forensics.
	if waitErr != nil && len(stdoutRes.out.Signals) > 0 {
		a.RawLog.WriteAnnotated(a.StepIdx, "exit-nonzero", []byte(strconv.Quote(waitErr.Error())))
	}

	return stepOutcome{
		Signals:           stdoutRes.out.Signals,
		ObservationKeys:   stdoutRes.out.ObservationKeys,
		ExitErr:           waitErr,
		StoppedExternally: stoppedExternally,
		CtxErr:            ctx.Err(),
	}, errors.Join(stdoutRes.err) // stderr errors are non-fatal
}

// envBytes is only used in tests to verify env-var building is stable.
func envBytes(env []string) []byte {
	var b bytes.Buffer
	for _, e := range env {
		b.WriteString(e)
		b.WriteByte('\n')
	}
	return b.Bytes()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/runtime/executor/ -run TestRunStep_Rejects -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/executor/step.go internal/runtime/executor/step_test.go
git commit -m "feat(runtime/executor): single-step exec with template + binder integration

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 10: Crash-hint synthesis

**Files:**
- Create: `internal/runtime/executor/crash.go`
- Create: `internal/runtime/executor/crash_test.go`

- [ ] **Step 1: Write the failing test**

```go
package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSynthesizeCrashHint_IncludesStderrTail(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.log")
	content := strings.Join([]string{
		"[2026-05-24T10:00:00.000000000Z step-01 stdout] noise",
		"[2026-05-24T10:00:00.000000000Z step-01 stderr] could not connect to redis",
		"[2026-05-24T10:00:00.000000000Z step-01 stderr] giving up",
	}, "\n") + "\n"
	if err := os.WriteFile(raw, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	hint := synthesizeCrashHint(raw, &SpawnError{Step: 1, Cause: errIO})
	if hint == nil {
		t.Fatalf("synthesizeCrashHint returned nil")
	}
	if !strings.Contains(hint.Summary, "step 1") {
		t.Errorf("Summary missing step ref: %q", hint.Summary)
	}
	if !strings.Contains(hint.Rationale, "could not connect to redis") {
		t.Errorf("Rationale missing stderr tail; got: %q", hint.Rationale)
	}
}

var errIO = stubErr("io broken")

type stubErr string

func (s stubErr) Error() string { return string(s) }
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/runtime/executor/ -run TestSynthesizeCrashHint -v
```

Expected: FAIL with `undefined: synthesizeCrashHint`.

- [ ] **Step 3: Write `crash.go`**

```go
package executor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iurykrieger/lastro/internal/aggregate"
)

const crashHintTailBytes = 4096

// synthesizeCrashHint builds a HealHint from the trailing crashHintTailBytes
// of raw.log. Used when a step exits non-zero with zero Signals — the
// per-sensor rollup would otherwise have no hint to attach.
//
// rawPath is the absolute path to <runDir>/raw.log.
func synthesizeCrashHint(rawPath string, cause error) *aggregate.HealHint {
	tail := readTail(rawPath, crashHintTailBytes)
	stderr := filterStream(tail, "stderr")
	rationale := fmt.Sprintf("stderr tail:\n%s", strings.TrimRight(stderr, "\n"))
	if cause != nil && cause.Error() != "" {
		rationale = fmt.Sprintf("%s\n\nunderlying cause: %s", rationale, cause.Error())
	}
	step := "?"
	if se, ok := cause.(interface{ stepIndex() int }); ok {
		step = fmt.Sprintf("%d", se.stepIndex())
	}
	// Use type-specific accessors when available.
	switch e := cause.(type) {
	case *SpawnError:
		step = fmt.Sprintf("%d", e.Step)
	case *TemplateError:
		step = fmt.Sprintf("%d", e.Step)
	}
	return &aggregate.HealHint{
		Summary:   fmt.Sprintf("sensor crashed at step %s (no signals emitted)", step),
		Rationale: rationale,
	}
}

// readTail returns the last n bytes of path; an empty string on error.
func readTail(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return ""
	}
	size := stat.Size()
	if size > int64(n) {
		if _, err := f.Seek(size-int64(n), io.SeekStart); err != nil {
			return ""
		}
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(b)
}

// filterStream returns only lines from raw whose annotated stream tag
// matches `stream` (e.g. "stderr").
func filterStream(raw, stream string) string {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	var b strings.Builder
	tag := " " + stream + "] "
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "[") {
			continue
		}
		idx := strings.Index(line, tag)
		if idx < 0 {
			continue
		}
		b.WriteString(line[idx+len(tag):])
		b.WriteByte('\n')
	}
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/runtime/executor/ -run TestSynthesizeCrashHint -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/runtime/executor/crash.go internal/runtime/executor/crash_test.go
git commit -m "feat(runtime/executor): synthesizeCrashHint from raw.log stderr tail

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 11: Executor.Run — multi-step orchestration

**Files:**
- Create: `internal/runtime/executor/executor.go`
- Create: `internal/runtime/executor/run_test.go`
- Create: `internal/runtime/executor/testdata/golden/assertion_pass.json`

- [ ] **Step 1: Write `executor.go`**

```go
package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/runtime/process"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// Options is the dependency wiring an Executor needs. All fields are
// read-only after New; concurrent Run calls are safe.
type Options struct {
	RepoRoot      string
	Resolver      *template.Resolver
	FixtureStore  fixture.FixtureStore
	UseCaseLookup func(sensorID string) (*usecase.UseCase, bool)
	Now           func() time.Time
	Shell         []string
	GroupSignaler process.GroupSignaler
	OnStepStart   func(stepIdx, pid, pgid int)
}

// Executor is a stateless function-table over Options. Construct once,
// call Run as many times as needed.
type Executor struct{ opts Options }

func New(opts Options) *Executor {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.GroupSignaler == nil {
		opts.GroupSignaler = process.Default()
	}
	return &Executor{opts: opts}
}

// Run executes one sensor end-to-end against runDir. runDir must already
// exist; Run creates runDir/scratch on demand. The caller controls Stop
// (typically nil for assertion sensors).
//
// expectedObs is forwarded to aggregate.RollupInput; pass nil for
// assertion sensors.
func (e *Executor) Run(
	ctx context.Context,
	s sensor.Sensor,
	runDir string,
	expectedObs []string,
	stop <-chan struct{},
) (aggregate.AggregateSignal, error) {
	uc, ok := e.opts.UseCaseLookup(s.ID)
	if !ok {
		return aggregate.AggregateSignal{}, fmt.Errorf("executor: use case lookup failed for sensor %q", s.ID)
	}

	if err := os.MkdirAll(filepath.Join(runDir, "scratch"), 0o700); err != nil {
		return aggregate.AggregateSignal{}, fmt.Errorf("executor: mkdir runDir: %w", err)
	}
	rawPath := filepath.Join(runDir, "raw.log")
	signalsPath := filepath.Join(runDir, "signals.jsonl")
	rl, err := newRawLog(rawPath, e.opts.Now)
	if err != nil {
		return aggregate.AggregateSignal{}, err
	}
	defer rl.Close()
	sw, err := newJSONLWriter(signalsPath)
	if err != nil {
		return aggregate.AggregateSignal{}, err
	}
	defer sw.Close()

	startedAt := e.opts.Now()
	allSignals := []aggregate.Signal{}
	observedKeys := []string{}
	termReason := enums.TerminationCompleted
	var stepErr error

	for i, step := range s.Steps {
		stepIdx := i + 1
		outcome, err := runStep(ctx, stepArgs{
			Step:        step,
			StepIdx:     stepIdx,
			RunDir:      runDir,
			UseCase:     uc,
			Store:       e.opts.FixtureStore,
			Resolver:    e.opts.Resolver,
			Signaler:    e.opts.GroupSignaler,
			Shell:       e.opts.Shell,
			ExpectedObs: expectedObs,
			RawLog:      rl,
			SignalsW:    sw,
			Stop:        stop,
			OnStart:     e.opts.OnStepStart,
		})
		allSignals = append(allSignals, toAggregateSignals(outcome.Signals)...)
		observedKeys = append(observedKeys, outcome.ObservationKeys...)

		// Structural error (template/spawn/binder) → halt with error.
		if err != nil {
			termReason = enums.TerminationError
			stepErr = err
			break
		}

		// External stop or context cancel → halt with appropriate reason.
		switch {
		case outcome.StoppedExternally:
			termReason = enums.TerminationStopped
		case errors.Is(outcome.CtxErr, context.DeadlineExceeded):
			termReason = enums.TerminationTimeout
			stepErr = outcome.CtxErr
		case errors.Is(outcome.CtxErr, context.Canceled):
			termReason = enums.TerminationStopped
		case outcome.ExitErr != nil && len(outcome.Signals) == 0:
			termReason = enums.TerminationError
			stepErr = fmt.Errorf("%w: %v", ErrStepCrashed, outcome.ExitErr)
		}
		if termReason != enums.TerminationCompleted {
			break
		}
	}

	endedAt := e.opts.Now()

	agg, rollupErr := aggregate.Rollup(aggregate.RollupInput{
		Signals:              allSignals,
		SensorID:             s.ID,
		UseCaseID:            s.UseCaseID,
		Angle:                s.Angle,
		Kind:                 s.Kind,
		OutputType:           s.OutputType,
		StartedAt:            startedAt,
		EndedAt:              endedAt,
		TerminationReason:    termReason,
		ExpectedObservations: expectedObs,
		ObservedKeys:         observedKeys,
	})
	if rollupErr != nil {
		return aggregate.AggregateSignal{}, fmt.Errorf("executor: rollup: %w", rollupErr)
	}

	// Crash-hint patch: if error termination produced an aggregate with
	// no heal hint, synthesize one from raw.log.
	if termReason == enums.TerminationError && agg.HealHint == nil && stepErr != nil {
		agg.HealHint = synthesizeCrashHint(rawPath, stepErr)
	}

	return agg, nil
}

// toAggregateSignals converts signal.Signals (the public typed record)
// into aggregate.signalstub.Signal, the structurally identical type the
// per-sensor rollup consumes. aggregate.Rollup defines its input slice
// over signalstub.Signal; the conversion is a field-for-field copy.
//
// (aggregate already re-exports HealHint/Locus from signalstub; the
// underlying Signal type is exported indirectly via the aggregate
// package's stub package — see internal/aggregate/internal/signalstub.)
func toAggregateSignals(in []signal.Signal) []aggregate.Signal {
	out := make([]aggregate.Signal, len(in))
	for i, s := range in {
		out[i] = aggregate.Signal{
			SchemaVersion: s.SchemaVersion,
			SensorID:      s.SensorID,
			UseCaseID:     s.UseCaseID,
			Angle:         s.Angle,
			EmittedAt:     s.EmittedAt,
			Verdict:       s.Verdict,
			Confidence:    s.Confidence,
			Evidence:      aggregate.Evidence(s.Evidence),
			HealHint:      s.HealHint,
		}
	}
	return out
}
```

> **Coordination note for the implementer:** `aggregate.Signal` and `aggregate.Evidence` may not be exported from `internal/aggregate` today (the package keeps the type behind `signalstub`). If they aren't, the simplest landing is to add re-export aliases at the top of `internal/aggregate/types.go`:
>
> ```go
> type Signal = signalstub.Signal
> type Evidence = signalstub.Evidence
> ```
>
> Then `aggregate.RollupInput.Signals` already takes `[]signalstub.Signal` ≡ `[]aggregate.Signal`. Verify by reading `internal/aggregate/types.go` and `internal/aggregate/internal/signalstub/signalstub.go` first; if `Signal` is not yet re-exported, add the aliases as a small co-change in this task and adjust the import in `executor.go` accordingly.

- [ ] **Step 2: Verify aggregate exports Signal**

```bash
grep -n "type Signal " internal/aggregate/types.go internal/aggregate/internal/signalstub/signalstub.go
```

If `aggregate.Signal` (or alias) does NOT appear in `types.go`, add:

```go
// (in internal/aggregate/types.go, near the existing HealHint alias)
type Signal = signalstub.Signal
type Evidence = signalstub.Evidence
```

Commit that as a tiny co-change before continuing:

```bash
git add internal/aggregate/types.go
git commit -m "feat(aggregate): re-export Signal + Evidence aliases for executor

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

- [ ] **Step 3: Write a minimal sensor + use case fixture for the run test**

Create `internal/runtime/executor/testdata/sensors/assertion_pass.yaml`:

```yaml
schema_version: 1.0.0
id: fake-pass-sensor
use_case_id: fake-uc
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses:
  - fake-stack
steps:
  - id: only-step
    run: "FAKESENSOR signal pass"
```

The `FAKESENSOR` token is a placeholder; the test replaces it with the absolute path to the compiled fakesensor binary before loading.

- [ ] **Step 4: Write `run_test.go` end-to-end test**

```go
package executor

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// fakeSensorBin holds the path to the compiled fakesensor binary, built
// once by TestMain.
var fakeSensorBin string

func TestMain(m *testing.M) {
	bin, err := buildFakeSensor()
	if err != nil {
		panic("build fakesensor: " + err.Error())
	}
	fakeSensorBin = bin
	code := m.Run()
	_ = os.Remove(bin)
	os.Exit(code)
}

func buildFakeSensor() (string, error) {
	dir, err := os.MkdirTemp("", "fakesensor-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "fakesensor")
	if isWindows() {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "../../testutil/fakesensor/main.go")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

func isWindows() bool {
	return strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") || strings.HasSuffix(strings.ToLower(os.Getenv("ComSpec")), "cmd.exe")
}

func TestRunAssertion_PassSingleStep(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0",
		ID:            "fake-pass-sensor",
		UseCaseID:     "fake-uc",
		Angle:         enums.AngleBuild,
		Kind:          enums.KindAssertion,
		Nature:        enums.NatureComputational,
		OutputType:    enums.OutputSingleShot,
		Uses:          []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "only", Run: fakeSensorBin + " signal pass"},
		},
	}
	exec := New(Options{
		RepoRoot:     t.TempDir(),
		Resolver:     &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore: emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) {
			if id == s.ID {
				return uc, true
			}
			return nil, false
		},
		Now: fixedExecNow,
	})

	runDir := t.TempDir()
	agg, err := exec.Run(context.Background(), s, runDir, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass", agg.Verdict)
	}
	if agg.TerminationReason != enums.TerminationCompleted {
		t.Errorf("termination_reason = %q, want completed", agg.TerminationReason)
	}
	if agg.Rollup.TotalSignals != 1 || agg.Rollup.PassCount != 1 {
		t.Errorf("rollup = %+v, want 1 pass", agg.Rollup)
	}
	// signals.jsonl should contain exactly one decoded line.
	b, _ := os.ReadFile(filepath.Join(runDir, "signals.jsonl"))
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("signals.jsonl line count = %d, want 1", len(lines))
	}
}

func TestRunAssertion_GoldenAggregate(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "fake-pass-sensor", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{{ID: "only", Run: fakeSensorBin + " signal pass --angle build"}},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})

	agg, err := ex.Run(context.Background(), s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Zero EmittedAt timestamps in the rolled-up signals to make the
	// golden comparison stable (the fakesensor stamps wall clock).
	// Since RollupCounts and verdict/confidence are deterministic, we
	// compare a normalized subset of the aggregate.
	got := normalizeForGolden(agg)
	want := readGolden(t, "testdata/golden/assertion_pass.json")
	if got != want {
		t.Errorf("aggregate mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func normalizeForGolden(a aggregate.AggregateSignal) string {
	a.StartedAt = goldenTime
	a.EndedAt = goldenTime
	b, _ := json.MarshalIndent(a, "", "  ")
	return string(b)
}

func readGolden(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	return strings.TrimRight(string(b), "\n")
}

var goldenTime, _ = time.Parse(time.RFC3339Nano, "2026-05-24T10:00:00Z")
var fixedExecNow = func() time.Time { return goldenTime }

type emptyStore struct{}

func (emptyStore) LookupFixture(id string) (fixture.Fixture, bool) { return fixture.Fixture{}, false }
func (emptyStore) FixturesForUseCase(uc string) []fixture.Fixture  { return nil }
```

- [ ] **Step 5: Generate the golden file (manual one-time)**

Run the first test (`TestRunAssertion_PassSingleStep`) to confirm Run works, then run the golden test once to capture output, paste into the golden file:

```bash
go test ./internal/runtime/executor/ -run TestRunAssertion_GoldenAggregate -v 2>&1 | head -100
```

Inspect the failure message ("got: …"), copy the JSON block into `internal/runtime/executor/testdata/golden/assertion_pass.json`. The expected golden content roughly looks like:

```json
{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "fake-pass-sensor",
  "use_case_id": "fake-uc",
  "angle": "build",
  "started_at": "2026-05-24T10:00:00Z",
  "ended_at": "2026-05-24T10:00:00Z",
  "termination_reason": "completed",
  "verdict": "pass",
  "confidence": 1,
  "rollup": {
    "total_signals": 1,
    "pass_count": 1,
    "warn_count": 0,
    "fail_count": 0,
    "inconclusive_count": 0
  }
}
```

(Exact fields depend on the AggregateSignal JSON contract — paste verbatim from the actual run.)

- [ ] **Step 6: Re-run the golden test; verify it passes**

```bash
go test ./internal/runtime/executor/ -run TestRunAssertion -v -race
```

Expected: both `TestRunAssertion_*` tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/runtime/executor/executor.go internal/runtime/executor/run_test.go internal/runtime/executor/testdata/
git commit -m "feat(runtime/executor): Run orchestration + golden assertion test

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 12: Cancellation, timeout, and crash tests

**Files:**
- Modify: `internal/runtime/executor/run_test.go`

- [ ] **Step 1: Add the cancellation test**

Append to `run_test.go`:

```go
func TestRunAssertion_ContextCancellationKillsChild(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "slow-sensor", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{{ID: "slow", Run: fakeSensorBin + " sleep 5s"}},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	agg, err := ex.Run(ctx, s, t.TempDir(), nil, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run returned error (should still complete with verdict): %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run took %v; should have aborted quickly after cancel", elapsed)
	}
	if agg.TerminationReason != enums.TerminationStopped {
		t.Errorf("termination_reason = %q, want stopped", agg.TerminationReason)
	}
}

func TestRunAssertion_TimeoutReports(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "slow-sensor", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{{ID: "slow", Run: fakeSensorBin + " sleep 5s"}},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	agg, err := ex.Run(ctx, s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.TerminationReason != enums.TerminationTimeout {
		t.Errorf("termination_reason = %q, want timeout", agg.TerminationReason)
	}
}

func TestRunAssertion_CrashedStepSynthesizesHint(t *testing.T) {
	uc := &usecase.UseCase{ID: "fake-uc"}
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "crash-sensor", UseCaseID: "fake-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake-stack"},
		Steps: []sensor.Step{
			{ID: "boom", Run: fakeSensorBin + ` crash --exit-code 2 --stderr "could not connect to redis"`},
		},
	}
	ex := New(Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
		Now:           fixedExecNow,
	})

	agg, err := ex.Run(context.Background(), s, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.TerminationReason != enums.TerminationError {
		t.Errorf("termination_reason = %q, want error", agg.TerminationReason)
	}
	if agg.HealHint == nil {
		t.Fatalf("heal_hint is nil; want synthesized hint")
	}
	if !strings.Contains(agg.HealHint.Rationale, "could not connect to redis") {
		t.Errorf("heal_hint.rationale missing stderr: %q", agg.HealHint.Rationale)
	}
}
```

- [ ] **Step 2: Run the new tests**

```bash
go test ./internal/runtime/executor/ -run "TestRunAssertion_(ContextCancellation|Timeout|Crashed)" -v -race
```

Expected: all three PASS.

- [ ] **Step 3: Run the entire executor test suite under -race**

```bash
go test ./internal/runtime/executor/ -v -race
```

Expected: 100% PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/runtime/executor/run_test.go
git commit -m "test(runtime/executor): cancellation, timeout, crash-hint scenarios

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

# Phase 4 — `internal/lifecycle`

## Task 13: Dependencies, errors, and `Handle` type

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/lifecycle/errors.go`
- Create: `internal/lifecycle/handle.go`
- Create: `internal/lifecycle/handle_test.go`

- [ ] **Step 1: Add the two new third-party deps**

```bash
go get github.com/rogpeppe/go-internal/lockedfile@latest
go get github.com/oklog/ulid/v2@latest
go mod tidy
```

Expected: `go.mod` now lists both modules under `require`.

- [ ] **Step 2: Create the directory and write `errors.go`**

```bash
mkdir -p internal/lifecycle
```

```go
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
)
```

- [ ] **Step 3: Write the failing test for `Handle` JSON**

```go
package lifecycle

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHandle_JSONRoundTrip(t *testing.T) {
	h := Handle{
		SensorID:             "fake-sensor",
		RunID:                "01JZQ9G7M0H3FX8N1QPYAS78MV",
		RunDir:               "/abs/.harness/runtime/fake-sensor/01JZQ9G7M0H3FX8N1QPYAS78MV",
		PID:                  42,
		PGID:                 42,
		StartedAt:            time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
		ExpectedObservations: []string{"order-received"},
		HarnessPID:           7,
		HarnessVersion:       "0.1.0",
		GOOS:                 "linux",
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Handle
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SensorID != h.SensorID || got.RunID != h.RunID || got.PID != h.PID || !got.StartedAt.Equal(h.StartedAt) {
		t.Errorf("round-trip mismatch:\ngot:  %+v\nwant: %+v", got, h)
	}
	if len(got.ExpectedObservations) != 1 || got.ExpectedObservations[0] != "order-received" {
		t.Errorf("ExpectedObservations lost: %v", got.ExpectedObservations)
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
go test ./internal/lifecycle/ -run TestHandle_JSONRoundTrip -v
```

Expected: FAIL with `undefined: Handle`.

- [ ] **Step 5: Write `handle.go`**

```go
package lifecycle

import "time"

// Handle is the public reference to a sensor run. It is reconstructable
// from running_sensors.json so a cross-process StopSensor can act
// without sharing in-memory state with StartSensor.
type Handle struct {
	SensorID             string    `json:"sensor_id"`
	RunID                string    `json:"run_id"`
	RunDir               string    `json:"run_dir"`
	PID                  int       `json:"pid"`
	PGID                 int       `json:"pgid"`
	StartedAt            time.Time `json:"started_at"`
	ExpectedObservations []string  `json:"expected_observations,omitempty"`
	HarnessPID           int       `json:"harness_pid"`
	HarnessVersion       string    `json:"harness_version"`
	GOOS                 string    `json:"goos"`
}
```

- [ ] **Step 6: Run test to verify it passes**

```bash
go test ./internal/lifecycle/ -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/lifecycle/errors.go internal/lifecycle/handle.go internal/lifecycle/handle_test.go
git commit -m "feat(lifecycle): typed errors + Handle type + lockedfile/ulid deps

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 14: Runtime directory helpers + central registry

**Files:**
- Create: `internal/lifecycle/runtime_dir.go`
- Create: `internal/lifecycle/runtime_dir_test.go`
- Create: `internal/lifecycle/registry.go`
- Create: `internal/lifecycle/registry_test.go`

- [ ] **Step 1: Write the failing test for runtime_dir**

```go
package lifecycle

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeDir_PathsAreScopedByIDAndRun(t *testing.T) {
	root := t.TempDir()
	got := runDirPath(root, "create-order-sensor", "01JZQ9G7M0H3FX8N1QPYAS78MV")
	want := filepath.Join(root, "create-order-sensor", "01JZQ9G7M0H3FX8N1QPYAS78MV")
	if got != want {
		t.Errorf("runDirPath = %q, want %q", got, want)
	}
}

func TestRegistryPath_IsAtRoot(t *testing.T) {
	got := registryPath(t.TempDir())
	if !strings.HasSuffix(got, string(filepath.Separator)+"running_sensors.json") {
		t.Errorf("registryPath = %q, want trailing running_sensors.json", got)
	}
}
```

- [ ] **Step 2: Write `runtime_dir.go`**

```go
package lifecycle

import "path/filepath"

func runDirPath(runtimeRoot, sensorID, runID string) string {
	return filepath.Join(runtimeRoot, sensorID, runID)
}

func registryPath(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "running_sensors.json")
}
```

- [ ] **Step 3: Write the failing test for the registry**

```go
package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRegistry_EmptyReadReturnsEmptySlice(t *testing.T) {
	root := t.TempDir()
	r := newRegistry(root)
	entries, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %v, want empty", entries)
	}
}

func TestRegistry_AppendAndFind(t *testing.T) {
	root := t.TempDir()
	r := newRegistry(root)
	h := Handle{
		SensorID: "s1", RunID: "r1", RunDir: filepath.Join(root, "s1", "r1"),
		PID: 1234, PGID: 1234, StartedAt: time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
	}
	if err := r.Append(h); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, ok, err := r.Find("s1", "r1")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if !ok {
		t.Fatalf("Find returned ok=false; want true")
	}
	if got.PID != 1234 {
		t.Errorf("got.PID = %d, want 1234", got.PID)
	}
}

func TestRegistry_RemoveByKey(t *testing.T) {
	root := t.TempDir()
	r := newRegistry(root)
	h := Handle{SensorID: "s1", RunID: "r1", PID: 99}
	_ = r.Append(h)
	if err := r.Remove("s1", "r1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, ok, _ := r.Find("s1", "r1")
	if ok {
		t.Errorf("Find after Remove returned ok=true")
	}
}

func TestRegistry_ConcurrentAppendDoesNotLoseEntries(t *testing.T) {
	root := t.TempDir()
	r := newRegistry(root)
	const N = 16
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h := Handle{
				SensorID: "s",
				RunID:    fmt.Sprintf("r%02d", i),
				PID:      1000 + i,
			}
			_ = r.Append(h)
		}(i)
	}
	wg.Wait()
	entries, _ := r.List()
	if len(entries) != N {
		t.Errorf("entries = %d, want %d", len(entries), N)
	}
}

func TestRegistry_AppendCreatesFile(t *testing.T) {
	root := t.TempDir()
	r := newRegistry(root)
	_ = r.Append(Handle{SensorID: "s", RunID: "r", PID: 1})
	if _, err := os.Stat(registryPath(root)); err != nil {
		t.Errorf("registry file not created: %v", err)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

```bash
go test ./internal/lifecycle/ -run TestRegistry -v
```

Expected: FAIL with `undefined: newRegistry`.

- [ ] **Step 5: Write `registry.go`**

```go
package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/rogpeppe/go-internal/lockedfile"
)

// registryDoc is the on-disk shape of running_sensors.json.
type registryDoc struct {
	SchemaVersion string   `json:"schema_version"`
	Entries       []Handle `json:"entries"`
}

const registrySchemaVersion = "1.0.0"

// registry wraps the read-modify-write cycle for running_sensors.json
// under a file lock provided by github.com/rogpeppe/go-internal/lockedfile.
type registry struct {
	path string
}

func newRegistry(runtimeRoot string) *registry {
	return &registry{path: registryPath(runtimeRoot)}
}

// List returns a copy of all entries. Shared read lock.
func (r *registry) List() ([]Handle, error) {
	data, err := lockedfile.Read(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("registry: read: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var doc registryDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("registry: decode: %w", err)
	}
	out := make([]Handle, len(doc.Entries))
	copy(out, doc.Entries)
	return out, nil
}

// Find returns the entry matching (sensorID, runID), if any.
func (r *registry) Find(sensorID, runID string) (Handle, bool, error) {
	entries, err := r.List()
	if err != nil {
		return Handle{}, false, err
	}
	for _, e := range entries {
		if e.SensorID == sensorID && e.RunID == runID {
			return e, true, nil
		}
	}
	return Handle{}, false, nil
}

// Append adds h to the registry under exclusive lock.
func (r *registry) Append(h Handle) error {
	return r.mutate(func(doc *registryDoc) {
		doc.Entries = append(doc.Entries, h)
	})
}

// Remove drops the entry matching (sensorID, runID), if any, under
// exclusive lock. No error if not found.
func (r *registry) Remove(sensorID, runID string) error {
	return r.mutate(func(doc *registryDoc) {
		out := doc.Entries[:0]
		for _, e := range doc.Entries {
			if e.SensorID == sensorID && e.RunID == runID {
				continue
			}
			out = append(out, e)
		}
		doc.Entries = out
	})
}

// UpdatePID locates the entry matching (sensorID, runID) and updates its
// PID/PGID. Used when a multi-step sensor enters the next step (the new
// step has a new PID).
func (r *registry) UpdatePID(sensorID, runID string, pid, pgid int) error {
	return r.mutate(func(doc *registryDoc) {
		for i := range doc.Entries {
			if doc.Entries[i].SensorID == sensorID && doc.Entries[i].RunID == runID {
				doc.Entries[i].PID = pid
				doc.Entries[i].PGID = pgid
				return
			}
		}
	})
}

// Prune removes entries for which keep(entry) returns false. Returns the
// number removed.
func (r *registry) Prune(keep func(Handle) bool) (int, error) {
	removed := 0
	err := r.mutate(func(doc *registryDoc) {
		out := doc.Entries[:0]
		for _, e := range doc.Entries {
			if keep(e) {
				out = append(out, e)
			} else {
				removed++
			}
		}
		doc.Entries = out
	})
	return removed, err
}

// mutate performs a read-modify-write under exclusive file lock.
func (r *registry) mutate(fn func(*registryDoc)) error {
	if err := os.MkdirAll(parentDir(r.path), 0o700); err != nil {
		return fmt.Errorf("registry: mkdir: %w", err)
	}
	mu, err := lockedfile.MutexAt(r.path).Lock()
	if err != nil {
		return fmt.Errorf("registry: lock: %w", err)
	}
	defer mu()

	doc := registryDoc{SchemaVersion: registrySchemaVersion}
	if data, err := os.ReadFile(r.path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("registry: decode existing: %w", err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("registry: read existing: %w", err)
	}

	fn(&doc)
	if doc.SchemaVersion == "" {
		doc.SchemaVersion = registrySchemaVersion
	}

	tmp := r.path + ".tmp"
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("registry: encode: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("registry: write tmp: %w", err)
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return fmt.Errorf("registry: rename: %w", err)
	}
	return nil
}

func parentDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return "."
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/lifecycle/ -v -race
```

Expected: all PASS, including the concurrent-append race test.

- [ ] **Step 7: Commit**

```bash
git add internal/lifecycle/runtime_dir.go internal/lifecycle/runtime_dir_test.go internal/lifecycle/registry.go internal/lifecycle/registry_test.go
git commit -m "feat(lifecycle): runtime_dir helpers + file-locked registry

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 15: Lifecycle.RunSensor (synchronous, both kinds)

**Files:**
- Create: `internal/lifecycle/lifecycle.go`
- Create: `internal/lifecycle/lifecycle_test.go`
- Create: `internal/lifecycle/testdata/sensors/assertion_pass.yaml`

- [ ] **Step 1: Write `lifecycle.go` (initial: RunSensor only; Start/Stop in later tasks)**

```go
package lifecycle

import (
	"context"
	"crypto/rand"
	"errors"
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

// Options wires a Lifecycle. All fields are read-only after New.
type Options struct {
	Sensors     sensor.Store
	Executor    *executor.Executor
	RuntimeRoot string                  // typically <repo>/.harness/runtime
	NewRunID    func() string           // optional; defaults to ULID
	Now         func() time.Time        // optional; defaults to time.Now
	GracePeriod time.Duration           // optional; defaults to 5s
	Version     string                  // harness version recorded in Handles
	Signaler    process.GroupSignaler   // optional; defaults to process.Default()
}

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

// RunSensor synchronously runs the sensor identified by sensorID.
// Works for both assertion and observational kinds; for observational
// sensors, RunSensor blocks until the subprocess exits on its own (or
// until ctx cancels).
//
// expectedObs is the observation-key list passed through to the
// executor's RollupInput; pass nil for assertion sensors.
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

	// Best-effort prune of dead entries before adding this run.
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

	// Hook OnStepStart: on the first step, append the entry; on later
	// steps, update the PID/PGID.
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
			_ = l.registry.Append(*h)
			registered = true
		} else {
			_ = l.registry.UpdatePID(sensorID, runID, pid, pgid)
		}
	}

	// Build a per-Run Executor with the OnStepStart hook.
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

	agg, err := exec.Run(ctx, s, runDir, expectedObs, stopCh)
	if err != nil {
		return aggregate.AggregateSignal{}, err
	}

	// Persist aggregate.json for cross-process Stop / orphan recovery.
	if encErr := writeAggregateJSON(filepath.Join(runDir, "aggregate.json"), agg); encErr != nil {
		// Non-fatal: aggregate is still returned in memory.
		_ = encErr
	}
	return agg, nil
}

// pruneDead removes registry entries whose PIDs are no longer alive.
// Returns the number removed.
func (l *Lifecycle) pruneDead() (int, error) {
	return l.registry.Prune(func(h Handle) bool {
		return l.opts.Signaler.IsAlive(h.PID, h.StartedAt)
	})
}

// writeAggregateJSON serializes the aggregate to the given path
// atomically (temp + rename).
func writeAggregateJSON(path string, agg aggregate.AggregateSignal) error {
	tmp := path + ".tmp"
	data, err := marshalIndent(agg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// marshalIndent JSON-encodes v with stable 2-space indent. Wrapped so
// tests can swap the encoder later if needed.
func marshalIndent(v any) ([]byte, error) {
	return jsonMarshalIndent(v)
}

var jsonMarshalIndent = func(v any) ([]byte, error) {
	// stdlib import inside an init-style closure keeps the surface
	// clean; the encoding/json package is already a transitive dep.
	return jsonEncode(v)
}

// errLifecycleNotImplemented is a placeholder for the Start/Stop tasks.
var errLifecycleNotImplemented = errors.New("lifecycle: feature lands in a later task")
```

> **Coordination note for the implementer:** `executor.Executor` in the spec is opaque about exposing its own `Options`. The simplest landing pattern is to add a small accessor:
>
> ```go
> // in internal/runtime/executor/executor.go
> func (e *Executor) OptionsRef() Options { return e.opts }
> ```
>
> This is a tiny extension. Add it now (commit as part of this task), update Phase 3 callers if needed, and re-run executor tests. Alternative: Lifecycle holds its own `executor.Options` directly and constructs Executors inline per RunSensor (then `Lifecycle.opts.Executor` becomes `Lifecycle.opts.ExecOpts executor.Options`). Pick the alternative if `OptionsRef` feels too leaky; document the choice in the commit message.

Also add `jsonEncode` to a new file `internal/lifecycle/json.go`:

```go
package lifecycle

import "encoding/json"

func jsonEncode(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
```

- [ ] **Step 2: Add `Lookup` to `sensor.Store`**

```bash
grep -n "func.*Lookup" internal/sensor/store.go
```

If `Lookup(id string) (Sensor, bool)` does NOT exist, add it. Test sensors get loaded into the store; Lifecycle just uses Lookup.

- [ ] **Step 3: Create a minimal test sensor YAML**

`internal/lifecycle/testdata/sensors/assertion_pass.yaml`:

```yaml
schema_version: 1.0.0
id: lifecycle-assertion-pass
use_case_id: lifecycle-uc
angle: build
kind: assertion
nature: computational
output_type: single-shot
uses:
  - fake
steps:
  - id: only
    run: "FAKESENSOR signal pass"
```

(`FAKESENSOR` is rewritten at test time to the absolute path to the compiled fakesensor binary.)

- [ ] **Step 4: Write the failing test**

Append to `lifecycle_test.go`:

```go
package lifecycle

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	rxexec "github.com/iurykrieger/lastro/internal/runtime/executor"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

var fakeSensorBin string

func TestMain(m *testing.M) {
	bin, err := buildFakeSensor()
	if err != nil {
		panic("build fakesensor: " + err.Error())
	}
	fakeSensorBin = bin
	code := m.Run()
	_ = os.Remove(bin)
	os.Exit(code)
}

func buildFakeSensor() (string, error) {
	dir, err := os.MkdirTemp("", "fakesensor-lc-")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "fakesensor")
	cmd := exec.Command("go", "build", "-o", out, "../testutil/fakesensor/main.go")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

func TestRunSensor_ErrSensorNotFound(t *testing.T) {
	lc := newTestLifecycle(t, nil)
	_, err := lc.RunSensor(context.Background(), "no-such-sensor", nil)
	if !errors.Is(err, ErrSensorNotFound) {
		t.Errorf("err = %v, want ErrSensorNotFound", err)
	}
}

func TestRunSensor_AssertionPass(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "lifecycle-assertion-pass", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake"},
		Steps: []sensor.Step{{ID: "only", Run: fakeSensorBin + " signal pass"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	agg, err := lc.RunSensor(context.Background(), s.ID, nil)
	if err != nil {
		t.Fatalf("RunSensor: %v", err)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass", agg.Verdict)
	}

	// aggregate.json should exist under the run dir.
	matches, _ := filepath.Glob(filepath.Join(lc.opts.RuntimeRoot, s.ID, "*", "aggregate.json"))
	if len(matches) != 1 {
		t.Errorf("aggregate.json count = %d, want 1", len(matches))
	}

	// Registry should be empty (the entry was removed on completion).
	entries, _ := lc.registry.List()
	if len(entries) != 0 {
		t.Errorf("registry entries after Run = %d, want 0; entries: %+v", len(entries), entries)
	}
}

// newTestLifecycle constructs a Lifecycle with a stub sensor store and
// an executor wired with empty stores. Use deterministic Now/NewRunID.
func newTestLifecycle(t *testing.T, sensors []sensor.Sensor) *Lifecycle {
	t.Helper()
	store := &stubSensorStore{by: map[string]sensor.Sensor{}}
	for _, s := range sensors {
		store.by[s.ID] = s
	}
	uc := &usecase.UseCase{ID: "lifecycle-uc"}
	ex := rxexec.New(rxexec.Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
	})
	root := t.TempDir()
	counter := 0
	return New(Options{
		Sensors:     store,
		Executor:    ex,
		RuntimeRoot: root,
		NewRunID: func() string {
			counter++
			return strings.Repeat("0", 25) + string(rune('A'+counter-1))
		},
		Version: "test-0.0.0",
	})
}

type stubSensorStore struct{ by map[string]sensor.Sensor }

func (s *stubSensorStore) Lookup(id string) (sensor.Sensor, bool) {
	v, ok := s.by[id]
	return v, ok
}

type emptyStore struct{}

func (emptyStore) LookupFixture(id string) (fixture.Fixture, bool) { return fixture.Fixture{}, false }
func (emptyStore) FixturesForUseCase(uc string) []fixture.Fixture  { return nil }
```

> **Coordination note:** if `internal/sensor.Store` is an interface, ensure `stubSensorStore` implements all of its methods. If `Store` is a struct, change the field type to `sensor.Store` and load via `(*store).Add(...)`. Read `internal/sensor/store.go` first to confirm.

- [ ] **Step 5: Run the tests**

```bash
go test ./internal/lifecycle/ -v -race
```

Expected: both `TestRunSensor_*` tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/lifecycle/lifecycle.go internal/lifecycle/json.go internal/lifecycle/lifecycle_test.go internal/lifecycle/testdata/
# If executor.OptionsRef was added, also stage that:
git add internal/runtime/executor/executor.go
# If sensor.Store needed adjustments, stage those too.
git commit -m "feat(lifecycle): RunSensor (synchronous run for both sensor kinds)

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 16: Lifecycle.StartSensor (observational only)

**Files:**
- Modify: `internal/lifecycle/lifecycle.go`
- Modify: `internal/lifecycle/lifecycle_test.go`

- [ ] **Step 1: Add `StartSensor` to `lifecycle.go`**

Insert after `RunSensor`:

```go
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
				select { case startErrCh <- err: default: }
				return
			}
			registered = true
			select { case startedCh <- struct{}{}: default: }
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
```

- [ ] **Step 2: Append the observational test**

```go
func TestStartSensor_ObservationalEmitsAndAppearsInRegistry(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "obs-pass", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses: []string{"fake"},
		Steps: []sensor.Step{
			{ID: "watch", Run: fakeSensorBin + " watch --emit order-received --emit order-validated --emit order-persisted --interval 30ms"},
		},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	h, err := lc.StartSensor(context.Background(), s.ID, []string{"order-received", "order-validated", "order-persisted"})
	if err != nil {
		t.Fatalf("StartSensor: %v", err)
	}
	if h == nil || h.RunID == "" {
		t.Fatalf("nil handle / empty RunID")
	}

	// Registry should now show one entry.
	entries, _ := lc.registry.List()
	if len(entries) != 1 || entries[0].SensorID != s.ID {
		t.Errorf("registry entries = %+v, want 1 for %q", entries, s.ID)
	}

	t.Cleanup(func() {
		_, _ = lc.StopSensor(context.Background(), h)
	})

	// Wait briefly for signals.jsonl to accumulate.
	signalsPath := filepath.Join(h.RunDir, "signals.jsonl")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(signalsPath); err == nil && bytes.Count(b, []byte{'\n'}) >= 3 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("signals.jsonl did not accumulate 3 lines within deadline")
}

func TestStartSensor_ErrAssertionSensor(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "assertion-only", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleBuild, Kind: enums.KindAssertion, Nature: enums.NatureComputational, OutputType: enums.OutputSingleShot,
		Uses: []string{"fake"},
		Steps: []sensor.Step{{ID: "only", Run: fakeSensorBin + " signal pass"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	_, err := lc.StartSensor(context.Background(), s.ID, nil)
	if !errors.Is(err, ErrAssertionSensor) {
		t.Errorf("err = %v, want ErrAssertionSensor", err)
	}
}
```

Add `"bytes"` and `"time"` to the file's import block.

- [ ] **Step 3: Run the new tests**

```bash
go test ./internal/lifecycle/ -run "TestStartSensor" -v -race
```

Expected: both PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/lifecycle/lifecycle.go internal/lifecycle/lifecycle_test.go
git commit -m "feat(lifecycle): StartSensor for observational sensors

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 17: Lifecycle.StopSensor + orphan recovery

**Files:**
- Modify: `internal/lifecycle/lifecycle.go`
- Modify: `internal/lifecycle/lifecycle_test.go`

- [ ] **Step 1: Add `StopSensor`, `ListRunning`, `LoadHandle` to `lifecycle.go`**

```go
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
			Evidence: aggregate.Evidence(sig.Evidence), HealHint: sig.HealHint,
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
```

Add helpers to `internal/lifecycle/json.go`:

```go
func jsonDecode(b []byte, v any) error {
	return json.Unmarshal(b, v)
}
```

Add `"github.com/iurykrieger/lastro/internal/signal"` to lifecycle.go imports.

- [ ] **Step 2: Add in-process Stop test**

```go
func TestStopSensor_InProcessFastPath(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "obs-stop", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses: []string{"fake"},
		Steps: []sensor.Step{{ID: "watch", Run: fakeSensorBin + " watch --emit k1 --interval 20ms"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	h, err := lc.StartSensor(context.Background(), s.ID, []string{"k1"})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(80 * time.Millisecond) // let it emit the one observation

	agg, err := lc.StopSensor(context.Background(), h)
	if err != nil {
		t.Fatalf("StopSensor: %v", err)
	}
	if agg.TerminationReason != enums.TerminationStopped {
		t.Errorf("termination_reason = %q, want stopped", agg.TerminationReason)
	}
	if agg.Verdict != enums.VerdictPass {
		t.Errorf("verdict = %q, want pass (observation arrived)", agg.Verdict)
	}

	entries, _ := lc.ListRunning()
	if len(entries) != 0 {
		t.Errorf("ListRunning after Stop = %d, want 0", len(entries))
	}
}

func TestStopSensor_FailWhenObservationMissing(t *testing.T) {
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "obs-missing", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses: []string{"fake"},
		Steps: []sensor.Step{{ID: "watch", Run: fakeSensorBin + " watch --emit k1 --interval 20ms"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	h, err := lc.StartSensor(context.Background(), s.ID, []string{"k1", "k2-never-arrives"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)

	agg, err := lc.StopSensor(context.Background(), h)
	if err != nil {
		t.Fatalf("StopSensor: %v", err)
	}
	if agg.Verdict != enums.VerdictFail {
		t.Errorf("verdict = %q, want fail (missing observation)", agg.Verdict)
	}
	if agg.HealHint == nil {
		t.Errorf("heal_hint is nil; want observational-missing hint")
	}
}
```

- [ ] **Step 3: Run the new tests**

```bash
go test ./internal/lifecycle/ -run "TestStopSensor" -v -race
```

Expected: both PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/lifecycle/lifecycle.go internal/lifecycle/json.go internal/lifecycle/lifecycle_test.go
git commit -m "feat(lifecycle): StopSensor + orphan recovery from signals.jsonl

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 18: Cross-process round-trip test (subtest fork)

**Files:**
- Modify: `internal/lifecycle/lifecycle_test.go`

- [ ] **Step 1: Add the cross-process round-trip test**

The pattern: the parent test calls `StartSensor`, then re-execs itself (`os.Args[0]`) with `-test.run=TestStopFromOtherProcess_Child` and env vars carrying the sensor id + run id. The child looks up the handle from the registry and calls `StopSensor`.

```go
func TestStopFromOtherProcess(t *testing.T) {
	// Only run as the "parent" half here; the child half exits directly.
	if os.Getenv("HARNESS_TEST_CHILD") == "1" {
		t.Skip("invoked as child; will be re-entered via the child test")
	}

	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: "obs-cross", UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses: []string{"fake"},
		Steps: []sensor.Step{{ID: "watch", Run: fakeSensorBin + " watch --emit k1 --interval 30ms"}},
	}
	lc := newTestLifecycle(t, []sensor.Sensor{s})

	h, err := lc.StartSensor(context.Background(), s.ID, []string{"k1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = lc.StopSensor(context.Background(), h) })

	time.Sleep(80 * time.Millisecond) // let one observation arrive

	// Re-exec ourselves as the "child" run. The child looks the run up
	// via the registry and calls StopSensor.
	cmd := exec.Command(os.Args[0],
		"-test.run", "TestStopFromOtherProcess_Child",
		"-test.v",
	)
	cmd.Env = append(os.Environ(),
		"HARNESS_TEST_CHILD=1",
		"HARNESS_TEST_RUNTIME_ROOT="+lc.opts.RuntimeRoot,
		"HARNESS_TEST_SENSOR_ID="+s.ID,
		"HARNESS_TEST_RUN_ID="+h.RunID,
		"HARNESS_TEST_FAKE_SENSOR="+fakeSensorBin,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child invocation failed: %v\noutput:\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("verdict=pass")) {
		t.Errorf("child did not report verdict=pass; output:\n%s", out)
	}
}

// TestStopFromOtherProcess_Child is the re-entrant half. It only runs
// when HARNESS_TEST_CHILD=1, which the parent sets.
func TestStopFromOtherProcess_Child(t *testing.T) {
	if os.Getenv("HARNESS_TEST_CHILD") != "1" {
		t.Skip("not invoked as child")
	}
	root := os.Getenv("HARNESS_TEST_RUNTIME_ROOT")
	sensorID := os.Getenv("HARNESS_TEST_SENSOR_ID")
	runID := os.Getenv("HARNESS_TEST_RUN_ID")
	fake := os.Getenv("HARNESS_TEST_FAKE_SENSOR")

	// Re-register the sensor so synthesizeOrphanAggregate can resolve.
	s := sensor.Sensor{
		SchemaVersion: "1.0.0", ID: sensorID, UseCaseID: "lifecycle-uc",
		Angle: enums.AngleLogs, Kind: enums.KindObservational, Nature: enums.NatureComputational, OutputType: enums.OutputStream,
		Uses: []string{"fake"},
		Steps: []sensor.Step{{ID: "watch", Run: fake + " watch --emit k1"}},
	}
	store := &stubSensorStore{by: map[string]sensor.Sensor{s.ID: s}}
	uc := &usecase.UseCase{ID: "lifecycle-uc"}
	ex := rxexec.New(rxexec.Options{
		RepoRoot:      t.TempDir(),
		Resolver:      &template.Resolver{Fixtures: emptyStore{}, EntryPoints: map[string]entrypoint.EntryPoint{}},
		FixtureStore:  emptyStore{},
		UseCaseLookup: func(id string) (*usecase.UseCase, bool) { return uc, true },
	})
	lc := New(Options{
		Sensors: store, Executor: ex, RuntimeRoot: root,
		NewRunID: func() string { return "child-not-used" },
		Version:  "test-child",
	})
	h, err := lc.LoadHandle(sensorID, runID)
	if err != nil {
		t.Fatalf("LoadHandle: %v", err)
	}
	agg, err := lc.StopSensor(context.Background(), h)
	if err != nil {
		t.Fatalf("StopSensor: %v", err)
	}
	// The parent greps for this exact substring in our stdout.
	fmt.Printf("verdict=%s\n", agg.Verdict)
}
```

Add `"fmt"` to the import list if not already there.

- [ ] **Step 2: Run the round-trip**

```bash
go test ./internal/lifecycle/ -run TestStopFromOtherProcess -v -race -count=1
```

Expected: parent PASSES; output includes the child's `verdict=pass` line.

- [ ] **Step 3: Run all lifecycle tests one more time**

```bash
go test ./internal/lifecycle/ -v -race -count=1
```

Expected: every test passes.

- [ ] **Step 4: Commit**

```bash
git add internal/lifecycle/lifecycle_test.go
git commit -m "test(lifecycle): cross-process StopSensor round-trip

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

# Phase 5 — Polish and acceptance

## Task 19: Race-clean across all new packages

**Files:**
- (none modified unless races are discovered)

- [ ] **Step 1: Run the new packages under -race**

```bash
go test -race -count=1 ./internal/signal/ ./internal/runtime/process/ ./internal/runtime/executor/ ./internal/lifecycle/
```

Expected: PASS for every package. If `-race` reports any data race, fix it inline (most likely culprit: shared map/slice access between the executor's stdout pump goroutine and the main step routine; protect with the existing rawLog mutex or a fresh sync.Mutex).

- [ ] **Step 2: Run the full repo test suite to confirm no regression in Phase A packages**

```bash
go test -race ./...
```

Expected: PASS across the entire module. If a Phase A package broke due to a transitive change (e.g., new aliases in `internal/aggregate`), fix inline and add the file to the next commit.

- [ ] **Step 3: Commit any race / regression fixes (if needed)**

```bash
git add -p
git commit -m "fix(b2): address race / regression surfaced by -race full-repo run

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

(Skip this commit if there is nothing to fix.)

---

## Task 20: Acceptance evidence walkthrough

This task is a manual verification step — no new code. Run each acceptance-criterion test from §9 of the spec and capture the pass evidence.

- [ ] **Step 1: Assertion three-step golden** (`TestRunAssertion_GoldenAggregate`)

```bash
go test ./internal/runtime/executor/ -run TestRunAssertion_GoldenAggregate -v
```

Expected: PASS. Golden file at [`internal/runtime/executor/testdata/golden/assertion_pass.json`](../../../internal/runtime/executor/testdata/golden/assertion_pass.json) matches actual.

- [ ] **Step 2: `ErrSensorNotFound` on missing id** (`TestRunSensor_ErrSensorNotFound`)

```bash
go test ./internal/lifecycle/ -run TestRunSensor_ErrSensorNotFound -v
```

Expected: PASS.

- [ ] **Step 3: Observational verdict matrix** (`TestStopSensor_InProcessFastPath`, `TestStopSensor_FailWhenObservationMissing`)

```bash
go test ./internal/lifecycle/ -run "TestStopSensor" -v
```

Expected: both PASS.

- [ ] **Step 4: Context cancellation kills child** (`TestRunAssertion_ContextCancellationKillsChild`)

```bash
go test ./internal/runtime/executor/ -run TestRunAssertion_ContextCancellationKillsChild -v
```

Expected: PASS; assertion confirms total time < 2s and `termination_reason=stopped`.

- [ ] **Step 5: Cross-process round-trip** (`TestStopFromOtherProcess`)

```bash
go test ./internal/lifecycle/ -run TestStopFromOtherProcess -v -count=1
```

Expected: parent PASS; child reports `verdict=pass`.

- [ ] **Step 6: -race clean across the touched tree**

```bash
go test -race ./internal/signal/ ./internal/runtime/process/ ./internal/runtime/executor/ ./internal/lifecycle/
```

Expected: PASS.

- [ ] **Step 7: Final commit (housekeeping only)**

If any documentation tweaks, `go mod tidy`, or formatting passes are needed (`gofmt -s -w ./...`), apply them and commit as `chore(b2): housekeeping after acceptance run`. Skip if clean.

```bash
gofmt -s -w ./...
go mod tidy
git status
```

If `git status` shows nothing modified, the implementation is clean. Otherwise commit the result.

- [ ] **Step 8: Push the branch — DO NOT open the PR**

```bash
git push -u origin feat/b2-executor-lifecycle
```

The PR opening is a separate, human-supervised action per the harness framework's working norms.

---

## Done criteria

The branch is ready to merge when:

- All twenty tasks are checked off, each with its own commit.
- `go test -race ./...` is green on the host running the tests.
- `GOOS=windows go build ./...` succeeds (cross-compile sanity for Windows-specific code paths).
- The five acceptance-criterion tests from §9 of the spec each pass in isolation.
- The spec's three documented follow-ups are linked in the PR description (no code action needed in B2):
  1. Potential `expected_observations` field on `Sensor` schema (deferred to B4 / future).
  2. Per-step `timeout` field (deferred to schema bump if needed).
  3. `harness clean` subcommand for old run dirs (deferred to B6 CLI).






