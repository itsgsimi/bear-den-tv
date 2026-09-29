// Runner: the fixed-argv process seam the installer uses (like the Flatpak
// launcher's and the audio backend's), with an exec implementation whose
// started processes can be stopped as a whole process group (cancel).
// Spec: docs/decisions/0011-per-user-flathub-installs.md.

package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"
)

// Command is one fixed-argv invocation of the flatpak CLI with its full
// environment.
type Command struct {
	Argv []string
	Env  []string
}

// Result is what a finished command produced. ExitCode is non-zero for a
// failed command; Runner.Output's error is only for commands that could not
// run at all (or whose context ended).
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Process is a started command.
type Process interface {
	// Done is closed once the process has exited.
	Done() <-chan struct{}
	// Wait blocks until exit and returns the exit error, if any.
	Wait() error
	// Signal sends sig to the process's whole process group.
	Signal(sig syscall.Signal) error
}

// Runner executes commands; ExecRunner is the real one, tests inject fakes.
type Runner interface {
	Output(ctx context.Context, cmd Command) (Result, error)
	// Start runs cmd in its own process group, streaming its output.
	Start(ctx context.Context, cmd Command, stdout, stderr io.Writer) (Process, error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// Output implements Runner.
func (ExecRunner) Output(ctx context.Context, cmd Command) (Result, error) {
	c := exec.CommandContext(ctx, cmd.Argv[0], cmd.Argv[1:]...)
	c.Env = cmd.Env
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return res, nil
	case ctx.Err() != nil:
		return res, fmt.Errorf("%s: %w", cmd.Argv[0], ctx.Err())
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	default:
		return res, fmt.Errorf("%s: %w", cmd.Argv[0], err)
	}
}

// Start implements Runner. The child leads its own process group so a
// cancel reaches flatpak and every helper it started, and nothing aimed at
// the coordinator's group reaches it.
func (ExecRunner) Start(ctx context.Context, cmd Command, stdout, stderr io.Writer) (Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := exec.Command(cmd.Argv[0], cmd.Argv[1:]...)
	c.Env = cmd.Env
	c.Stdout, c.Stderr = stdout, stderr
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("%s: %w", cmd.Argv[0], err)
	}
	p := &execProcess{pid: c.Process.Pid, done: make(chan struct{})}
	go func() {
		p.err = c.Wait()
		close(p.done)
	}()
	return p, nil
}

type execProcess struct {
	pid  int
	done chan struct{}
	err  error
}

func (p *execProcess) Done() <-chan struct{} { return p.done }
func (p *execProcess) Wait() error {
	<-p.done
	return p.err
}
func (p *execProcess) Signal(sig syscall.Signal) error {
	select {
	case <-p.done:
		return nil
	default:
	}
	return syscall.Kill(-p.pid, sig)
}
