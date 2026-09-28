// Tests for the Flatpak launcher with a fake Runner (flatpak.go, runner.go).

package flatpak

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/applications"
)

const infoPlexSystem = `Plex HTPC - Plex HTPC

          ID: tv.plex.PlexHTPC
         Ref: app/tv.plex.PlexHTPC/x86_64/stable
        Arch: x86_64
      Branch: stable
     Version: 1.68.1.1240-97fe4b5c
     License: LicenseRef-proprietary
      Origin: flathub
  Collection: org.flathub.Stable
Installation: system
   Installed: 213.7 MB
     Runtime: org.freedesktop.Platform/x86_64/23.08
         Sdk: org.freedesktop.Sdk/x86_64/23.08

      Commit: 4b1f9d3b1a4c2e1f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2a1f0e9d8c7b6a5f
     Subject: Update to 1.68.1 (abcdef12)
        Date: 2026-08-30 10:11:12 +0000
`

const psNonTTY = "2612853\t2612853\ttv.plex.PlexHTPC\n2613001\t2613001\trocks.shy.VacuumTube\n"

const psTTY = `Instance PID     Application
2612853  2612853 tv.plex.PlexHTPC
2613001  2613001 rocks.shy.VacuumTube
`

type fakeProc struct {
	pid  int
	done chan struct{}
	err  error
}

func (p *fakeProc) PID() int              { return p.pid }
func (p *fakeProc) Done() <-chan struct{} { return p.done }
func (p *fakeProc) Wait() error           { <-p.done; return p.err }

type fakeRunner struct {
	mu       sync.Mutex
	outputs  map[string]Result // key: argv joined by space
	errs     map[string]error
	calls    []Command
	starts   []Command
	proc     *fakeProc
	startErr error
	onStart  func(stdout, stderr io.Writer)
}

func (f *fakeRunner) Output(ctx context.Context, cmd Command) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, cmd)
	key := strings.Join(cmd.Argv, " ")
	if err, ok := f.errs[key]; ok {
		return Result{}, err
	}
	if res, ok := f.outputs[key]; ok {
		return res, nil
	}
	return Result{ExitCode: 1, Stderr: []byte("error: unexpected command " + key)}, nil
}

func (f *fakeRunner) Start(ctx context.Context, cmd Command, stdout, stderr io.Writer) (Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, cmd)
	if f.startErr != nil {
		return nil, f.startErr
	}
	if f.onStart != nil {
		f.onStart(stdout, stderr)
	}
	return f.proc, nil
}

func (f *fakeRunner) argvs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, strings.Join(c.Argv, " "))
	}
	return out
}

func TestParseInfo(t *testing.T) {
	f := ParseInfo([]byte(infoPlexSystem))
	if f.ID != "tv.plex.PlexHTPC" || f.Version != "1.68.1.1240-97fe4b5c" || f.Installation != "system" || f.Origin != "flathub" || f.Runtime != "org.freedesktop.Platform/x86_64/23.08" || f.Ref != "app/tv.plex.PlexHTPC/x86_64/stable" {
		t.Fatalf("%+v", f)
	}
}

func TestParsePS(t *testing.T) {
	for _, out := range []string{psNonTTY, psTTY} {
		insts := ParsePS([]byte(out))
		if len(insts) != 2 || insts[0].InstanceID != "2612853" || insts[0].PID != 2612853 || insts[0].FlatpakID != "tv.plex.PlexHTPC" || insts[1].FlatpakID != "rocks.shy.VacuumTube" {
			t.Fatalf("%+v", insts)
		}
	}
	if got := ParsePS(nil); got != nil {
		t.Fatalf("%v", got)
	}
}

