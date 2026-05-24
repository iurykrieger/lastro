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
