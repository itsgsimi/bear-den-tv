// Shell-level tests of scripts/start-session.sh as it runs on the TV
// (docs/operations.md → The watchdog): it detaches what it starts from the
// terminal, its watchdog restarts a coordinator that is stopped (state T),
// unresponsive, or deaf to SIGTERM, and `stop` continues a stopped
// coordinator before ending it, finds one whose binary an upgrade replaced,
// and never touches a process whose command line only mentions the name.
//
// The coordinator is a fake: this test binary, copied into an installed
// layout as <prefix>/bin/bear-den-tv (TestMain runs fakeCoordinator when
// started under that name). `session` writes a heartbeat every 100 ms (not
// while <fake>/hang exists) and records its pid in <fake>/starts;
// `doctor --ping` succeeds while the heartbeat is fresh. The watchdog's
// timings are shortened through BDTV_WATCH_* and BDTV_STOP_TIMEOUT.
package packaging_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == "bear-den-tv" {
		os.Exit(fakeCoordinator(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeCoordinator stands in for /usr/bin/bear-den-tv (see the file comment).
func fakeCoordinator(args []string) int {
	dir := os.Getenv("BDTV_FAKE_DIR")
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(dir, name)); return err == nil }
	appendLine := func(name, line string) {
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, line)
			f.Close()
		}
	}
	switch {
	case len(args) > 0 && args[0] == "session":
		term := make(chan os.Signal, 1)
		if exists("ignore-term") {
			signal.Ignore(syscall.SIGTERM)
		} else {
			signal.Notify(term, syscall.SIGTERM)
		}
		appendLine("starts", strconv.Itoa(os.Getpid()))
		tick := time.NewTicker(100 * time.Millisecond)
		for {
			select {
			case <-term:
				appendLine("events", "term "+strconv.Itoa(os.Getpid()))
				return 0 // like the real coordinator: SIGTERM is a clean exit
			case <-tick.C:
				if !exists("hang") {
					_ = os.WriteFile(filepath.Join(dir, "heartbeat"), []byte(strconv.Itoa(os.Getpid())), 0o644)
				}
			}
		}
	case len(args) > 1 && args[0] == "doctor" && args[1] == "--ping":
		appendLine("pings", strconv.Itoa(os.Getpid()))
		st, err := os.Stat(filepath.Join(dir, "heartbeat"))
		if err != nil || time.Since(st.ModTime()) > time.Second {
			fmt.Fprintln(os.Stderr, "bear-den-tv: the coordinator did not answer (fake: no fresh heartbeat)")
			return 1
		}
		fmt.Println("ok: the coordinator answered in 0 ms")
		return 0
	}
	fmt.Fprintln(os.Stderr, "fake bear-den-tv: unexpected arguments", args)
	return 2
}