func TestValidateAppIDAndInstallCommand(t *testing.T) {
	for _, ok := range []string{"tv.plex.PlexHTPC", "rocks.shy.VacuumTube", "org.kde.Platform", "com.example.my-app_1"} {
		if err := ValidateAppID(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "plex", "tv.plex", "-x.y.z", "a.b.c d", "a.b.c;rm", "1a.b.c", "../x.y"} {
		if err := ValidateAppID(bad); !errors.Is(err, ErrInvalidAppID) {
			t.Errorf("%q accepted", bad)
		}
	}
	if got := strings.Join(InstallCommand("tv.plex.PlexHTPC"), " "); got != "flatpak install --user flathub tv.plex.PlexHTPC" {
		t.Fatal(got)
	}
}

func TestPassthroughEnv(t *testing.T) {
	env := PassthroughEnv([]string{"DISPLAY=:0", "DEEPSEEK_API_KEY=x", "HOME=/home/u", "BDTV_TOKEN=secret", "MALFORMED"})
	if strings.Join(env, ",") != "DISPLAY=:0,HOME=/home/u" {
		t.Fatalf("%v", env)
	}
}

func TestDiscover(t *testing.T) {
	notInstalled := Result{ExitCode: 1, Stderr: []byte("error: tv.plex.PlexHTPC/x86_64/stable not installed\n")}
	r := &fakeRunner{outputs: map[string]Result{
		"flatpak info --user tv.plex.PlexHTPC":       notInstalled,
		"flatpak info --system tv.plex.PlexHTPC":     {Stdout: []byte(infoPlexSystem)},
		"flatpak info --user rocks.shy.VacuumTube":   {ExitCode: 1, Stderr: []byte("error: rocks.shy.VacuumTube/x86_64/stable not installed")},
		"flatpak info --system rocks.shy.VacuumTube": {ExitCode: 1, Stderr: []byte("error: rocks.shy.VacuumTube/x86_64/stable not installed")},
		"flatpak info --user org.kde.Platform":       {ExitCode: 1, Stderr: []byte("error: Unable to load summary from remote")},
	}}
	l := New(Options{Runner: r, Environ: []string{"DISPLAY=:0"}})
	ctx := context.Background()
	inst, err := l.Discover(ctx, "tv.plex.PlexHTPC")
	if err != nil || !inst.Installed || inst.Scope != "system" || inst.Version != "1.68.1.1240-97fe4b5c" {
		t.Fatalf("%+v %v", inst, err)
	}
	inst, err = l.Discover(ctx, "rocks.shy.VacuumTube")
	if err != nil || inst.Installed || inst.Scope != "none" {
		t.Fatalf("%+v %v", inst, err)
	}
	if _, err := l.Discover(ctx, "org.kde.Platform"); err == nil {
		t.Fatal("unexpected CLI error not surfaced")
	}
	if _, err := l.Discover(ctx, "plex"); !errors.Is(err, ErrInvalidAppID) {
		t.Fatal(err)
	}
	r2 := &fakeRunner{errs: map[string]error{"flatpak info --user tv.plex.PlexHTPC": errors.New("exec: flatpak: not found")}}
	if inst, err := New(Options{Runner: r2, Environ: []string{}}).Discover(ctx, "tv.plex.PlexHTPC"); err == nil || inst.Scope != "unknown" {
		t.Fatalf("%+v %v", inst, err)
	}
}

func TestLaunchResolvesInstanceByPID(t *testing.T) {
	proc := &fakeProc{pid: 2612853, done: make(chan struct{})}
	r := &fakeRunner{proc: proc, outputs: map[string]Result{
		"flatpak ps --columns=instance,pid,application": {Stdout: []byte(psNonTTY)},
	}}
	l := New(Options{Runner: r, Environ: []string{"DISPLAY=:0", "SECRET=1"}, InstanceWait: time.Second})
	inst, err := l.Launch(context.Background(), "tv.plex.PlexHTPC", nil)
	if err != nil {
		t.Fatal(err)
	}
	if inst.InstanceID != "2612853" || inst.PID != 2612853 || inst.FlatpakID != "tv.plex.PlexHTPC" {
		t.Fatalf("%+v", inst)
	}
	if got := strings.Join(r.starts[0].Argv, " "); got != "flatpak run tv.plex.PlexHTPC" {
		t.Fatal(got)
	}
	if strings.Join(r.starts[0].Env, ",") != "DISPLAY=:0" {
		t.Fatalf("env leaked: %v", r.starts[0].Env)
	}
}

func TestLaunchResolvesFreshInstanceAndCapturesOutput(t *testing.T) {
	proc := &fakeProc{pid: 999, done: make(chan struct{})}
	r := &fakeRunner{proc: proc, onStart: func(stdout, stderr io.Writer) {
		_, _ = io.WriteString(stderr, "Gtk-Message: something\n")
	}}
	r.outputs = map[string]Result{"flatpak ps --columns=instance,pid,application": {Stdout: []byte("1\t1\ttv.plex.PlexHTPC\n")}}
	l := New(Options{Runner: r, Environ: []string{}, InstanceWait: time.Second})
	go func() {
		time.Sleep(150 * time.Millisecond)
		r.mu.Lock()
		r.outputs["flatpak ps --columns=instance,pid,application"] = Result{Stdout: []byte("1\t1\ttv.plex.PlexHTPC\n7\t7\trocks.shy.VacuumTube\n")}
		r.mu.Unlock()
	}()
	inst, err := l.Launch(context.Background(), "rocks.shy.VacuumTube", []string{"--fullscreen", "--no-window-decorations"})
	if err != nil {
		t.Fatal(err)
	}
	if inst.InstanceID != "7" || inst.PID != 7 {
		t.Fatalf("%+v", inst)
	}
	if got := strings.Join(r.starts[0].Argv, " "); got != "flatpak run rocks.shy.VacuumTube --fullscreen --no-window-decorations" {
		t.Fatal(got)
	}
	if _, stderr, ok := l.Output(999); !ok || !strings.Contains(stderr, "Gtk-Message") {
		t.Fatalf("%q %v", stderr, ok)
	}
}

func TestLaunchEarlyExitIsFailure(t *testing.T) {
	proc := &fakeProc{pid: 4, done: make(chan struct{}), err: errors.New("exit status 1")}
	close(proc.done)
	r := &fakeRunner{proc: proc, onStart: func(_, stderr io.Writer) {
		_, _ = io.WriteString(stderr, "error: app/tv.plex.PlexHTPC/x86_64/stable not installed")
	}}
	r.outputs = map[string]Result{"flatpak ps --columns=instance,pid,application": {}}
	_, err := New(Options{Runner: r, Environ: []string{}, InstanceWait: time.Second}).Launch(context.Background(), "tv.plex.PlexHTPC", nil)
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("err=%v", err)
	}
	if _, err := New(Options{Runner: r, Environ: []string{}}).Launch(context.Background(), "tv.plex.PlexHTPC", []string{"bad\narg"}); err == nil {
		t.Fatal("control character in arg accepted")
	}
}

