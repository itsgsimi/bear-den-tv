// Package install installs the adapter table's apps from Flathub for the
// TV's user, without root (docs/decisions/0011-per-user-flathub-installs.md,
// contracts/http.md "App installs"). Everything it runs is a fixed argv of
// the flatpak CLI with --user: the one remote (flathub, its URL a constant
// here), the ids of the adapter table only (Options.Allowed), x86_64 only,
// Flatpak's own signature checks untouched. One install runs at a time; its
// progress (state, phase, percent) is read from flatpak's output and from
// how much the disk under the user Flatpak directory has filled. Nothing
// here decides *whether* to install: the coordinator calls Start only after
// an owner press (internal/session/install.go).
package install

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"bear-den-tv/internal/applications/flatpak"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
)

const (
	// Remote is the only remote the installer adds or uses, per user.
	Remote = "flathub"
	// RemoteURL is Flathub's repo file. Compile-time: nothing else is ever
	// passed to remote-add.
	RemoteURL = "https://dl.flathub.org/repo/flathub.flatpakrepo"
	// Arch is the only architecture installs run on.
	Arch = "x86_64"
	// Margin is kept free on top of the install's own estimate.
	Margin = 512 << 20

	defaultPoll      = 2 * time.Second
	defaultKillGrace = 5 * time.Second
	infoTimeout      = 60 * time.Second
	stderrKeep       = 16 << 10
)

// Reasons shown to the owner (state.applications[].install.message).
const (
	ReasonNoFlatpak = "Flatpak isn't installed on this box"
	ReasonArch      = "App installs need an x86_64 box"
	ReasonNetwork   = "Couldn't reach Flathub. Check the TV's internet connection."
	ReasonNoSpace   = "Not enough space on this box to finish the install."
	ReasonNotFound  = "Flathub doesn't have this app for this box."
	ReasonCancelled = "Install cancelled"
	ReasonUnchecked = "Flatpak finished, but the app isn't there."
)

// Errors from Start and Cancel.
var (
	ErrNotAllowed  = errors.New("install: only the apps Bear Den knows can be installed")
	ErrUnavailable = errors.New("install: installs are not available")
	ErrBusy        = errors.New("install: another install is running")
	ErrNotRunning  = errors.New("install: nothing is installing")
)

// Status is one Flatpak's install as the coordinator reports it. The zero
// value means nothing has happened in this session.
type Status struct {
	State     string // contract.Install* ("" = no job)
	Phase     string // contract.Phase*
	Progress  int
	SizeBytes int64 // download estimate; 0 = unknown
	DiskBytes int64 // installed estimate; 0 = unknown
	Message   string
}

// Sizes is what Flathub says an install takes: the app plus its runtime
// when that is installed neither for the user nor system-wide.
type Sizes struct {
	Download int64
	Disk     int64
	// RuntimeMissing: the app's runtime is not installed anywhere yet, so
	// GL drivers and codecs come too (not listed by remote-info).
	RuntimeMissing bool
	RuntimeDisk    int64
}

// Need is how much free space an install must find: Expected plus Margin.
func (s Sizes) Need() int64 { return s.expected() + Margin }

// expected is the app plus, when its runtime is missing, the runtime twice:
// the GL drivers and codec extensions that come with it are about as big
// again and remote-info does not list them (measured in the container:
// Moonlight's app + runtime 1.1 GB, the real install 2.5 GB).
func (s Sizes) expected() int64 {
	if s.RuntimeMissing {
		return s.Disk + s.RuntimeDisk
	}
	return s.Disk
}

