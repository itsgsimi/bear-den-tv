// Supervisor: starts, restarts and stops the TV shell process.

package shellipc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"bear-den-tv/internal/clock"
)

// Shell states reported in state.session.shell_state.
const (
	StateStopped     = "stopped"
	StateStarting    = "starting"
	StateRunning     = "running"
	StateCrashed     = "crashed"
	StateRestarting  = "restarting"
	StateCircuitOpen = "circuit_open"
	StateExited      = "exited"
)

// DefaultBackoff is the restart schedule after crashes; the last value repeats.
var DefaultBackoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}

// Circuit breaker: this many crashes within the window opens the circuit.
const (
	DefaultCircuitCrashes = 5
	DefaultCircuitWindow  = 5 * time.Minute
)

// envPassthrough lists the environment the shell inherits; everything else
// from the coordinator's environment is dropped. BDTV_THEMES_DIR must reach
// the shell so the TV and the phones load themes from the same folder.
var envPassthrough = []string{"DISPLAY", "XAUTHORITY", "WAYLAND_DISPLAY", "DBUS_SESSION_BUS_ADDRESS", "HOME", "PATH", "LANG", "BDTV_FPS_LOG", "BDTV_SCREENSAVER_SECONDS", "BDTV_BEARS_SECONDS", "BDTV_BEARS_ACT", "BDTV_REST_SECONDS", "BDTV_THEMES_DIR"}

// ShellEnvironment filters base (KEY=VALUE pairs) to the allowed names and
// prefixes (XDG_*, QT_*) and appends extra.
func ShellEnvironment(base []string, extra map[string]string) []string {
	out := make([]string, 0, len(base)+len(extra))
	for _, kv := range base {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, override := extra[name]; override {
			continue
		}
		allowed := strings.HasPrefix(name, "XDG_") || strings.HasPrefix(name, "QT_")
		for _, n := range envPassthrough {
			if name == n {
				allowed = true
			}
		}
		if allowed {
			out = append(out, kv)
		}
	}
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}

// SupervisorOptions configures a Supervisor.
type SupervisorOptions struct {
	Binary string
	Args   []string
	// Env is the full environment for the shell (see ShellEnvironment).
	Env    []string
	Clock  clock.Clock
	Logger *slog.Logger
	Stdout io.Writer
	Stderr io.Writer
	// OnState is called on every state change, outside the supervisor lock.
	OnState        func(state string)
	Backoff        []time.Duration
	CircuitCrashes int
	CircuitWindow  time.Duration
	// KillGrace is how long a hung shell gets after SIGTERM before SIGKILL.
	KillGrace time.Duration
}

// Supervisor starts the shell binary and restarts it after crashes with
// bounded backoff and a circuit breaker. It never signals any process other
// than the one it started.
type Supervisor struct {
	opts SupervisorOptions

	mu          sync.Mutex
	state       string
	cmd         *exec.Cmd
	intentional bool
	stopping    bool
	crashes     []time.Time
	consecutive int
	restart     clock.Timer
	exited      chan struct{}
}

// NewSupervisor prepares a supervisor; nothing runs until Start.
func NewSupervisor(opts SupervisorOptions) *Supervisor {
	if opts.Clock == nil {
		opts.Clock = clock.Real{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if len(opts.Backoff) == 0 {
		opts.Backoff = DefaultBackoff
	}
	if opts.CircuitCrashes <= 0 {
		opts.CircuitCrashes = DefaultCircuitCrashes
	}
	if opts.CircuitWindow <= 0 {
		opts.CircuitWindow = DefaultCircuitWindow
	}
	if opts.KillGrace <= 0 {
		opts.KillGrace = 3 * time.Second
	}
	return &Supervisor{opts: opts, state: StateStopped}
}

// State returns the current shell state.
func (s *Supervisor) State() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// PID returns the running shell pid or 0.
func (s *Supervisor) PID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	return 0
}

func (s *Supervisor) setState(st string) {
	s.state = st
	if s.opts.OnState != nil {
		go s.opts.OnState(st)
	}
}

// Start launches the shell. It is a no-op while a shell process is alive.
func (s *Supervisor) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startLocked()
}

func (s *Supervisor) startLocked() error {
	if s.cmd != nil {
		return nil
	}
	if s.restart != nil {
		s.restart.Stop()
		s.restart = nil
	}
	cmd := exec.Command(s.opts.Binary, s.opts.Args...)
	cmd.Env = s.opts.Env
	cmd.Stdout = s.opts.Stdout
	cmd.Stderr = s.opts.Stderr
	cmd.SysProcAttr = shellSysProcAttr()
	if err := cmd.Start(); err != nil {
		s.opts.Logger.Error("shell: start failed", "error", err.Error())
		s.setState(StateCrashed)
		return err
	}
	s.cmd = cmd
	s.intentional = false
	s.exited = make(chan struct{})
	s.setState(StateStarting)
	s.opts.Logger.Info("shell: started", "pid", cmd.Process.Pid)
	exited := s.exited
	go s.wait(cmd, exited)
	return nil
}

