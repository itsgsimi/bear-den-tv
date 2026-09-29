// Tests for proc.go: a directory shaped like /proc with the files Flatpak
// really writes (/.flatpak-info reached through <pid>/root, the systemd
// scope in <pid>/cgroup), plus one read of this machine's own /proc.

package proc

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// vacuumTubeInfo is the shape of a real /.flatpak-info (VacuumTube's app
// id; the other keys trimmed).
const vacuumTubeInfo = `[Application]
name=rocks.shy.VacuumTube
runtime=runtime/org.freedesktop.Platform/x86_64/24.08

[Instance]
instance-id=1234567890
app-path=/var/lib/flatpak/app/rocks.shy.VacuumTube/x86_64/stable/active/files
`

type fakeProc struct {
	t    *testing.T
	root string
}

func newFakeProc(t *testing.T) *fakeProc {
	return &fakeProc{t: t, root: t.TempDir()}
}

// add makes <root>/<pid> with a stat naming ppid, and optionally a
// .flatpak-info under its root and a cgroup file.
func (f *fakeProc) add(pid, ppid int, info, cgroup string) {
	f.t.Helper()
	d := filepath.Join(f.root, strconv.Itoa(pid))
	if err := os.MkdirAll(filepath.Join(d, "root"), 0o755); err != nil {
		f.t.Fatal(err)
	}
	stat := strconv.Itoa(pid) + " (a (weird) name) S " + strconv.Itoa(ppid) + " 1 1 0 -1\n"
	if err := os.WriteFile(filepath.Join(d, "stat"), []byte(stat), 0o644); err != nil {
		f.t.Fatal(err)
	}
	if info != "" {
		if err := os.WriteFile(filepath.Join(d, "root", ".flatpak-info"), []byte(info), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	if cgroup != "" {
		if err := os.WriteFile(filepath.Join(d, "cgroup"), []byte(cgroup), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *fakeProc) table() Table { return Table{Root: f.root, UID: os.Getuid()} }

func TestFlatpakIDFromInfoAndScope(t *testing.T) {
	f := newFakeProc(t)
	scope := "0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-flatpak-rocks.shy.VacuumTube-2871.scope\n"
	f.add(100, 1, vacuumTubeInfo, scope)                      // both agree
	f.add(101, 1, vacuumTubeInfo, "0::/user.slice/x.scope\n") // info only (the D-Bus proxy's sandbox)
	f.add(102, 1, "", scope)                                  // scope only
	f.add(103, 1, "", "0::/user.slice/user-1000.slice/session-2.scope\n")
	f.add(104, 1, vacuumTubeInfo, "0::/app.slice/app-flatpak-tv.plex.PlexHTPC-9.scope\n") // disagree
	f.add(105, 1, "[Application]\nruntime=x\n", "")                                       // no name
	f.add(106, 1, "", `0::/app.slice/app-flatpak-org.example.My\x2dApp-12.scope`)
	tab := f.table()
	for _, tc := range []struct {
		pid  int
		want string
		err  bool
	}{
		{100, "rocks.shy.VacuumTube", false},
		{101, "rocks.shy.VacuumTube", false},
		{102, "rocks.shy.VacuumTube", false},
		{103, "", false},
		{104, "", true},
		{105, "", true},
		{106, "org.example.My-App", false},
		{999, "", true}, // gone
	} {
		got, err := tab.FlatpakID(tc.pid)
		if got != tc.want || (err != nil) != tc.err {
			t.Errorf("pid %d: %q, %v; want %q, error %v", tc.pid, got, err, tc.want, tc.err)
		}
	}
}

func TestOtherUsersProcessesAreNeverRead(t *testing.T) {
	f := newFakeProc(t)
	f.add(100, 1, vacuumTubeInfo, "")
	tab := f.table()
	tab.UID = os.Getuid() + 1
	if id, err := tab.FlatpakID(100); !errors.Is(err, ErrOtherUser) || id != "" {
		t.Fatalf("another user's process: %q, %v", id, err)
	}
	if ok, err := tab.DescendsFrom(100, 50); ok || err != nil {
		t.Fatalf("another user's process descends: %v, %v", ok, err)
	}
}

func TestDescendsFrom(t *testing.T) {
	f := newFakeProc(t)
	f.add(10, 1, "", "")  // the web manager's browser (flatpak run → bwrap)
	f.add(11, 10, "", "") // the D-Bus proxy
	f.add(12, 10, "", "") // bwrap child
	f.add(13, 12, "", "") // chromium
	f.add(20, 1, "", "")  // another browser
	f.add(21, 20, "", "") // its chromium
	f.add(30, 30, "", "") // a loop ends the walk
	tab := f.table()
	for _, tc := range []struct {
		pid, root int
		want      bool
	}{
		{10, 10, true}, {11, 10, true}, {13, 10, true},
		{21, 10, false}, {20, 10, false}, {13, 20, false}, {30, 10, false},
	} {
		if got, err := tab.DescendsFrom(tc.pid, tc.root); got != tc.want || err != nil {
			t.Errorf("DescendsFrom(%d, %d) = %v, %v; want %v", tc.pid, tc.root, got, err, tc.want)
		}
	}
	if _, err := tab.DescendsFrom(13, 1); err == nil {
		t.Error("init as the root must be refused")
	}
}

func TestValidAppID(t *testing.T) {
	for id, want := range map[string]bool{
		"rocks.shy.VacuumTube": true, "tv.plex.PlexHTPC": true, "org.chromium.Chromium": true,
		"chromium": false, "a..b.c": false, "a.b": false, "a.b.c/d": false, "": false, "a.b.c d": false,
	} {
		if ValidAppID(id) != want {
			t.Errorf("ValidAppID(%q) != %v", id, want)
		}
	}
}

func TestThisMachine(t *testing.T) {
	tab := Host()
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc here")
	}
	if ok, err := tab.DescendsFrom(os.Getpid(), os.Getppid()); !ok || err != nil {
		t.Fatalf("this test does not descend from its parent: %v, %v", ok, err)
	}
	if _, err := tab.FlatpakID(os.Getpid()); err != nil {
		t.Fatalf("reading this process: %v", err)
	}
}
