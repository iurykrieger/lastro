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