func TestLaunchTimesOutWithoutInstance(t *testing.T) {
	proc := &fakeProc{pid: 5, done: make(chan struct{})}
	r := &fakeRunner{proc: proc, outputs: map[string]Result{"flatpak ps --columns=instance,pid,application": {}}}
	inst, err := New(Options{Runner: r, Environ: []string{}, InstanceWait: 150 * time.Millisecond}).Launch(context.Background(), "tv.plex.PlexHTPC", nil)
	if err != nil || inst.InstanceID != "" || inst.PID != 5 {
		t.Fatalf("%+v %v", inst, err)
	}
}

func TestKillNeverByName(t *testing.T) {
	r := &fakeRunner{outputs: map[string]Result{
		"flatpak ps --columns=instance,pid,application": {Stdout: []byte(psNonTTY)},
		"flatpak kill 2612853":                          {},
		"flatpak kill 2613001":                          {},
	}}
	l := New(Options{Runner: r, Environ: []string{}})
	ctx := context.Background()
	if err := l.Kill(ctx, applications.Instance{FlatpakID: "tv.plex.PlexHTPC", InstanceID: "2612853"}); err != nil {
		t.Fatal(err)
	}
	// Unknown instance id, exactly one instance of the app: resolved to its id.
	if err := l.Kill(ctx, applications.Instance{FlatpakID: "rocks.shy.VacuumTube", PID: 2613001}); err != nil {
		t.Fatal(err)
	}
	// Tracked pid disagrees with the running instance: refused.
	if err := l.Kill(ctx, applications.Instance{FlatpakID: "rocks.shy.VacuumTube", PID: 1}); !errors.Is(err, ErrAmbiguousInstance) {
		t.Fatal(err)
	}
	// Two instances of the app: refused.
	r.mu.Lock()
	r.outputs["flatpak ps --columns=instance,pid,application"] = Result{Stdout: []byte("1\t1\ttv.plex.PlexHTPC\n2\t2\ttv.plex.PlexHTPC\n")}
	r.mu.Unlock()
	if err := l.Kill(ctx, applications.Instance{FlatpakID: "tv.plex.PlexHTPC"}); !errors.Is(err, ErrAmbiguousInstance) {
		t.Fatal(err)
	}
	// An instance id that is not numeric can never reach argv.
	if err := l.Kill(ctx, applications.Instance{FlatpakID: "tv.plex.PlexHTPC", InstanceID: "tv.plex.PlexHTPC"}); err == nil {
		t.Fatal("app id passed as instance id")
	}
	for _, argv := range r.argvs() {
		if strings.HasPrefix(argv, "flatpak kill ") && !isNumeric(strings.TrimPrefix(argv, "flatpak kill ")) {
			t.Fatalf("kill by name issued: %s", argv)
		}
	}
}

