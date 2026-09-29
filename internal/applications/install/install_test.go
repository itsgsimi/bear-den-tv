// Tests for the installer (install.go) against a fake flatpak CLI fed with
// flatpak's real output, captured in an Ubuntu 24.04 container as a
// non-root user (testdata/, see testdata/README.md): exact argv (no remote,
// ref or flag but the fixed ones, never anything that weakens signature
// checks), progress and phases, cancel, disk full, network failure, the
// adapter table as the only source of ids, updates.

package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/contract"
)

const moonlight = adapters.MoonlightFlatpakID

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeProc is a started `flatpak install|update`: the test writes its
// output lines and decides when it exits.
type fakeProc struct {
	stdout, stderr io.Writer
	done           chan struct{}
	once           sync.Once
	err            error
	mu             sync.Mutex
	signals        []syscall.Signal
	ignoreTerm     bool
}

func (p *fakeProc) Done() <-chan struct{} { return p.done }
func (p *fakeProc) Wait() error           { <-p.done; return p.err }
func (p *fakeProc) Signal(sig syscall.Signal) error {
	p.mu.Lock()
	p.signals = append(p.signals, sig)
	ignore := p.ignoreTerm && sig == syscall.SIGTERM
	p.mu.Unlock()
	if !ignore {
		// Like the real run: SIGTERM ends it with status 1 and the cursor
		// escape it prints on the way out (testdata/install-cancel.stdout).
		_, _ = io.WriteString(p.stdout, "\x1b[?25h")
		p.exit(errors.New("signal: terminated"), "")
	}
	return nil
}
func (p *fakeProc) Signals() []syscall.Signal {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]syscall.Signal(nil), p.signals...)
}
func (p *fakeProc) line(s string) { _, _ = io.WriteString(p.stdout, s+"\n") }
func (p *fakeProc) exit(err error, stderr string) {
	p.once.Do(func() {
		_, _ = io.WriteString(p.stderr, stderr)
		p.err = err
		close(p.done)
	})
}

// fakeRunner answers Output from a table keyed by the joined argv (absent
// means exit 1 "not installed", like `flatpak info` for a missing ref) and
// hands every started process to the test.
type fakeRunner struct {
	mu      sync.Mutex
	argvs   [][]string
	answers map[string]Result
	started chan *fakeProc
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{answers: map[string]Result{}, started: make(chan *fakeProc, 4)}
}

func (f *fakeRunner) answer(argv string, res Result) { f.answers[argv] = res }

func (f *fakeRunner) record(argv []string) {
	f.mu.Lock()
	f.argvs = append(f.argvs, append([]string(nil), argv...))
	f.mu.Unlock()
}

func (f *fakeRunner) Output(ctx context.Context, cmd Command) (Result, error) {
	f.record(cmd.Argv)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	f.mu.Lock()
	res, ok := f.answers[strings.Join(cmd.Argv, " ")]
	f.mu.Unlock()
	if !ok {
		return Result{ExitCode: 1, Stderr: []byte("error: not installed\n")}, nil
	}
	return res, nil
}

func (f *fakeRunner) Start(ctx context.Context, cmd Command, stdout, stderr io.Writer) (Process, error) {
	f.record(cmd.Argv)
	p := &fakeProc{stdout: stdout, stderr: stderr, done: make(chan struct{})}
	f.started <- p
	return p, nil
}

func (f *fakeRunner) all() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string(nil), f.argvs...)
}

// realAnswers scripts the fake with the container's real output for
// Moonlight: the remote added, the app and its missing KDE runtime.
func realAnswers(t *testing.T, f *fakeRunner) {
	f.answer("flatpak remote-add --user --if-not-exists flathub "+RemoteURL, Result{})
	f.answer("flatpak remote-info --user flathub "+moonlight, Result{Stdout: testdata(t, "remote-info-moonlight.stdout")})
	f.answer("flatpak remote-info --user flathub runtime/org.kde.Platform/x86_64/6.11", Result{Stdout: testdata(t, "remote-info-kde-runtime.stdout")})
	f.answer("flatpak info --user "+moonlight, Result{Stdout: []byte("ID: " + moonlight + "\nInstallation: user\n")})
}

// disk is the fake filesystem under the user Flatpak directory.
type disk struct {
	mu   sync.Mutex
	free int64
}

func (d *disk) Free(string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.free, nil
}
func (d *disk) use(n int64) { d.mu.Lock(); d.free -= n; d.mu.Unlock() }