// Options configures New.
type Options struct {
	// Runner executes the CLI; nil uses ExecRunner.
	Runner Runner
	// Environ is filtered like the launcher's (flatpak.PassthroughEnv);
	// nil uses os.Environ(). The locale is forced to C.UTF-8 so flatpak's
	// lines parse the same everywhere.
	Environ []string
	Clock   clock.Clock
	// Allowed are the Flatpak ids that may be installed: the adapter
	// table's (adapters.Registry). Nothing else is ever passed to flatpak.
	Allowed []string
	// UserDir is the per-user installation (default
	// $XDG_DATA_HOME/flatpak, i.e. ~/.local/share/flatpak).
	UserDir string
	// FreeBytes reports the free space of the filesystem holding path;
	// nil uses statfs.
	FreeBytes func(path string) (int64, error)
	// LookPath finds the flatpak binary; nil uses exec.LookPath.
	LookPath func(string) (string, error)
	// GOARCH overrides runtime.GOARCH (tests).
	GOARCH string
	// OnChange is called after every status change (the coordinator
	// publishes a new state).
	OnChange func()
	// Poll is how often progress is sampled while flatpak runs (2 s).
	Poll time.Duration
	// KillGrace is how long a cancel waits after SIGTERM before SIGKILL.
	KillGrace time.Duration
}

// Installer runs at most one install (or update) at a time.
type Installer struct {
	run      Runner
	env      []string
	clk      clock.Clock
	allowed  map[string]bool
	userDir  string
	free     func(string) (int64, error)
	lookPath func(string) (string, error)
	goarch   string
	onChange func()
	poll     time.Duration
	grace    time.Duration

	mu     sync.Mutex
	status map[string]*Status
	sizes  map[string]Sizes
	active string             // flatpak id installing, "" when idle
	cancel context.CancelFunc // cancels the active install or update
	update bool               // the active job is an update
}

// New builds an installer.
func New(opts Options) *Installer {
	i := &Installer{
		run: opts.Runner, clk: opts.Clock, allowed: map[string]bool{},
		userDir: opts.UserDir, free: opts.FreeBytes, lookPath: opts.LookPath,
		goarch: opts.GOARCH, onChange: opts.OnChange, poll: opts.Poll, grace: opts.KillGrace,
		status: map[string]*Status{}, sizes: map[string]Sizes{},
	}
	if i.run == nil {
		i.run = ExecRunner{}
	}
	if i.clk == nil {
		i.clk = clock.Real{}
	}
	for _, id := range opts.Allowed {
		if flatpak.ValidateAppID(id) == nil {
			i.allowed[id] = true
		}
	}
	environ := opts.Environ
	if environ == nil {
		environ = os.Environ()
	}
	for _, kv := range flatpak.PassthroughEnv(environ) {
		k, _, _ := strings.Cut(kv, "=")
		if k == "LANG" || k == "LC_ALL" || k == "LC_MESSAGES" {
			continue
		}
		i.env = append(i.env, kv)
	}
	i.env = append(i.env, "LC_ALL=C.UTF-8")
	if i.userDir == "" {
		i.userDir = DefaultUserDir(environ)
	}
	if i.free == nil {
		i.free = statfsFree
	}
	if i.lookPath == nil {
		i.lookPath = exec.LookPath
	}
	if i.goarch == "" {
		i.goarch = runtime.GOARCH
	}
	if i.poll <= 0 {
		i.poll = defaultPoll
	}
	if i.grace <= 0 {
		i.grace = defaultKillGrace
	}
	return i
}

// DefaultUserDir is Flatpak's per-user installation: $XDG_DATA_HOME/flatpak,
// else ~/.local/share/flatpak.
func DefaultUserDir(environ []string) string {
	get := func(k string) string {
		for _, kv := range environ {
			if v, ok := strings.CutPrefix(kv, k+"="); ok {
				return v
			}
		}
		return ""
	}
	if d := get("XDG_DATA_HOME"); filepath.IsAbs(d) {
		return filepath.Join(d, "flatpak")
	}
	return filepath.Join(get("HOME"), ".local", "share", "flatpak")
}

// UserDir is where installs go.
func (i *Installer) UserDir() string { return i.userDir }

// Allowed reports whether id may be installed (it is in the adapter table).
func (i *Installer) Allowed(id string) bool { return i.allowed[id] }

