package links

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeMime is xdg-mime with one default for https links.
type fakeMime struct {
	def   string
	calls []string
}

func (f *fakeMime) run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if name != "xdg-mime" {
		return "", os.ErrNotExist
	}
	switch args[0] {
	case "query":
		return f.def + "\n", nil
	case "default":
		f.def = args[1]
		return "", nil
	}
	return "", os.ErrInvalid
}

func testPaths(t *testing.T) Paths {
	dir := t.TempDir()
	return Paths{
		DataHome:  filepath.Join(dir, "data"),
		StateHome: filepath.Join(dir, "state"),
		DataDirs:  []string{filepath.Join(dir, "usr")},
		System:    filepath.Join(dir, "usr", "applications", DesktopID),
	}
}

func TestEnsureTakesOverAndDisableGivesBack(t *testing.T) {
	ctx := context.Background()
	p := testPaths(t)
	m := &fakeMime{def: "firefox.desktop"}
	changed, err := Ensure(ctx, p, "/opt/bear den/bear-den-tv", m.run)
	if err != nil || !changed || m.def != DesktopID {
		t.Fatalf("Ensure: changed=%v err=%v default=%s", changed, err, m.def)
	}
	if st, _ := Load(p); st.Previous != "firefox.desktop" {
		t.Fatalf("remembered %+v", st)
	}
	// A checkout writes its own entry, for its own binary.
	b, err := os.ReadFile(filepath.Join(p.DataHome, "applications", DesktopID))
	if err != nil {
		t.Fatal(err)
	}
	line, _ := execLine(string(b))
	if argv, err := ExecArgv(line, "https://x.test/"); err != nil || !reflect.DeepEqual(argv, []string{"/opt/bear den/bear-den-tv", "open-url", "https://x.test/"}) {
		t.Fatalf("entry Exec %q gives %q %v", line, argv, err)
	}
	// Again: nothing changes, and the remembered browser is not overwritten
	// with Bear Den itself.
	if changed, err := Ensure(ctx, p, "/x", m.run); changed || err != nil {
		t.Fatalf("second Ensure changed=%v %v", changed, err)
	}
	if st, _ := Load(p); st.Previous != "firefox.desktop" {
		t.Fatalf("remembered %+v after a second Ensure", st)
	}
	if on, _, _ := Status(ctx, p, m.run); !on {
		t.Fatal("status says off")
	}

	if err := Disable(ctx, p, m.run); err != nil || m.def != "firefox.desktop" {
		t.Fatalf("Disable: %v, default %s", err, m.def)
	}
	if _, err := os.Stat(filepath.Join(p.DataHome, "applications", DesktopID)); !os.IsNotExist(err) {
		t.Fatalf("the user entry is still there: %v", err)
	}
	// Turned off by the owner: the coordinator's Ensure leaves it alone.
	if changed, _ := Ensure(ctx, p, "/x", m.run); changed || m.def != "firefox.desktop" {
		t.Fatalf("Ensure after Disable took over: %s", m.def)
	}
	if changed, err := Enable(ctx, p, "/x", m.run); !changed || err != nil || m.def != DesktopID {
		t.Fatalf("Enable: %v %v %s", changed, err, m.def)
	}
}

func TestEnsureWithThePackagedEntryWritesNone(t *testing.T) {
	p := testPaths(t)
	if err := os.MkdirAll(filepath.Dir(p.System), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.System, []byte("[Desktop Entry]\nExec=/usr/bin/bear-den-tv open-url %u\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A leftover entry from a checkout would shadow the packaged one.
	user := filepath.Join(p.DataHome, "applications", DesktopID)
	_ = os.MkdirAll(filepath.Dir(user), 0o755)
	_ = os.WriteFile(user, []byte("[Desktop Entry]\nExec=/old/checkout open-url %u\n"), 0o644)
	m := &fakeMime{def: "firefox.desktop"}
	if _, err := Ensure(context.Background(), p, "/ignored", m.run); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(user); !os.IsNotExist(err) {
		t.Fatal("the checkout's entry was kept beside the packaged one")
	}
}

func TestForwardRunsThePreviousBrowsersEntry(t *testing.T) {
	p := testPaths(t)
	apps := filepath.Join(p.DataDirs[0], "applications")
	_ = os.MkdirAll(apps, 0o755)
	_ = os.WriteFile(filepath.Join(apps, "com.brave.Browser.desktop"), []byte(
		"[Desktop Entry]\nName=Brave\nExec=/usr/bin/flatpak run --branch=stable --command=brave --file-forwarding com.brave.Browser @@u %U @@\n"+
			"[Desktop Action new-window]\nExec=/usr/bin/flatpak run com.brave.Browser --new-window\n"), 0o644)
	m := &fakeMime{def: "com.brave.Browser.desktop"}
	if _, err := Ensure(context.Background(), p, "/x", m.run); err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := Forward(p, "https://example.org/a?b=c", func(argv []string) error { got = argv; return nil }); err != nil {
		t.Fatal(err)
	}
	want := []string{"/usr/bin/flatpak", "run", "--branch=stable", "--command=brave", "--file-forwarding", "com.brave.Browser", "@@u", "https://example.org/a?b=c", "@@"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("forwarded with %q", got)
	}
}

func TestExecArgv(t *testing.T) {
	cases := []struct {
		exec string
		want []string
	}{
		{"firefox %u", []string{"firefox", "L"}},
		{"chromium --new-window", []string{"chromium", "--new-window", "L"}},
		{`"/opt/My Browser/run" --icon %i %U`, []string{"/opt/My Browser/run", "--icon", "L"}},
		{`sh -c "echo \\"$1\\" 100%%" sh %u`, []string{"sh", "-c", `echo "$1" 100%`, "sh", "L"}},
		{`browser --url=%u`, []string{"browser", "--url=L"}},
	}
	for _, c := range cases {
		got, err := ExecArgv(c.exec, "L")
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q %v, want %q", c.exec, got, err, c.want)
		}
	}
	if _, err := ExecArgv(`"unterminated %u`, "L"); err == nil {
		t.Error("an unterminated quote was accepted")
	}
	// Quoting a path for an Exec line round-trips.
	for _, path := range []string{"/usr/bin/x", "/opt/a b/x", `/opt/q"uo$te\x`} {
		got, err := ExecArgv(quoteExec(path)+" open-url %u", "L")
		if err != nil || !reflect.DeepEqual(got, []string{path, "open-url", "L"}) {
			t.Errorf("quote %q: %q %v", path, got, err)
		}
	}
	if _, err := ExecFor(Paths{}, "../evil.desktop", "L"); err == nil {
		t.Error("a path was accepted as a desktop id")
	}
}