func newInstaller(t *testing.T, f *fakeRunner, d *disk) *Installer {
	t.Helper()
	return New(Options{
		Runner: f, Environ: []string{"HOME=/home/tv", "LANG=de_DE.UTF-8", "PATH=/usr/bin"},
		Allowed: []string{moonlight, adapters.ChromiumFlatpakID}, FreeBytes: d.Free,
		LookPath: func(string) (string, error) { return "/usr/bin/flatpak", nil }, GOARCH: "amd64",
		Poll: 2 * time.Millisecond, KillGrace: 30 * time.Millisecond,
	})
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func started(t *testing.T, f *fakeRunner) *fakeProc {
	t.Helper()
	select {
	case p := <-f.started:
		return p
	case <-time.After(3 * time.Second):
		t.Fatal("flatpak install was never started")
		return nil
	}
}

// forbidden are argv words that would weaken Flatpak's checks or reach
// anything but the one remote.
var forbidden = []string{"--no-gpg-verify", "--gpg-import", "--from", "--sideload-repo", "--system", "--installation", "--no-related", "--no-deps", "--reinstall", "--arch", "--commit", "http://", "file:"}

// checkArgvs asserts every recorded call is flatpak with --user and nothing
// forbidden (--system only to *read* whether something is installed).
func checkArgvs(t *testing.T, argvs [][]string) {
	t.Helper()
	for _, argv := range argvs {
		joined := strings.Join(argv, " ")
		if argv[0] != "flatpak" {
			t.Errorf("ran %q", joined)
		}
		readOnlySystem := len(argv) >= 3 && argv[1] == "info" && argv[2] == "--system"
		if !readOnlySystem && !strings.Contains(joined, " --user") {
			t.Errorf("without --user: %q", joined)
		}
		for _, bad := range forbidden {
			if bad == "--system" && readOnlySystem {
				continue
			}
			if strings.Contains(joined, bad) {
				t.Errorf("forbidden %q in %q", bad, joined)
			}
		}
		for _, a := range argv {
			if strings.Contains(a, "://") && a != RemoteURL {
				t.Errorf("a URL other than Flathub's: %q", joined)
			}
		}
	}
}

func TestInstallRunsExactlyTheFixedArgvAndReportsProgress(t *testing.T) {
	f := newFakeRunner()
	realAnswers(t, f)
	d := &disk{free: 50e9}
	var mu sync.Mutex
	var seen []Status
	in := newInstaller(t, f, d)
	in.onChange = func() { mu.Lock(); seen = append(seen, in.Status(moonlight)); mu.Unlock() }
	if err := in.Start(moonlight); err != nil {
		t.Fatal(err)
	}
	p := started(t, f)
	// Replay the real run: one line per operation while the disk fills
	// (2.5 GB in all, measured).
	for _, line := range strings.Split(strings.TrimSpace(string(testdata(t, "install-moonlight.stdout"))), "\n") {
		p.line(line)
		d.use(400e6)
		time.Sleep(10 * time.Millisecond)
	}
	waitFor(t, "phase app", func() bool { return in.Status(moonlight).Phase == contract.PhaseApp })
	p.exit(nil, "")
	waitFor(t, "done", func() bool { return in.Status(moonlight).State == contract.InstallDone })

	st := in.Status(moonlight)
	if st.Progress != 100 || st.SizeBytes != 7_700_000+409_900_000 || st.DiskBytes != 18_600_000+2*1_100_000_000 {
		t.Fatalf("done status %+v", st)
	}
	want := [][]string{
		{"flatpak", "remote-add", "--user", "--if-not-exists", "flathub", "https://dl.flathub.org/repo/flathub.flatpakrepo"},
		{"flatpak", "remote-info", "--user", "flathub", "com.moonlight_stream.Moonlight"},
		{"flatpak", "info", "--user", "runtime/org.kde.Platform/x86_64/6.11"},
		{"flatpak", "info", "--system", "runtime/org.kde.Platform/x86_64/6.11"},
		{"flatpak", "remote-info", "--user", "flathub", "runtime/org.kde.Platform/x86_64/6.11"},
		{"flatpak", "install", "--user", "--noninteractive", "-y", "flathub", "com.moonlight_stream.Moonlight"},
		{"flatpak", "info", "--user", "com.moonlight_stream.Moonlight"},
	}
	got := f.all()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("argv\n got %q\nwant %q", got, want)
	}
	checkArgvs(t, got)

	// The walk: preparing, then downloading with phases from the real
	// lines and a percentage that only grows and stays below 100, then
	// installing (the check), then done.
	mu.Lock()
	defer mu.Unlock()
	states, last, phases := []string{}, -1, map[string]bool{}
	for _, s := range seen {
		if len(states) == 0 || states[len(states)-1] != s.State {
			states = append(states, s.State)
		}
		if s.State == contract.InstallDownloading {
			if s.Progress < last || s.Progress > 99 {
				t.Fatalf("progress went %d → %d", last, s.Progress)
			}
			last = s.Progress
			phases[s.Phase] = true
		}
	}
	if fmt.Sprint(states) != fmt.Sprint([]string{"preparing", "downloading", "installing", "done"}) {
		t.Fatalf("states %v", states)
	}
	if !phases[contract.PhaseRuntime] || !phases[contract.PhaseApp] || last < 80 {
		t.Fatalf("phases %v, last progress %d", phases, last)
	}
	// The locale is forced so the lines parse; the owner's is dropped.
	for _, kv := range in.env {
		if strings.HasPrefix(kv, "LANG=") {
			t.Fatalf("env kept %s", kv)
		}
	}
}

