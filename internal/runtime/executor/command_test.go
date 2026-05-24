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
