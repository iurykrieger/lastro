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
