//go:build linux

// Linux process attributes for the supervised shell.

package shellipc

import "syscall"

// shellSysProcAttr starts the shell in its own process group and asks the
// kernel to SIGTERM it if the coordinator dies (even by SIGKILL), so a crashed
// coordinator never leaves an orphaned full-screen shell behind for its
// restarted successor to collide with. Pdeathsig fires when the spawning OS
// thread exits; the Go runtime keeps those threads for the process lifetime.
func shellSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
}