func TestRefsComeOnlyFromTheAdapterTable(t *testing.T) {
	f := newFakeRunner()
	reg := adapters.NewRegistry()
	var ids []string
	for _, a := range reg.All() {
		ids = append(ids, a.FlatpakID())
	}
	in := New(Options{Runner: f, Allowed: ids, FreeBytes: (&disk{free: 1e12}).Free,
		LookPath: func(string) (string, error) { return "/usr/bin/flatpak", nil }, GOARCH: "amd64"})
	for _, a := range reg.All() {
		if !in.Allowed(a.FlatpakID()) {
			t.Errorf("%s (%s) is in the table but not allowed", a.Name(), a.FlatpakID())
		}
	}
	if !in.Allowed(adapters.ChromiumFlatpakID) {
		t.Fatal("Chromium is not installable")
	}
	for _, id := range []string{"org.example.Evil", "--no-gpg-verify", "app/com.moonlight_stream.Moonlight/x86_64/stable", "com.moonlight_stream.Moonlight//beta", ""} {
		if err := in.Start(id); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("Start(%q) = %v, want ErrNotAllowed", id, err)
		}
		if _, err := in.Info(context.Background(), id); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("Info(%q) = %v, want ErrNotAllowed", id, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := in.Update(ctx, []string{moonlight, "org.example.Evil"}); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("Update with a foreign id = %v", err)
	}
	if n := len(f.all()); n != 0 {
		t.Fatalf("flatpak ran %d times for ids outside the table: %q", n, f.all())
	}
}

func TestRemoteAddIsIdempotent(t *testing.T) {
	f := newFakeRunner()
	realAnswers(t, f)
	// A remote that already exists: the real second remote-add prints
	// nothing and exits 0 (testdata/README.md).
	in := newInstaller(t, f, &disk{free: 50e9})
	for n := 0; n < 2; n++ {
		if _, err := in.Info(context.Background(), moonlight); err != nil {
			t.Fatal(err)
		}
	}
	adds := 0
	for _, argv := range f.all() {
		if argv[1] == "remote-add" {
			adds++
			if strings.Join(argv, " ") != "flatpak remote-add --user --if-not-exists flathub "+RemoteURL {
				t.Fatalf("remote-add argv %q", argv)
			}
		}
	}
	if adds != 2 {
		t.Fatalf("%d remote-adds", adds)
	}
	checkArgvs(t, f.all())
}

func TestCancelStopsFlatpakAndReturnsToAvailable(t *testing.T) {
	for _, stubborn := range []bool{false, true} {
		t.Run(fmt.Sprintf("ignores SIGTERM %v", stubborn), func(t *testing.T) {
			f := newFakeRunner()
			realAnswers(t, f)
			in := newInstaller(t, f, &disk{free: 50e9})
			if err := in.Cancel(moonlight); !errors.Is(err, ErrNotRunning) {
				t.Fatalf("cancel with nothing running = %v", err)
			}
			if err := in.Start(moonlight); err != nil {
				t.Fatal(err)
			}
			p := started(t, f)
			p.mu.Lock()
			p.ignoreTerm = stubborn
			p.mu.Unlock()
			p.line("Installing runtime/org.freedesktop.Platform.GL.default/x86_64/25.08")
			waitFor(t, "downloading", func() bool { return in.Status(moonlight).State == contract.InstallDownloading })
			if err := in.Start(adapters.ChromiumFlatpakID); !errors.Is(err, ErrBusy) {
				t.Fatalf("a second install = %v, want ErrBusy", err)
			}
			if err := in.Cancel(moonlight); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "available again", func() bool { return in.Status(moonlight).State == contract.InstallAvailable })
			if st := in.Status(moonlight); st.Message != ReasonCancelled {
				t.Fatalf("status after cancel %+v", st)
			}
			sigs := p.Signals()
			want := []syscall.Signal{syscall.SIGTERM}
			if stubborn {
				want = []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}
			}
			if fmt.Sprint(sigs) != fmt.Sprint(want) {
				t.Fatalf("signals %v, want %v", sigs, want)
			}
			if in.Busy() {
				t.Fatal("still busy after the cancel")
			}
			for _, argv := range f.all() {
				if argv[1] == "info" && argv[len(argv)-1] == moonlight {
					t.Fatal("a cancelled install was checked as if it had finished")
				}
			}
		})
	}
}

