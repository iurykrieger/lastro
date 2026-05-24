//go:build !windows

package process

import (
	"os/exec"
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
