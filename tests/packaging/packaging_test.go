// Package packaging_test checks the .deb's moving parts that need no build
// (docs/operations.md → Packaging): the Debian version mapping in
// packaging/version.sh, scripts/start-session.sh finding the coordinator in
// both the checkout and the installed layout, and packaging/nfpm.yaml
// installing the start script where the autostart and menu entries run it,
// without enabling autostart. The package build and the install/remove smoke
// test in clean containers are `make package` and packaging/smoke-deb.sh.
package packaging_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestVersionMapping(t *testing.T) {
	script := filepath.Join(repoRoot(t), "packaging", "version.sh")
	debian := regexp.MustCompile(`^[0-9][A-Za-z0-9.+~]*$`)
	for describe, want := range map[string]string{
		"v1.2.0":                  "1.2.0",
		"1.2.0":                   "1.2.0",
		"v1.2.0-dirty":            "1.2.0+dirty",
		"v1.2.0-3-gabc1234":       "1.2.0+git3.gabc1234",
		"v1.2.0-3-gabc1234-dirty": "1.2.0+git3.gabc1234.dirty",
		"v1.2.0-rc1":              "1.2.0~rc1",
		"v1.2.0-rc1-2-gdef5678":   "1.2.0~rc1+git2.gdef5678",
		"abc1234":                 "0.1.0~git.abc1234",
		"5ea4119-dirty":           "0.1.0~git.5ea4119.dirty",
		"1234567":                 "0.1.0~git.1234567",
	} {
		cmd := exec.Command("bash", script, describe)
		cmd.Env = append(os.Environ(), "VERSION=")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", describe, err)
		}
		got := strings.TrimSpace(string(out))
		if got != want {
			t.Errorf("%s: got %q, want %q", describe, got, want)
		}
		if !debian.MatchString(got) {
			t.Errorf("%s: %q is not a Debian upstream version", describe, got)
		}
	}
	cmd := exec.Command("bash", script, "v1.2.0")
	cmd.Env = append(os.Environ(), "VERSION=9.9.9")
	if out, err := cmd.Output(); err != nil || strings.TrimSpace(string(out)) != "9.9.9" {
		t.Fatalf("VERSION override: %q %v", out, err)
	}
}

// startSession copies scripts/start-session.sh to script (creating the
// layout around it) and runs `start-session.sh which`.
func startSession(t *testing.T, script string, executables ...string) (string, error) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "start-session.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range append([]string{script}, executables...) {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(script, src, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("bash", script, "which").CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func TestStartSessionCheckoutLayout(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	bin := filepath.Join(repo, "build", "bin", "bear-den-tv")
	got, err := startSession(t, filepath.Join(repo, "scripts", "start-session.sh"), bin)
	want := bin + "\n" + filepath.Join(repo, "build", "bin", "bear-den-tv-shell")
	if err != nil || got != want {
		t.Fatalf("checkout: got %q (%v), want %q", got, err, want)
	}
}

// The .deb layout: /usr/lib/bear-den-tv/start-session.sh runs /usr/bin/bear-den-tv.
func TestStartSessionInstalledLayout(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "usr")
	bin := filepath.Join(prefix, "bin", "bear-den-tv")
	got, err := startSession(t, filepath.Join(prefix, "lib", "bear-den-tv", "start-session.sh"), bin)
	want := bin + "\n" + filepath.Join(prefix, "bin", "bear-den-tv-shell")
	if err != nil || got != want {
		t.Fatalf("installed: got %q (%v), want %q", got, err, want)
	}
}

func TestStartSessionNoBinaryFailsWithReason(t *testing.T) {
	got, err := startSession(t, filepath.Join(t.TempDir(), "x", "scripts", "start-session.sh"))
	if err == nil || !strings.Contains(got, "checkout") || !strings.Contains(got, "installed") {
		t.Fatalf("want a failure naming both layouts, got %q (%v)", got, err)
	}
}

// nfpmEntry is one `contents` item of packaging/nfpm.yaml.
type nfpmEntry struct{ src, dst, typ string }

// nfpmContents reads the contents list with a line parser (the file is flat
// enough; no YAML dependency for one test).
func nfpmContents(t *testing.T) []nfpmEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "packaging", "nfpm.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var out []nfpmEntry
	for _, line := range strings.Split(string(raw), "\n") {
		l := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(l, "- src: "):
			out = append(out, nfpmEntry{src: strings.TrimPrefix(l, "- src: ")})
		case strings.HasPrefix(l, "dst: ") && len(out) > 0:
			out[len(out)-1].dst = strings.TrimPrefix(l, "dst: ")
		case strings.HasPrefix(l, "type: ") && len(out) > 0:
			out[len(out)-1].typ = strings.TrimPrefix(l, "type: ")
		}
	}
	if len(out) == 0 {
		t.Fatal("no contents in nfpm.yaml")
	}
	return out
}

func TestNfpmContents(t *testing.T) {
	root := repoRoot(t)
	dst := map[string]nfpmEntry{}
	for _, e := range nfpmContents(t) {
		dst[e.dst] = e
		if strings.HasPrefix(e.dst, "/etc/xdg/autostart") || strings.Contains(e.dst, "/.config/autostart") {
			t.Errorf("the package must not enable autostart: %s", e.dst)
		}
		// Repository files must exist; build/ files are staged by build-deb.sh.
		if e.typ != "symlink" && !strings.HasPrefix(e.src, "build/") {
			if _, err := os.Stat(filepath.Join(root, e.src)); err != nil {
				t.Errorf("missing source %s", e.src)
			}
		}
	}
	const start = "/usr/lib/bear-den-tv/start-session.sh"
	if dst[start].src != "scripts/start-session.sh" {
		t.Errorf("%s must come from scripts/start-session.sh, got %+v", start, dst[start])
	}
	if dst["/usr/bin/bear-den-tv"].src == "" || dst["/usr/bin/bear-den-tv-shell"].typ != "symlink" {
		t.Errorf("want /usr/bin/bear-den-tv and a /usr/bin/bear-den-tv-shell symlink: %+v", dst)
	}
	// The entries the package ships run the installed start script.
	for _, d := range []string{"/usr/share/applications/bear-den-tv.desktop", "/usr/share/bear-den-tv/autostart/bear-den-tv.desktop"} {
		raw, err := os.ReadFile(filepath.Join(root, dst[d].src))
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		if !strings.Contains(string(raw), "\nExec="+start+" --watch\n") {
			t.Errorf("%s does not run %s --watch:\n%s", d, start, raw)
		}
	}
}
