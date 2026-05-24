//go:build !windows

package process

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPOSIX_SpawnSetsPgidFlag(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "true")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if cmd.SysProcAttr == nil {
		t.Fatalf("SysProcAttr is nil after Spawn")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Errorf("Setpgid = false, want true")
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
	// Spawn a shell that backgrounds a `sleep` child and prints the
	// sleep's PID on stdout. After SignalGroup(SIGTERM), both the shell
	// AND the backgrounded sleep must be dead — that's what proves the
	// signal reached the whole process group.
	cmd := exec.Command("/bin/sh", "-c", "sleep 5 & echo $! ; wait")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pgid, _ := defaultSignaler().GroupID(cmd)

	// Read the first line of stdout: the sleep child's PID.
	var sleepPID int
	{
		buf := make([]byte, 64)
		n, err := stdout.Read(buf)
		if err != nil || n == 0 {
			_ = defaultSignaler().SignalGroup(cmd.Process.Pid, pgid, SignalKill)
			t.Fatalf("could not read sleep PID from shell stdout: %v", err)
		}
		s := strings.TrimSpace(string(buf[:n]))
		// stdout may include later output; take the first whitespace-delimited token.
		if i := strings.IndexAny(s, " \t\n\r"); i >= 0 {
			s = s[:i]
		}
		sleepPID, err = strconv.Atoi(s)
		if err != nil {
			_ = defaultSignaler().SignalGroup(cmd.Process.Pid, pgid, SignalKill)
			t.Fatalf("could not parse sleep PID %q: %v", s, err)
		}
	}

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

	// The shell is dead; now confirm the backgrounded sleep is also dead.
	// kill(pid, 0) returns ESRCH if the process no longer exists.
	// Allow a brief grace window because SIGTERM propagation can race
	// with the wait syscall returning in the shell.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(sleepPID, 0); err != nil {
			return // sleep is dead → success
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Kill(sleepPID, syscall.SIGKILL) // best-effort cleanup
	t.Fatalf("backgrounded sleep (PID %d) was not killed by SignalGroup", sleepPID)
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
	// 2^30 ≈ 1 billion — far above Linux pid_max even when tuned to 4 million on 64-bit.
	if defaultSignaler().IsAlive(1<<30, time.Now()) {
		t.Errorf("IsAlive returned true for non-existent PID")
	}
	if defaultSignaler().IsAlive(0, time.Now()) {
		t.Errorf("IsAlive returned true for PID 0")
	}
}

func TestPOSIX_ReadBootTime(t *testing.T) {
	// readBootTime is Linux-specific.
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only: /proc/stat")
	}
	got, err := readBootTime()
	if err != nil {
		t.Fatalf("readBootTime: %v", err)
	}
	if got.IsZero() {
		t.Errorf("readBootTime returned zero time")
	}
	if got.After(time.Now()) {
		t.Errorf("readBootTime returned future time: %v (now=%v)", got, time.Now())
	}
	// Sanity: boot time should be within the last 10 years.
	if got.Before(time.Now().Add(-10 * 365 * 24 * time.Hour)) {
		t.Errorf("readBootTime returned time more than 10 years ago: %v", got)
	}
}

func TestPOSIX_ProcStartTimeMatchesWithinTolerance(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only: /proc/<pid>/stat")
	}
	cmd := exec.Command("/bin/sh", "-c", "sleep 0.5")
	if err := defaultSignaler().Spawn(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })

	startedAt := time.Now()
	if !procStartTimeMatches(cmd.Process.Pid, startedAt) {
		t.Errorf("procStartTimeMatches returned false for fresh child")
	}
	// A startedAt 30 minutes in the past must NOT match (defeats PID recycling).
	if procStartTimeMatches(cmd.Process.Pid, startedAt.Add(-30*time.Minute)) {
		t.Errorf("procStartTimeMatches returned true for startedAt 30m in the past")
	}
}