// rig is an installed layout around a copy of start-session.sh and the fake.
type rig struct {
	t                 *testing.T
	root, bin, script string
	fake, log         string
	env               []string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	root := t.TempDir()
	r := &rig{t: t, root: root,
		bin:    filepath.Join(root, "usr", "bin", "bear-den-tv"),
		script: filepath.Join(root, "usr", "lib", "bear-den-tv", "start-session.sh"),
		fake:   filepath.Join(root, "fake"),
		log:    filepath.Join(root, "state", "bear-den-tv", "session.log"),
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	copyFile(t, self, r.bin)
	copyFile(t, filepath.Join(repoRoot(t), "scripts", "start-session.sh"), r.script)
	for _, d := range []string{r.fake, filepath.Join(root, "home"), filepath.Join(root, "run")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	r.env = append(os.Environ(),
		"HOME="+filepath.Join(root, "home"),
		"XDG_STATE_HOME="+filepath.Join(root, "state"),
		"XDG_RUNTIME_DIR="+filepath.Join(root, "run"),
		"XDG_SESSION_ID=test", "DISPLAY=:99",
		"BDTV_FAKE_DIR="+r.fake,
		"BDTV_WATCH_INTERVAL=1", "BDTV_WATCH_FAILURES=2", "BDTV_WATCH_GRACE=2",
		"BDTV_WATCH_PING_TIMEOUT=1", "BDTV_STOP_TIMEOUT=3",
	)
	t.Cleanup(func() {
		out, err := r.run("stop")
		if err != nil {
			t.Errorf("cleanup stop: %v\n%s", err, out)
		}
		for _, pid := range r.starts() { // a safety net if stop is broken
			_ = syscall.Kill(pid, syscall.SIGCONT)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		if t.Failed() {
			t.Logf("session.log:\n%s", r.logText())
		}
	})
	return r
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	raw, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, raw, 0o755); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) command(args ...string) *exec.Cmd {
	cmd := exec.Command("bash", append([]string{r.script}, args...)...)
	cmd.Env = r.env
	return cmd
}

func (r *rig) run(args ...string) (string, error) {
	out, err := r.command(args...).CombinedOutput()
	return string(out), err
}

// runOnTerminal runs the script as the session leader of a new pty, which
// is its controlling terminal (like an interactive ssh login), and returns
// what it printed. Closing the pty afterwards hangs the terminal up.
func (r *rig) runOnTerminal(args ...string) string {
	r.t.Helper()
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		r.t.Fatalf("no pty: %v", err)
	}
	defer ptmx.Close()
	if err := unix.IoctlSetPointerInt(int(ptmx.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		r.t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(ptmx.Fd()), unix.TIOCGPTN)
	if err != nil {
		r.t.Fatal(err)
	}
	pts, err := os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	var out bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&out, ptmx); close(done) }()
	run := func(cmd *exec.Cmd) {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = pts, pts, pts
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
		if err := cmd.Run(); err != nil {
			r.t.Fatalf("%v: %v", cmd.Args, err)
		}
	}
	// The control: a process started this way does have the terminal.
	probe := exec.Command("bash", "-c", `s=$(cat /proc/$$/stat); s=${s##*) }; set -- $s; echo "probe tty_nr=$5"`)
	run(probe)
	run(r.command(args...))
	pts.Close()
	ptmx.Close() // hang up
	<-done
	return out.String()
}

func (r *rig) starts() []int {
	raw, _ := os.ReadFile(filepath.Join(r.fake, "starts"))
	var pids []int
	for _, f := range strings.Fields(string(raw)) {
		if p, err := strconv.Atoi(f); err == nil {
			pids = append(pids, p)
		}
	}
	return pids
}