// Available reports whether installs can run on this box, and why not.
func (i *Installer) Available() (bool, string) {
	if i.goarch != "amd64" {
		return false, ReasonArch
	}
	if _, err := i.lookPath("flatpak"); err != nil {
		return false, ReasonNoFlatpak
	}
	return true, ""
}

// Status returns id's install status (zero when nothing happened).
func (i *Installer) Status(id string) Status {
	i.mu.Lock()
	defer i.mu.Unlock()
	st := Status{}
	if s := i.status[id]; s != nil {
		st = *s
	}
	if sz, ok := i.sizes[id]; ok {
		st.SizeBytes, st.DiskBytes = sz.Download, sz.Expected()
	}
	return st
}

// Expected is the installed estimate shown to the owner: the app, plus the
// runtime when it is missing.
func (s Sizes) Expected() int64 { return s.expected() }

// Busy reports whether an install or an update is running.
func (i *Installer) Busy() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.cancel != nil
}

// Updating reports whether the running job is an update.
func (i *Installer) Updating() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.cancel != nil && i.update
}

func (i *Installer) changed() {
	if i.onChange != nil {
		i.onChange()
	}
}

func (i *Installer) set(id string, fn func(*Status)) {
	i.mu.Lock()
	s := i.status[id]
	if s == nil {
		s = &Status{}
		i.status[id] = s
	}
	before := *s
	fn(s)
	after := *s
	i.mu.Unlock()
	if before != after {
		i.changed()
	}
}

func (i *Installer) check(id string) error {
	if flatpak.ValidateAppID(id) != nil || !i.allowed[id] {
		return fmt.Errorf("%w: %q", ErrNotAllowed, id)
	}
	if ok, why := i.Available(); !ok {
		return fmt.Errorf("%w: %s", ErrUnavailable, why)
	}
	return nil
}

func (i *Installer) output(ctx context.Context, argv ...string) (Result, error) {
	return i.run.Output(ctx, Command{Argv: argv, Env: i.env})
}

// EnsureRemote adds the per-user flathub remote when it is missing
// (idempotent: --if-not-exists).
func (i *Installer) EnsureRemote(ctx context.Context) error {
	res, err := i.output(ctx, "flatpak", "remote-add", "--user", "--if-not-exists", Remote, RemoteURL)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return &Failure{Reason: Explain(string(res.Stderr)), Detail: lastLine(string(res.Stderr))}
	}
	return nil
}

// Failure is an install step that failed, with the owner's words.
type Failure struct {
	Reason string
	Detail string
}

func (f *Failure) Error() string {
	if f.Detail == "" {
		return f.Reason
	}
	return f.Reason + " (" + f.Detail + ")"
}

// Info adds the remote if needed and asks Flathub how big id's install is.
// The numbers are kept for Status.
func (i *Installer) Info(ctx context.Context, id string) (Sizes, error) {
	if err := i.check(id); err != nil {
		return Sizes{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, infoTimeout)
	defer cancel()
	if err := i.EnsureRemote(ctx); err != nil {
		return Sizes{}, err
	}
	app, err := i.remoteInfo(ctx, id)
	if err != nil {
		return Sizes{}, err
	}
	sz := Sizes{Download: app.Download, Disk: app.Installed}
	if rt := app.Runtime; rt != "" {
		installed, err := i.runtimeInstalled(ctx, rt)
		if err != nil {
			return Sizes{}, err
		}
		if !installed {
			ri, err := i.remoteInfo(ctx, "runtime/"+rt)
			if err != nil {
				return Sizes{}, err
			}
			sz.RuntimeMissing = true
			sz.Download += ri.Download
			sz.RuntimeDisk = ri.Installed
			sz.Disk += ri.Installed
		}
	}
	i.mu.Lock()
	i.sizes[id] = sz
	i.mu.Unlock()
	i.changed()
	return sz, nil
}

// RemoteInfo is the parsed subset of `flatpak remote-info`.
type RemoteInfo struct {
	Ref       string
	Arch      string
	Runtime   string // name/arch/branch, apps only
	Download  int64
	Installed int64
}

// runtimePattern is a runtime as remote-info prints it: name/x86_64/branch.
var runtimePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_\-]*(\.[A-Za-z_][A-Za-z0-9_\-]*){2,}/x86_64/[A-Za-z0-9_.\-]+$`)

// ParseRemoteInfo reads the "Key: Value" block of `flatpak remote-info`.
func ParseRemoteInfo(out []byte) RemoteInfo {
	var ri RemoteInfo
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "Ref":
			ri.Ref = value
		case "Arch":
			ri.Arch = value
		case "Runtime":
			if runtimePattern.MatchString(value) {
				ri.Runtime = value
			}
		case "Download":
			ri.Download, _ = ParseSize(value)
		case "Installed":
			ri.Installed, _ = ParseSize(value)
		}
	}
	return ri
}

// sizePattern is GLib's g_format_size: a decimal number, a (no-break)
// space — printed as "?" in a C locale — and an SI unit.
var sizePattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)[^0-9A-Za-z]*(bytes?|kB|MB|GB|TB|PB)$`)