func TestNotEnoughSpaceStopsBeforeDownloading(t *testing.T) {
	f := newFakeRunner()
	realAnswers(t, f)
	// The container's 150 MB tmpfs (testdata/README.md).
	in := newInstaller(t, f, &disk{free: 149_479_424})
	if err := in.Start(moonlight); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "failed", func() bool { return in.Status(moonlight).State == contract.InstallFailed })
	st := in.Status(moonlight)
	if !strings.HasPrefix(st.Message, "Not enough space: this needs about 2.8 GB free, and the box has 149 MB.") {
		t.Fatalf("message %q", st.Message)
	}
	for _, argv := range f.all() {
		if argv[1] == "install" {
			t.Fatal("flatpak install ran without the space for it")
		}
	}
}

func TestFlatpaksOwnFailuresInPlainWords(t *testing.T) {
	cases := []struct {
		name, stderr, want string
	}{
		{"disk full while pulling", "install-diskfull.stderr", ReasonNoSpace},
		{"network gone after the remote was added", "install-offline.stderr", ReasonNetwork},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeRunner()
			realAnswers(t, f)
			in := newInstaller(t, f, &disk{free: 50e9})
			if err := in.Start(moonlight); err != nil {
				t.Fatal(err)
			}
			p := started(t, f)
			p.exit(errors.New("exit status 1"), string(testdata(t, tc.stderr)))
			waitFor(t, "failed", func() bool { return in.Status(moonlight).State == contract.InstallFailed })
			if st := in.Status(moonlight); st.Message != tc.want {
				t.Fatalf("message %q, want %q", st.Message, tc.want)
			}
			if in.Busy() {
				t.Fatal("busy after a failure")
			}
		})
	}
}

func TestNoNetworkFailsWhileAddingTheRemote(t *testing.T) {
	f := newFakeRunner()
	f.answer("flatpak remote-add --user --if-not-exists flathub "+RemoteURL, Result{ExitCode: 1, Stderr: testdata(t, "remote-add-offline.stderr")})
	in := newInstaller(t, f, &disk{free: 50e9})
	if err := in.Start(moonlight); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "failed", func() bool { return in.Status(moonlight).State == contract.InstallFailed })
	if st := in.Status(moonlight); st.Message != ReasonNetwork {
		t.Fatalf("message %q", st.Message)
	}
	if n := len(f.all()); n != 1 {
		t.Fatalf("ran %q after the remote failed", f.all())
	}
}

func TestExplainRealErrors(t *testing.T) {
	for file, want := range map[string]string{
		"remote-add-offline.stderr":  ReasonNetwork,
		"install-offline.stderr":     ReasonNetwork,
		"install-diskfull.stderr":    ReasonNoSpace,
		"remote-info-missing.stderr": ReasonNotFound,
	} {
		if got := Explain(string(testdata(t, file))); got != want {
			t.Errorf("%s: %q, want %q", file, got, want)
		}
	}
	if got := Explain("error: Something odd\n"); got != "Flatpak said: Something odd" {
		t.Errorf("unknown error: %q", got)
	}
}