// waitStarts waits until the fake coordinator has started n times.
func (r *rig) waitStarts(n int, within time.Duration) []int {
	r.t.Helper()
	deadline := time.Now().Add(within)
	for {
		if p := r.starts(); len(p) >= n {
			return p
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("the coordinator started %d times within %s, want %d", len(r.starts()), within, n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (r *rig) logText() string {
	raw, _ := os.ReadFile(r.log)
	return string(raw)
}

// waitLog waits until session.log contains want.
func (r *rig) waitLog(want string, within time.Duration) {
	r.t.Helper()
	deadline := time.Now().Add(within)
	for !strings.Contains(r.logText(), want) {
		if time.Now().After(deadline) {
			r.t.Fatalf("session.log has no %q within %s", want, within)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (r *rig) touch(name string) {
	if err := os.WriteFile(filepath.Join(r.fake, name), nil, 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// stat is one process's /proc/<pid>/stat fields after the command name.
type stat struct {
	state                  string
	ppid, pgrp, sid, ttyNr int
}

func procStat(pid int) (stat, bool) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return stat{}, false
	}
	s := string(raw)
	f := strings.Fields(s[strings.LastIndex(s, ")")+2:])
	atoi := func(i int) int { v, _ := strconv.Atoi(f[i]); return v }
	return stat{state: f[0], ppid: atoi(1), pgrp: atoi(2), sid: atoi(3), ttyNr: atoi(4)}, true
}

// running: the process exists and is not a zombie.
func running(pid int) bool {
	st, ok := procStat(pid)
	return ok && st.state != "Z"
}

// heartbeatAfter waits until the fake coordinator writes a heartbeat after t0.
func (r *rig) heartbeatAfter(t0 time.Time, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if st, err := os.Stat(filepath.Join(r.fake, "heartbeat")); err == nil && st.ModTime().After(t0) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// Started from a terminal (an ssh login with a pty), the watchdog and the
// coordinator end up in their own session, with no controlling terminal,
// stdin from /dev/null and output in the log; they survive the terminal
// hanging up, and the terminal's job-control signals cannot stop them.
func TestStartSessionDetachesFromTheTerminal(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	out := r.runOnTerminal("--watch")
	var probeTTY int
	if i := strings.Index(out, "probe tty_nr="); i >= 0 {
		probeTTY, _ = strconv.Atoi(strings.Fields(out[i+len("probe tty_nr="):])[0])
	}
	if probeTTY == 0 {
		t.Fatalf("the control process had no controlling terminal, so this test proves nothing:\n%s", out)
	}
	if !strings.Contains(out, "started with watchdog") {
		t.Fatalf("start printed:\n%s", out)
	}
	coord := r.waitStarts(1, 15*time.Second)[0]
	cs, ok := procStat(coord)
	if !ok {
		t.Fatal("coordinator gone")
	}
	watcher := cs.ppid
	ws, ok := procStat(watcher)
	if !ok {
		t.Fatal("watchdog gone")
	}
	if ws.sid != watcher || ws.pgrp != watcher {
		t.Errorf("watchdog pid %d: session %d, group %d; want a session and group leader", watcher, ws.sid, ws.pgrp)
	}
	if cs.sid != watcher {
		t.Errorf("coordinator pid %d in session %d, want the watchdog's %d", coord, cs.sid, watcher)
	}
	for name, st := range map[string]stat{"watchdog": ws, "coordinator": cs} {
		if st.ttyNr != 0 {
			t.Errorf("%s has controlling terminal %d, want none", name, st.ttyNr)
		}
	}
	for name, pid := range map[string]int{"watchdog": watcher, "coordinator": coord} {
		want := map[string]string{"0": "/dev/null", "1": r.log, "2": r.log}
		for fd, target := range want {
			if got, _ := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, fd)); got != target {
				t.Errorf("%s fd %s is %q, want %q", name, fd, got, target)
			}
		}
	}
	// The terminal is gone (runOnTerminal hung it up); both still run.
	if !running(watcher) || !r.heartbeatAfter(time.Now(), 5*time.Second) {
		t.Fatal("the watchdog or coordinator did not survive the terminal hanging up")
	}
	// Job-control signals are discarded for its (orphaned) process group:
	// the coordinator keeps beating after each. (The real coordinator also
	// ignores them itself: cmd/bear-den-tv/daemon.go.)
	for _, sig := range []syscall.Signal{syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU} {
		t0 := time.Now()
		if err := syscall.Kill(coord, sig); err != nil {
			t.Fatal(err)
		}
		if !r.heartbeatAfter(t0, 5*time.Second) {
			st, _ := procStat(coord)
			t.Fatalf("after %s the coordinator stopped beating (state %s)", unix.SignalName(sig), st.state)
		}
	}
	if len(r.starts()) != 1 {
		t.Fatalf("restarted %d times, want never", len(r.starts())-1)
	}
}

// The incident: the coordinator in state T (SIGSTOP). The watchdog sees the
// state, continues and terminates it, and starts a fresh one.
func TestWatchdogRestartsAStoppedCoordinator(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if out, err := r.run("--watch"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	first := r.waitStarts(1, 15*time.Second)[0]
	stopped := time.Now()
	if err := syscall.Kill(first, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	second := r.waitStarts(2, 20*time.Second)[1]
	t.Logf("recovered in %s (check interval 1 s)", time.Since(stopped).Round(100*time.Millisecond))
	if running(first) {
		t.Fatalf("the stopped coordinator %d still runs next to %d", first, second)
	}
	if !r.heartbeatAfter(time.Now(), 5*time.Second) {
		t.Fatal("the new coordinator does not run")
	}
	for _, want := range []string{
		fmt.Sprintf("coordinator pid %d is stopped (state T in /proc/%d/stat); sending SIGCONT and SIGTERM", first, first),
		fmt.Sprintf("coordinator pid %d ended after SIGTERM", first),
		fmt.Sprintf("started coordinator pid %d", second),
	} {
		r.waitLog(want, 5*time.Second)
	}
}

// A coordinator that runs but no longer answers (a deadlock) is restarted
// after BDTV_WATCH_FAILURES failed pings in a row; one that ignores SIGTERM
// is killed after the grace period. stop then also has to SIGKILL the next
// one, which ignores SIGTERM too.
func TestWatchdogRestartsAnUnresponsiveCoordinator(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.touch("ignore-term")
	if out, err := r.run("--watch"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	first := r.waitStarts(1, 15*time.Second)[0]
	if !r.heartbeatAfter(time.Now(), 5*time.Second) {
		t.Fatal("no heartbeat")
	}
	r.touch("hang")
	second := r.waitStarts(2, 30*time.Second)[1]
	_ = os.Remove(filepath.Join(r.fake, "hang"))
	if running(first) {
		t.Fatalf("the unresponsive coordinator %d still runs", first)
	}
	for _, want := range []string{
		fmt.Sprintf("coordinator pid %d did not answer a ping (1 of 2)", first),
		fmt.Sprintf("coordinator pid %d did not answer 2 pings in a row; sending SIGCONT and SIGTERM", first),
		fmt.Sprintf("coordinator pid %d still running 2s after SIGTERM; sending SIGKILL", first),
		fmt.Sprintf("coordinator pid %d killed", first),
		fmt.Sprintf("started coordinator pid %d", second),
	} {
		r.waitLog(want, 5*time.Second)
	}
	start := time.Now()
	out, err := r.run("stop")
	if err != nil || !strings.Contains(out, "stopped") || !strings.Contains(out, "sending SIGKILL") {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if running(second) {
		t.Fatalf("stop left %d running", second)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("stop took %s", d)
	}
}

// stop continues a stopped coordinator (which holds SIGTERM until it runs
// again) and waits for it, and it matches by executable and arguments: a
// process whose command line only mentions the coordinator and the watchdog
// (here a shell, like an ssh command running them) is left alone.
func TestStopEndsAStoppedCoordinatorAndNothingElse(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	decoy := exec.Command("sh", "-c", "sleep 60; : "+r.bin+" session "+r.script+" __watch")
	if err := decoy.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decoy.Process.Kill(); _ = decoy.Wait() })
	if out, err := r.run("--watch"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	coord := r.waitStarts(1, 15*time.Second)[0]
	cs, _ := procStat(coord)
	watcher := cs.ppid
	if err := syscall.Kill(coord, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	out, err := r.run("stop")
	if err != nil || strings.TrimSpace(out) != "stopped" {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if running(coord) || running(watcher) {
		t.Fatalf("stop left the coordinator (%v) or the watchdog (%v) running", running(coord), running(watcher))
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("stop took %s: it waited for the timeout instead of continuing the coordinator", d)
	}
	raw, _ := os.ReadFile(filepath.Join(r.fake, "events"))
	if !strings.Contains(string(raw), "term "+strconv.Itoa(coord)) {
		t.Fatalf("the coordinator did not get SIGTERM (it was killed instead): %q", raw)
	}
	if !running(decoy.Process.Pid) {
		t.Fatal("stop killed a process whose command line only mentions bear-den-tv session")
	}
}

// An upgrade (dpkg, deploy-target.sh) replaces the binary by rename; the
// running coordinator's /proc/<pid>/exe then reads "... (deleted)", and stop
// must still find it.
func TestStopFindsACoordinatorWhoseBinaryWasReplaced(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if out, err := r.run("--watch"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	coord := r.waitStarts(1, 15*time.Second)[0]
	copyFile(t, r.bin, r.bin+".new")
	if err := os.Rename(r.bin+".new", r.bin); err != nil {
		t.Fatal(err)
	}
	if exe, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", coord)); !strings.HasSuffix(exe, " (deleted)") {
		t.Fatalf("exe %q: the replacement did not take", exe)
	}
	if out, err := r.run("stop"); err != nil || strings.TrimSpace(out) != "stopped" {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if running(coord) {
		t.Fatal("stop missed the coordinator running the replaced binary")
	}
}