// ParseSize turns "7.7 MB" (SI, as flatpak prints it) into bytes.
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

func (i *Installer) remoteInfo(ctx context.Context, ref string) (RemoteInfo, error) {
	res, err := i.output(ctx, "flatpak", "remote-info", "--user", Remote, ref)
	if err != nil {
		return RemoteInfo{}, err
	}
	if res.ExitCode != 0 {
		return RemoteInfo{}, &Failure{Reason: Explain(string(res.Stderr)), Detail: lastLine(string(res.Stderr))}
	}
	ri := ParseRemoteInfo(res.Stdout)
	if ri.Arch != "" && ri.Arch != Arch {
		return RemoteInfo{}, &Failure{Reason: ReasonNotFound, Detail: "arch " + ri.Arch}
	}
	return ri, nil
}

// runtimeInstalled checks the user and the system installation.
func (i *Installer) runtimeInstalled(ctx context.Context, rt string) (bool, error) {
	for _, scope := range []string{"--user", "--system"} {
		res, err := i.output(ctx, "flatpak", "info", scope, "runtime/"+rt)
		if err != nil {
			return false, err
		}
		if res.ExitCode == 0 {
			return true, nil
		}
	}
	return false, nil
}

// Installed reports whether id is installed for this user (the check after
// an install).
func (i *Installer) Installed(ctx context.Context, id string) bool {
	res, err := i.output(ctx, "flatpak", "info", "--user", id)
	return err == nil && res.ExitCode == 0
}

// Start begins installing id in the background: preparing (remote, sizes,
// free space), downloading (flatpak install), installing (the check), then
// done or failed. It returns at once.
func (i *Installer) Start(id string) error {
	if err := i.check(id); err != nil {
		return err
	}
	i.mu.Lock()
	if i.cancel != nil {
		i.mu.Unlock()
		return ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	i.active, i.cancel, i.update = id, cancel, false
	i.mu.Unlock()
	i.set(id, func(s *Status) {
		*s = Status{State: contract.InstallPreparing, Phase: contract.PhaseChecking}
	})
	go i.job(ctx, id)
	return nil
}

// Cancel stops id's running install.
func (i *Installer) Cancel(id string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.cancel == nil || i.update || i.active != id {
		return ErrNotRunning
	}
	i.cancel()
	return nil
}

// CancelUpdate stops a running update (an app is starting).
func (i *Installer) CancelUpdate() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.cancel == nil || !i.update {
		return false
	}
	i.cancel()
	return true
}

func (i *Installer) finish() {
	i.mu.Lock()
	if i.cancel != nil {
		i.cancel()
	}
	i.active, i.cancel, i.update = "", nil, false
	i.mu.Unlock()
}