func TestParseRealOutput(t *testing.T) {
	ri := ParseRemoteInfo(testdata(t, "remote-info-moonlight.stdout"))
	if ri.Download != 7_700_000 || ri.Installed != 18_600_000 || ri.Runtime != "org.kde.Platform/x86_64/6.11" || ri.Arch != "x86_64" {
		t.Fatalf("remote-info %+v", ri)
	}
	rt := ParseRemoteInfo(testdata(t, "remote-info-kde-runtime.stdout"))
	if rt.Download != 409_900_000 || rt.Installed != 1_100_000_000 || rt.Runtime != "" {
		t.Fatalf("runtime remote-info %+v", rt)
	}
	// Without a UTF-8 locale GLib's no-break space prints as "?".
	for in, want := range map[string]int64{"7.7?MB": 7_700_000, "7.7 MB": 7_700_000, "1.1 GB": 1_100_000_000, "512 bytes": 512, "18 kB": 18_000} {
		if got, ok := ParseSize(in); !ok || got != want {
			t.Errorf("ParseSize(%q) = %d, %v", in, got, ok)
		}
	}
	var phases []string
	for _, l := range strings.Split(string(testdata(t, "install-moonlight.stdout")), "\n") {
		if p, ok := PhaseOf(l); ok {
			phases = append(phases, p)
		}
	}
	if fmt.Sprint(phases) != "[runtime runtime runtime runtime runtime app]" {
		t.Fatalf("phases %v", phases)
	}
	// A runtime line that could smuggle anything into argv is dropped.
	if ri := ParseRemoteInfo([]byte("Runtime: --no-gpg-verify\n")); ri.Runtime != "" {
		t.Fatalf("runtime %q", ri.Runtime)
	}
}

func TestRuntimeInstalledSystemWideIsNotCounted(t *testing.T) {
	f := newFakeRunner()
	realAnswers(t, f)
	f.answer("flatpak info --system runtime/org.kde.Platform/x86_64/6.11", Result{Stdout: []byte("Installation: system\n")})
	in := newInstaller(t, f, &disk{free: 50e9})
	sz, err := in.Info(context.Background(), moonlight)
	if err != nil {
		t.Fatal(err)
	}
	if sz.Download != 7_700_000 || sz.Expected() != 18_600_000 || sz.RuntimeMissing {
		t.Fatalf("sizes %+v", sz)
	}
	for _, argv := range f.all() {
		if argv[1] == "remote-info" && strings.HasPrefix(argv[len(argv)-1], "runtime/") {
			t.Fatal("asked Flathub for a runtime the system already has")
		}
	}
}

func TestUnavailableWithoutFlatpakOrOffX86(t *testing.T) {
	f := newFakeRunner()
	in := New(Options{Runner: f, Allowed: []string{moonlight}, GOARCH: "amd64",
		LookPath: func(string) (string, error) { return "", errors.New("not found") }})
	if ok, why := in.Available(); ok || why != ReasonNoFlatpak {
		t.Fatalf("Available() = %v, %q", ok, why)
	}
	if err := in.Start(moonlight); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Start = %v", err)
	}
	arm := New(Options{Runner: f, Allowed: []string{moonlight}, GOARCH: "arm64",
		LookPath: func(string) (string, error) { return "/usr/bin/flatpak", nil }})
	if ok, why := arm.Available(); ok || why != ReasonArch {
		t.Fatalf("arm64 Available() = %v, %q", ok, why)
	}
	if len(f.all()) != 0 {
		t.Fatalf("ran %q", f.all())
	}
}

func TestUpdateArgvAndCancel(t *testing.T) {
	f := newFakeRunner()
	in := newInstaller(t, f, &disk{free: 50e9})
	errc := make(chan error, 1)
	go func() { errc <- in.Update(context.Background(), []string{moonlight, adapters.ChromiumFlatpakID}) }()
	p := started(t, f)
	waitFor(t, "updating", in.Updating)
	if err := in.Start(moonlight); !errors.Is(err, ErrBusy) {
		t.Fatalf("install during an update = %v", err)
	}
	if !in.CancelUpdate() {
		t.Fatal("CancelUpdate found nothing to cancel")
	}
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("Update = %v", err)
	}
	if fmt.Sprint(p.Signals()) != fmt.Sprint([]syscall.Signal{syscall.SIGTERM}) {
		t.Fatalf("signals %v", p.Signals())
	}
	want := "[[flatpak update --user --noninteractive -y com.moonlight_stream.Moonlight org.chromium.Chromium]]"
	if fmt.Sprint(f.all()) != want {
		t.Fatalf("argv %q", f.all())
	}
	checkArgvs(t, f.all())

	// Up to date: the real output, exit 0.
	go func() { errc <- in.Update(context.Background(), []string{moonlight}) }()
	p = started(t, f)
	p.line(strings.TrimSpace(string(testdata(t, "update-uptodate.stdout"))))
	p.exit(nil, "")
	if err := <-errc; err != nil {
		t.Fatalf("up-to-date update = %v", err)
	}
}
