// Runner: the fixed-argv process seam the Flatpak launcher uses, with an exec
// implementation.

package flatpak

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// Command is one fixed-argv invocation of the flatpak CLI. Env is the full
// environment for the child (already filtered by PassthroughEnv).
type Command struct {
	Argv []string
	Env  []string
	// ExtraFiles are inherited by a started child as fds 3, 4, ... (the
	// DevTools pipe of a web app, internal/applications/web). `flatpak run`
	// in the foreground execs bubblewrap without closing inherited fds.
	ExtraFiles []*os.File
}

// Result is what a completed command produced. ExitCode is non-zero for a
// failed command; the error return of Runner.Output is reserved for commands
// that could not run at all.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Process is a started, detached command.
type Process interface {
	PID() int
	// Done is closed once the process has exited.
	Done() <-chan struct{}
	// Wait blocks until exit and returns the exit error, if any.
	Wait() error
}

// Runner executes commands; ExecRunner is the real one, tests inject fakes.
type Runner interface {
	// Output runs cmd to completion under ctx.
	Output(ctx context.Context, cmd Command) (Result, error)
	// Start launches cmd in its own session (setsid) with the given output
	// sinks. The process must outlive ctx: ctx only bounds the spawn.
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
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
		if ctx.Err() != nil {
			return res, fmt.Errorf("%s: %w", cmd.Argv[0], ctx.Err())
		}
		return res, nil
	default:
		return res, fmt.Errorf("%s: %w", cmd.Argv[0], err)
	}
}

// Start implements Runner. The child gets its own session so signals aimed at
// the coordinator never reach the application, and its exit is reaped on a
// goroutine so no zombie lingers.
func (ExecRunner) Start(ctx context.Context, cmd Command, stdout, stderr io.Writer) (Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := exec.Command(cmd.Argv[0], cmd.Argv[1:]...)
	c.Env = cmd.Env
	c.Stdout, c.Stderr = stdout, stderr
	c.ExtraFiles = cmd.ExtraFiles
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
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
	once sync.Once
}

func (p *execProcess) PID() int              { return p.pid }
func (p *execProcess) Done() <-chan struct{} { return p.done }
func (p *execProcess) Wait() error {
	<-p.done
	return p.err
}

// Ring is a bounded io.Writer that keeps the last Size bytes written; the
// launcher attaches one per stream so a chatty application cannot grow the
// coordinator's memory.
type Ring struct {
	mu   sync.Mutex
	buf  []byte
	size int
	full bool
	pos  int
}

// NewRing keeps the last size bytes.
func NewRing(size int) *Ring {
	return &Ring{buf: make([]byte, size), size: size}
}

// Write implements io.Writer and never fails.
func (r *Ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	if n >= r.size {
		copy(r.buf, p[n-r.size:])
		r.pos, r.full = 0, true
		return n, nil
	}
	first := copy(r.buf[r.pos:], p)
	if first < n {
		copy(r.buf, p[first:])
		r.full = true
	}
	r.pos = (r.pos + n) % r.size
	if r.pos == 0 && n > 0 {
		r.full = true
	}
	return n, nil
}

// Tail returns the retained bytes in write order.
func (r *Ring) Tail() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return string(r.buf[:r.pos])
	}
	return string(r.buf[r.pos:]) + string(r.buf[:r.pos])
}