func (i *Installer) fail(id string, err error, cancelled bool) {
	if cancelled {
		i.set(id, func(s *Status) {
			*s = Status{State: contract.InstallAvailable, Message: ReasonCancelled}
		})
		return
	}
	msg := err.Error()
	var f *Failure
	if errors.As(err, &f) {
		msg = f.Reason
	}
	i.set(id, func(s *Status) {
		*s = Status{State: contract.InstallFailed, Message: truncate(msg, 200)}
	})
}

func (i *Installer) job(ctx context.Context, id string) {
	defer i.finish()
	defer i.changed()
	sz, err := i.Info(ctx, id)
	if err != nil {
		i.fail(id, err, ctx.Err() != nil)
		return
	}
	free, err := i.free(existingParent(i.userDir))
	if err != nil {
		i.fail(id, fmt.Errorf("could not read the free space: %w", err), false)
		return
	}
	if free < sz.Need() {
		i.fail(id, &Failure{Reason: fmt.Sprintf("Not enough space: this needs about %s free, and the box has %s.", HumanSize(sz.Need()), HumanSize(free))}, false)
		return
	}
	i.set(id, func(s *Status) { s.State, s.Phase, s.Progress = contract.InstallDownloading, contract.PhaseRuntime, 0 })
	pr := &progress{start: free, expected: sz.expected()}
	res := i.flatpakRun(ctx, []string{"flatpak", "install", "--user", "--noninteractive", "-y", Remote, id}, func(line string) {
		if phase, ok := PhaseOf(line); ok {
			i.set(id, func(s *Status) { s.Phase = phase })
		}
	}, func() {
		if f, err := i.free(existingParent(i.userDir)); err == nil {
			p := pr.sample(f)
			i.set(id, func(s *Status) {
				if s.State == contract.InstallDownloading {
					s.Progress = p
				}
			})
		}
	})
	if ctx.Err() != nil {
		i.fail(id, errors.New(ReasonCancelled), true)
		return
	}
	if res.err != nil {
		i.fail(id, res.err, false)
		return
	}
	i.set(id, func(s *Status) { s.State, s.Phase = contract.InstallInstalling, contract.PhaseFinishing })
	if !i.Installed(ctx, id) {
		i.fail(id, &Failure{Reason: ReasonUnchecked}, false)
		return
	}
	i.set(id, func(s *Status) {
		*s = Status{State: contract.InstallDone, Progress: 100}
	})
}

// Update runs `flatpak update --user` for ids (installed for this user,
// all in the table). It blocks until done; CancelUpdate or ctx stops it.
func (i *Installer) Update(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := i.check(id); err != nil {
			return err
		}
	}
	i.mu.Lock()
	if i.cancel != nil {
		i.mu.Unlock()
		return ErrBusy
	}
	ctx, cancel := context.WithCancel(ctx)
	i.cancel, i.update = cancel, true
	i.mu.Unlock()
	defer i.finish()
	argv := append([]string{"flatpak", "update", "--user", "--noninteractive", "-y"}, ids...)
	res := i.flatpakRun(ctx, argv, nil, nil)
	if ctx.Err() != nil {
		return context.Canceled
	}
	return res.err
}

type runResult struct{ err error }

