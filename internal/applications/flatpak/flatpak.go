// Package flatpak is the applications.Launcher for Flatpak applications. Every
// invocation is a fixed argument vector (`flatpak info|run|ps|kill`) built from
// a validated application id, the adapter's approved launch arguments, or a
// numeric instance id; nothing from the network reaches argv. Kill never names
// an application: an unknown instance id is resolved to the single running
// instance of that application id or refused.
package flatpak

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bear-den-tv/internal/applications"
)

const (
	// Remote is the only remote InstallCommand references.
	Remote = "flathub"
	// RingSize bounds each captured output stream per launched process.
	RingSize = 64 * 1024

	defaultInstanceWait = 3 * time.Second
	instancePoll        = 100 * time.Millisecond
)

// PassthroughKeys is the closed list of environment variables handed to
// `flatpak run`: what the CLI needs to find the session and its own binaries.
var PassthroughKeys = []string{
	"HOME", "USER", "LOGNAME", "PATH", "LANG", "LC_ALL", "LC_MESSAGES", "TZ",
	"DISPLAY", "XAUTHORITY", "WAYLAND_DISPLAY",
	"XDG_SESSION_TYPE", "XDG_SESSION_ID", "XDG_RUNTIME_DIR", "XDG_SEAT", "XDG_VTNR",
	"XDG_DATA_DIRS", "XDG_CONFIG_DIRS", "XDG_CURRENT_DESKTOP", "DESKTOP_SESSION",
	"DBUS_SESSION_BUS_ADDRESS",
}

// appIDPattern is Flatpak's application id shape: at least three dot-separated
// components of [A-Za-z0-9_-], the first of each not starting with a digit.
var appIDPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_\-]*(\.[A-Za-z_][A-Za-z0-9_\-]*){2,}$`)

// ErrInvalidAppID is returned for ids that do not match Flatpak's id grammar.
var ErrInvalidAppID = errors.New("flatpak: invalid application id")

// ErrAmbiguousInstance is returned by Kill when the instance cannot be pinned
// to exactly one running instance of the application.
var ErrAmbiguousInstance = errors.New("flatpak: cannot identify a single instance; refusing to kill by name")

// ValidateAppID checks id against Flatpak's id grammar.
func ValidateAppID(id string) error {
	if !appIDPattern.MatchString(id) || len(id) > 255 {
		return fmt.Errorf("%w: %q", ErrInvalidAppID, id)
	}
	return nil
}

// PassthroughEnv keeps only PassthroughKeys from an os.Environ-shaped list.
func PassthroughEnv(environ []string) []string {
	allowed := map[string]bool{}
	for _, k := range PassthroughKeys {
		allowed[k] = true
	}
	var out []string
	for _, kv := range environ {
		k, _, ok := strings.Cut(kv, "=")
		if ok && allowed[k] {
			out = append(out, kv)
		}
	}
	sort.Strings(out)
	return out
}

// InstallCommand is the argv a person could run to install id from flathub
// for the current user. It is never executed by the coordinator.
func InstallCommand(id string) []string {
	return []string{"flatpak", "install", "--user", Remote, id}
}

// InfoFields is the parsed subset of `flatpak info`.
type InfoFields struct {
	ID           string `json:"id"`
	Ref          string `json:"ref"`
	Version      string `json:"version"`
	Origin       string `json:"origin"`
	Installation string `json:"installation"`
	Runtime      string `json:"runtime"`
	// Installed is the app's own size as printed ("18.6 MB"); InstalledBytes
	// is it in bytes (0 when absent or unreadable).
	Installed      string `json:"installed"`
	InstalledBytes int64  `json:"installed_bytes"`
}

// ParseInfo reads the "Key: Value" block of `flatpak info` output.
func ParseInfo(out []byte) InfoFields {
	var f InfoFields
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "ID":
			f.ID = value
		case "Ref":
			f.Ref = value
		case "Version":
			f.Version = value
		case "Origin":
			f.Origin = value
		case "Installation":
			f.Installation = value
		case "Runtime":
			f.Runtime = value
		case "Installed":
			f.Installed = value
			f.InstalledBytes, _ = ParseSize(value)
		}
	}
	return f
}

var sizePattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)[^0-9A-Za-z]*(bytes?|kB|MB|GB|TB|PB)$`)

