//go:build !windows

package process

import (
	"errors"
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
		if errors.Is(err, syscall.EPERM) {
			// Process exists but we can't signal it — still alive.
			return true
		}
		return false
	}
	if runtime.GOOS != "linux" {
		// Best-effort: no cheap start-time read outside Linux.
		return true
	}
	return procStartTimeMatches(pid, startedAt)
}

// procStartTimeMatches reads /proc/<pid>/stat field 22 (1-based kernel
// /proc(5) numbering; index 19 in the post-comm substring) — the
// starttime in clock ticks since boot — and compares it against
// startedAt within a 2-second tolerance.
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
			return time.Time{}, fmt.Errorf("process: parse btime: %w", err)
		}
		return time.Unix(sec, 0), nil
	}
	return time.Time{}, fmt.Errorf("process: btime not found in /proc/stat")
}