// flatpakRun starts argv, hands every stdout line to onLine, calls onTick
// every poll interval while it runs, and stops the process group when ctx
// ends (SIGTERM, then SIGKILL after the grace period).
func (i *Installer) flatpakRun(ctx context.Context, argv []string, onLine func(string), onTick func()) runResult {
	pr, pw := io.Pipe()
	stderr := &tail{max: stderrKeep}
	proc, err := i.run.Start(ctx, Command{Argv: argv, Env: i.env}, pw, stderr)
	if err != nil {
		pw.Close()
		return runResult{err: err}
	}
	lines := make(chan struct{})
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64<<10), 64<<10)
		for sc.Scan() {
			if onLine != nil {
				onLine(sc.Text())
			}
		}
		_, _ = io.Copy(io.Discard, pr)
	}()
	stopped := false
	for done := false; !done; {
		t := i.clk.NewTimer(i.poll)
		select {
		case <-proc.Done():
			done = true
		case <-ctx.Done():
			if !stopped {
				stopped = true
				_ = proc.Signal(syscall.SIGTERM)
				go func() {
					kill := i.clk.NewTimer(i.grace)
					defer kill.Stop()
					select {
					case <-proc.Done():
					case <-kill.C():
						_ = proc.Signal(syscall.SIGKILL)
					}
				}()
			}
			<-proc.Done()
			done = true
		case <-t.C():
			if onTick != nil {
				onTick()
			}
		}
		t.Stop()
	}
	werr := proc.Wait()
	pw.Close()
	<-lines
	if werr != nil {
		msg := stderr.String()
		return runResult{err: &Failure{Reason: Explain(msg), Detail: lastLine(msg)}}
	}
	return runResult{}
}

// PhaseOf reads one line of flatpak's quiet (--noninteractive) output: it
// prints "Installing <ref>" (or "Updating <ref>") when it starts each
// operation, runtimes and extensions first, the app last.
func PhaseOf(line string) (string, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\x1b[?25h"))
	for _, verb := range []string{"Installing ", "Updating "} {
		if ref, ok := strings.CutPrefix(line, verb); ok {
			switch {
			case strings.HasPrefix(ref, "app/"):
				return contract.PhaseApp, true
			case strings.HasPrefix(ref, "runtime/"):
				return contract.PhaseRuntime, true
			}
		}
	}
	return "", false
}

// progress turns free-space samples into a percentage that never goes
// backwards and stays below 100 until the install is checked.
type progress struct {
	start    int64
	expected int64
	last     int
}

func (p *progress) sample(free int64) int {
	used := p.start - free
	pct := 0
	if p.expected > 0 && used > 0 {
		pct = int(used * 100 / p.expected)
	}
	if pct > 99 {
		pct = 99
	}
	if pct > p.last {
		p.last = pct
	}
	return p.last
}

// Explain turns flatpak's error output into the owner's words.
func Explain(stderr string) string {
	s := strings.ToLower(stderr)
	switch {
	case containsAny(s, "no space left", "free space", "not enough disk space"):
		return ReasonNoSpace
	case containsAny(s, "couldn't resolve", "could not resolve", "unable to connect", "could not connect to", "failed to connect",
		"network is unreachable", "timeout was reached", "temporary failure in name resolution", "while fetching"):
		return ReasonNetwork
	case containsAny(s, "can't find ref", "nothing matches", "no remote refs found"):
		return ReasonNotFound
	}
	if l := lastLine(stderr); l != "" {
		return truncate("Flatpak said: "+l, 200)
	}
	return "Flatpak stopped without saying why."
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// lastLine is the last error line flatpak printed (container-only bwrap
// noise and warnings about optional extensions skipped when a real error
// follows).
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for j := len(lines) - 1; j >= 0; j-- {
		l := strings.TrimSpace(lines[j])
		if l == "" || strings.HasPrefix(l, "bwrap:") {
			continue
		}
		for _, p := range []string{"error: ", "Error: "} {
			l = strings.TrimPrefix(l, p)
		}
		return l
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// HumanSize prints bytes the way the TV shows them ("1.2 GB", "420 MB").
func HumanSize(b int64) string {
	switch {
	case b >= 1e9:
		return strconv.FormatFloat(float64(b)/1e9, 'f', 1, 64) + " GB"
	case b >= 1e6:
		return strconv.FormatInt((b+5e5)/1e6, 10) + " MB"
	default:
		return strconv.FormatInt((b+500)/1e3, 10) + " kB"
	}
}

// existingParent is path or its nearest existing ancestor (the user
// directory may not exist before the first install).
func existingParent(path string) string {
	for p := path; ; p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil || p == filepath.Dir(p) {
			return p
		}
	}
}

func statfsFree(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

// tail keeps the last max bytes written (flatpak's stderr).
type tail struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