// ParseSize turns a size as flatpak prints it ("7.7 MB", SI units, with a
// no-break space in `flatpak info`) into bytes.
func ParseSize(s string) (int64, bool) {
	m := sizePattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	mult := map[string]float64{"byte": 1, "bytes": 1, "kB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12, "PB": 1e15}[m[2]]
	return int64(v * mult), true
}

// ParsePS reads `flatpak ps --columns=instance,pid,application` output in
// either the tab-separated non-tty form or the aligned tty form with a header.
func ParsePS(out []byte) []applications.Instance {
	var insts []applications.Instance
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil || !isNumeric(fields[0]) {
			continue
		}
		insts = append(insts, applications.Instance{InstanceID: fields[0], PID: pid, FlatpakID: fields[2]})
	}
	return insts
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Options configures New.
type Options struct {
	// Runner executes the CLI; nil uses ExecRunner.
	Runner Runner
	// Environ is the process environment to filter; nil uses os.Environ().
	Environ []string
	// InstanceWait bounds how long Launch polls `flatpak ps` for the new
	// instance id; zero uses 3 s.
	InstanceWait time.Duration
}

// Launcher implements applications.Launcher over the flatpak CLI.
type Launcher struct {
	run  Runner
	env  []string
	wait time.Duration

	mu       sync.Mutex
	launched map[int]*launched
}

type launched struct {
	proc   Process
	stdout *Ring
	stderr *Ring
}

// New builds a launcher.
func New(opts Options) *Launcher {
	l := &Launcher{run: opts.Runner, wait: opts.InstanceWait, launched: map[int]*launched{}}
	if l.run == nil {
		l.run = ExecRunner{}
	}
	environ := opts.Environ
	if environ == nil {
		environ = os.Environ()
	}
	l.env = PassthroughEnv(environ)
	if l.wait == 0 {
		l.wait = defaultInstanceWait
	}
	return l
}

func (l *Launcher) output(ctx context.Context, argv ...string) (Result, error) {
	return l.run.Output(ctx, Command{Argv: argv, Env: l.env})
}

// Version returns `flatpak --version` for diagnostics.
func (l *Launcher) Version(ctx context.Context) (string, error) {
	res, err := l.output(ctx, "flatpak", "--version")
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("flatpak --version exited %d: %s", res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// Discover implements applications.Launcher: `flatpak info --user <id>` then
// `--system`; Scope is user, system, or none.
func (l *Launcher) Discover(ctx context.Context, flatpakID string) (applications.Installation, error) {
	if err := ValidateAppID(flatpakID); err != nil {
		return applications.Installation{Scope: "unknown"}, err
	}
	var lastErr string
	for _, scope := range []string{"user", "system"} {
		res, err := l.output(ctx, "flatpak", "info", "--"+scope, flatpakID)
		if err != nil {
			return applications.Installation{Scope: "unknown"}, err
		}
		if res.ExitCode == 0 {
			f := ParseInfo(res.Stdout)
			if f.Installation != "" {
				scope = f.Installation
			}
			return applications.Installation{Installed: true, Version: f.Version, Scope: scope, SizeBytes: f.InstalledBytes}, nil
		}
		lastErr = strings.TrimSpace(string(res.Stderr))
		if !strings.Contains(lastErr, "not installed") {
			return applications.Installation{Scope: "unknown"}, fmt.Errorf("flatpak info --%s %s exited %d: %s", scope, flatpakID, res.ExitCode, lastErr)
		}
	}
	return applications.Installation{Scope: "none"}, nil
}

// Instances implements applications.Launcher via `flatpak ps`.
func (l *Launcher) Instances(ctx context.Context) ([]applications.Instance, error) {
	res, err := l.output(ctx, "flatpak", "ps", "--columns=instance,pid,application")
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("flatpak ps exited %d: %s", res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	return ParsePS(res.Stdout), nil
}

// Launch implements applications.Launcher: `flatpak run <id> <args...>` in its
// own session with both output streams captured into bounded rings, then the
// instance id is resolved from `flatpak ps` by the child's pid or, failing
// that, by being the one instance of id that did not exist before the launch.
// A process that exits before an instance appears is a launch failure carrying
// the stderr tail. An instance id may stay empty when the CLI has not listed
// the instance yet; callers re-resolve through Instances.
func (l *Launcher) Launch(ctx context.Context, flatpakID string, args []string) (applications.Instance, error) {
	if err := ValidateAppID(flatpakID); err != nil {
		return applications.Instance{}, err
	}
	for _, a := range args {
		if a == "" || strings.ContainsAny(a, "\x00\n\r") {
			return applications.Instance{}, fmt.Errorf("flatpak: invalid launch argument %q", a)
		}
	}
	before := map[string]bool{}
	if existing, err := l.Instances(ctx); err == nil {
		for _, e := range existing {
			if e.FlatpakID == flatpakID {
				before[e.InstanceID] = true
			}
		}
	}
	argv := append([]string{"flatpak", "run", flatpakID}, args...)
	stdout, stderr := NewRing(RingSize), NewRing(RingSize)
	proc, err := l.run.Start(ctx, Command{Argv: argv, Env: l.env}, stdout, stderr)
	if err != nil {
		return applications.Instance{}, fmt.Errorf("flatpak run %s: %w", flatpakID, err)
	}
	l.mu.Lock()
	l.launched[proc.PID()] = &launched{proc: proc, stdout: stdout, stderr: stderr}
	l.mu.Unlock()
	inst := applications.Instance{FlatpakID: flatpakID, PID: proc.PID()}
	deadline := time.Now().Add(l.wait)
	for {
		select {
		case <-proc.Done():
			return inst, fmt.Errorf("flatpak run %s exited before an instance appeared (%v): %s", flatpakID, proc.Wait(), strings.TrimSpace(stderr.Tail()))
		case <-ctx.Done():
			return inst, nil
		default:
		}
		if found, ok := l.resolveNew(ctx, flatpakID, proc.PID(), before); ok {
			return found, nil
		}
		if time.Now().After(deadline) {
			return inst, nil
		}
		time.Sleep(instancePoll)
	}
}

func (l *Launcher) resolveNew(ctx context.Context, flatpakID string, pid int, before map[string]bool) (applications.Instance, bool) {
	insts, err := l.Instances(ctx)
	if err != nil {
		return applications.Instance{}, false
	}
	var fresh []applications.Instance
	for _, i := range insts {
		if i.FlatpakID != flatpakID {
			continue
		}
		if i.PID == pid {
			return i, true
		}
		if !before[i.InstanceID] {
			fresh = append(fresh, i)
		}
	}
	if len(fresh) == 1 {
		return fresh[0], true
	}
	return applications.Instance{}, false
}

// Output returns the captured stdout/stderr tails of a process this launcher
// started, for crash feedback.
func (l *Launcher) Output(pid int) (stdout, stderr string, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.launched[pid]
	if !ok {
		return "", "", false
	}
	return entry.stdout.Tail(), entry.stderr.Tail(), true
}

// Kill implements applications.Launcher with `flatpak kill <instance-id>`.
// When inst carries no instance id it is resolved to the only running
// instance of inst.FlatpakID (whose pid must agree when inst.PID is set);
// anything else is refused with ErrAmbiguousInstance. The application id is
// never passed to `flatpak kill`.
func (l *Launcher) Kill(ctx context.Context, inst applications.Instance) error {
	id := inst.InstanceID
	if id == "" {
		if err := ValidateAppID(inst.FlatpakID); err != nil {
			return err
		}
		insts, err := l.Instances(ctx)
		if err != nil {
			return err
		}
		var matching []applications.Instance
		for _, i := range insts {
			if i.FlatpakID == inst.FlatpakID {
				matching = append(matching, i)
			}
		}
		if len(matching) != 1 {
			return fmt.Errorf("%w: %d running instances of %s", ErrAmbiguousInstance, len(matching), inst.FlatpakID)
		}
		if inst.PID != 0 && matching[0].PID != inst.PID {
			return fmt.Errorf("%w: instance pid %d differs from tracked pid %d", ErrAmbiguousInstance, matching[0].PID, inst.PID)
		}
		id = matching[0].InstanceID
	}
	if !isNumeric(id) {
		return fmt.Errorf("flatpak: instance id %q is not numeric; refusing", id)
	}
	res, err := l.output(ctx, "flatpak", "kill", id)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("flatpak kill %s exited %d: %s", id, res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}
