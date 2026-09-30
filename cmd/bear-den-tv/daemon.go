// `bear-den-tv session` as a daemon: the terminal job-control signals
// (SIGTSTP, SIGTTIN, SIGTTOU) are ignored, so a coordinator that ends up tied
// to a terminal (started by hand, or by an older start script) can never be
// stopped by one and freeze the TV (docs/operations.md → The watchdog;
// the incident in docs/IMPLEMENTATION_STATUS.md). SIGSTOP cannot be ignored;
// the watchdog in scripts/start-session.sh restarts a stopped coordinator.

package main

import (
	"os/signal"
	"syscall"
)

// jobControlSignals are ignored by `session` (not by `dev`, which is run from
// a terminal where Ctrl+Z is expected to work).
var jobControlSignals = []syscall.Signal{syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU}

// ignoreJobControl sets the job-control signals to ignored (SIG_IGN). Ignored,
// not caught: a background read or write on a terminal then fails with EIO
// instead of being retried forever.
func ignoreJobControl() {
	for _, s := range jobControlSignals {
		signal.Ignore(s)
	}
}
