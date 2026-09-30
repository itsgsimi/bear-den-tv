// Tests for daemon.go: `session` ignores the terminal job-control signals.
// A helper copy of this test binary runs in its own process group of this
// session (not orphaned), where the kernel's default action for SIGTSTP,
// SIGTTIN and SIGTTOU is to stop it.

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const helperEnv = "BDTV_TEST_JOBCONTROL_HELPER"

// TestJobControlHelper is not a test: run by startHelper, it applies
// ignoreJobControl (unless told not to) and echoes each stdin line.
func TestJobControlHelper(t *testing.T) {
	mode := os.Getenv(helperEnv)
	if mode == "" {
		return // only meaningful as the helper process
	}
	if mode == "ignore" {
		ignoreJobControl()
	}
	fmt.Println("ready")
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		fmt.Println("echo " + in.Text())
	}
	os.Exit(0)
}

type helper struct {
	cmd   *exec.Cmd
	in    *os.File
	lines chan string
}

func startHelper(t *testing.T, mode string) *helper {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestJobControlHelper$")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own group, same session: not orphaned
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdin, cmd.Stdout = inR, outW
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	inR.Close()
	outW.Close()
	h := &helper{cmd: cmd, in: inW, lines: make(chan string, 16)}
	go func() {
		s := bufio.NewScanner(outR)
		for s.Scan() {
			h.lines <- s.Text()
		}
		close(h.lines)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGCONT)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		inW.Close()
	})
	if got := h.next(t, 10*time.Second); got != "ready" {
		t.Fatalf("helper said %q, want ready", got)
	}
	return h
}

func (h *helper) next(t *testing.T, within time.Duration) string {
	t.Helper()
	select {
	case l, ok := <-h.lines:
		if !ok {
			t.Fatal("helper ended")
		}
		return l
	case <-time.After(within):
		return ""
	}
}

// state is the one-letter process state from /proc/<pid>/stat.
func procState(pid int) string {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	s := string(raw)
	s = s[strings.LastIndex(s, ")")+2:]
	return s[:1]
}

// The control: without ignoreJobControl, SIGTSTP stops the helper (state T),
// so the helper's setup is one where these signals do stop a process.
func TestJobControlStopsWithoutIgnore(t *testing.T) {
	h := startHelper(t, "default")
	_ = syscall.Kill(h.cmd.Process.Pid, syscall.SIGTSTP)
	deadline := time.Now().Add(10 * time.Second)
	for procState(h.cmd.Process.Pid) != "T" {
		if time.Now().After(deadline) {
			t.Fatalf("SIGTSTP did not stop the control helper (state %q)", procState(h.cmd.Process.Pid))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// With ignoreJobControl (what `session` calls), none of the three stops it:
// a line written after the signal is sent is answered. The kernel acts on a
// pending stop signal before the helper's read returns, so an answer proves
// the signal was ignored, not merely late.
func TestSessionIgnoresJobControlSignals(t *testing.T) {
	h := startHelper(t, "ignore")
	pid := h.cmd.Process.Pid
	for _, sig := range jobControlSignals {
		if err := syscall.Kill(pid, sig); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintln(h.in, unix.SignalName(sig)); err != nil {
			t.Fatal(err)
		}
		if got := h.next(t, 10*time.Second); got != "echo "+unix.SignalName(sig) {
			t.Fatalf("after %s the helper did not answer (got %q, state %q): it was stopped", unix.SignalName(sig), got, procState(pid))
		}
	}
	if st := procState(pid); st == "T" {
		t.Fatalf("helper is stopped")
	}
}