func TestRing(t *testing.T) {
	r := NewRing(8)
	_, _ = r.Write([]byte("abc"))
	if r.Tail() != "abc" {
		t.Fatal(r.Tail())
	}
	_, _ = r.Write([]byte("defghij"))
	if r.Tail() != "cdefghij" {
		t.Fatalf("%q", r.Tail())
	}
	_, _ = r.Write([]byte("0123456789ABCDEF"))
	if r.Tail() != "89ABCDEF" {
		t.Fatalf("%q", r.Tail())
	}
	r2 := NewRing(4)
	_, _ = r2.Write([]byte("abcd"))
	if r2.Tail() != "abcd" {
		t.Fatalf("%q", r2.Tail())
	}
	_, _ = r2.Write([]byte("e"))
	if r2.Tail() != "bcde" {
		t.Fatalf("%q", r2.Tail())
	}
}

func TestExecRunnerOutputAndStart(t *testing.T) {
	ctx := context.Background()
	res, err := ExecRunner{}.Output(ctx, Command{Argv: []string{"sh", "-c", "echo out; echo err >&2; exit 3"}, Env: []string{"PATH=/usr/bin:/bin"}})
	if err != nil || res.ExitCode != 3 || strings.TrimSpace(string(res.Stdout)) != "out" || strings.TrimSpace(string(res.Stderr)) != "err" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := (ExecRunner{}).Output(ctx, Command{Argv: []string{"/nonexistent/flatpak"}}); err == nil {
		t.Fatal("missing binary did not error")
	}
	ring := NewRing(64)
	p, err := ExecRunner{}.Start(ctx, Command{Argv: []string{"sh", "-c", "echo started"}, Env: []string{"PATH=/usr/bin:/bin"}}, ring, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if p.PID() <= 0 {
		t.Fatal("no pid")
	}
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}
	if err := p.Wait(); err != nil || strings.TrimSpace(ring.Tail()) != "started" {
		t.Fatalf("%v %q", err, ring.Tail())
	}
}
