//go:build woden_testing && !windows

package gitdep

import (
	"os"
	"syscall"
)

// terminatePID force-terminates the process with the given ID.
func terminatePID(pid int) {
	if pid <= 0 {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = process.Signal(syscall.SIGKILL)
}
