//go:build !linux

// Process attributes for the supervised shell on non-Linux builds.

package shellipc

import "syscall"

// shellSysProcAttr starts the shell in its own process group.
func shellSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
