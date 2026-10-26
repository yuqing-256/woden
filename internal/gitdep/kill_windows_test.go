//go:build woden_testing && windows

package gitdep

import "syscall"

// terminatePID force-terminates the process with the given ID. taskkill is not
// usable here: inside a sandboxed test run it can be denied access even when the
// test owns the process, while TerminateProcess works.
func terminatePID(pid int) {
	if pid <= 0 {
		return
	}
	handle, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer func() { _ = syscall.CloseHandle(handle) }()
	_ = syscall.TerminateProcess(handle, 1)
}