func (s *Supervisor) wait(cmd *exec.Cmd, exited chan struct{}) {
	err := cmd.Wait()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	s.mu.Lock()
	s.cmd = nil
	close(exited)
	intentional := s.intentional || s.stopping
	if intentional && code == 0 {
		s.opts.Logger.Info("shell: exited intentionally")
		s.setState(StateExited)
		s.mu.Unlock()
		return
	}
	if s.stopping {
		s.opts.Logger.Warn("shell: exited during shutdown", "code", code)
		s.setState(StateExited)
		s.mu.Unlock()
		return
	}
	now := s.opts.Clock.Now()
	s.crashes = append(s.crashes, now)
	kept := s.crashes[:0]
	for _, t := range s.crashes {
		if now.Sub(t) <= s.opts.CircuitWindow {
			kept = append(kept, t)
		}
	}
	s.crashes = kept
	s.consecutive++
	s.opts.Logger.Error("shell: crashed", "code", code, "recent_crashes", len(s.crashes))
	if len(s.crashes) >= s.opts.CircuitCrashes {
		s.setState(StateCircuitOpen)
		s.mu.Unlock()
		return
	}
	s.setState(StateCrashed)
	idx := s.consecutive - 1
	if idx >= len(s.opts.Backoff) {
		idx = len(s.opts.Backoff) - 1
	}
	delay := s.opts.Backoff[idx]
	s.setState(StateRestarting)
	s.restart = s.opts.Clock.AfterFunc(delay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.stopping || s.state != StateRestarting {
			return
		}
		_ = s.startLocked()
	})
	s.mu.Unlock()
}

// MarkRunning records that the shell completed the IPC handshake.
func (s *Supervisor) MarkRunning() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil {
		s.consecutive = 0
		s.setState(StateRunning)
	}
}

// MarkIntentionalExit records shell.exit or a sent shutdown, so a following
// exit code 0 is not treated as a crash.
func (s *Supervisor) MarkIntentionalExit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.intentional = true
}

// SocketLost handles a dropped IPC connection: if the process is still alive
// and did not announce an exit, it is terminated so the exit path decides on
// a restart. Nothing else is signalled.
func (s *Supervisor) SocketLost() {
	s.mu.Lock()
	cmd := s.cmd
	intentional := s.intentional || s.stopping
	s.mu.Unlock()
	if cmd == nil || intentional {
		return
	}
	s.opts.Logger.Warn("shell: socket lost without shell.exit; terminating", "pid", cmd.Process.Pid)
	s.terminate(cmd)
}

func (s *Supervisor) terminate(cmd *exec.Cmd) {
	s.mu.Lock()
	exited := s.exited
	s.mu.Unlock()
	_ = cmd.Process.Signal(syscall.SIGTERM)
	timer := s.opts.Clock.NewTimer(s.opts.KillGrace)
	defer timer.Stop()
	select {
	case <-exited:
	case <-timer.C():
		_ = cmd.Process.Kill()
	}
}

// Restart is the owner recovery path: it resets the circuit breaker and
// crash history and starts the shell if it is not running.
func (s *Supervisor) Restart() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.crashes = nil
	s.consecutive = 0
	if s.cmd != nil {
		return nil
	}
	return s.startLocked()
}

// Stop ends supervision: no further restarts. If the shell is alive, the
// caller should have sent shutdown; Stop waits up to ctx for the exit, then
// terminates the process.
func (s *Supervisor) Stop(ctx context.Context) {
	s.mu.Lock()
	s.stopping = true
	if s.restart != nil {
		s.restart.Stop()
		s.restart = nil
	}
	cmd := s.cmd
	exited := s.exited
	if cmd == nil {
		if s.state != StateExited {
			s.setState(StateStopped)
		}
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	select {
	case <-exited:
		return
	case <-ctx.Done():
	}
	s.terminate(cmd)
	<-exited
}

// Exited returns a channel closed when the current process exits (nil when none).
func (s *Supervisor) Exited() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exited
}

// LookupBinary resolves the shell binary: an explicit path, else
// bear-den-tv-shell beside the running executable, else on PATH.
func LookupBinary(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if exe, err := os.Executable(); err == nil {
		candidate := exe[:strings.LastIndex(exe, "/")+1] + "bear-den-tv-shell"
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
	}
	return exec.LookPath("bear-den-tv-shell")
}
