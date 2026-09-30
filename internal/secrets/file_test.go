// Tests for the private-file store and the keyring-then-file fallback
// (file.go, fallback.go): a locked or missing keyring falls back to the
// file, an unlocked one is used, sign-out deletes whichever holds the
// value, permissions are 0700/0600, symbolic links are refused, and the
// value never appears in an error.

package secrets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testToken = "tok-DEMO-9f8e7d6c5b4a"

func fileStore(t *testing.T) (*File, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bear-den-tv", "secrets")
	return NewFile(dir, "plex-"), dir
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestFileRoundTripIsPrivate(t *testing.T) {
	ctx := context.Background()
	f, dir := fileStore(t)
	if _, err := f.Get(ctx, "plex"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty store Get = %v", err)
	}
	if err := f.Set(ctx, "plex", testToken, "label"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plex-plex")
	if m := mode(t, dir); m != 0o700 {
		t.Fatalf("secrets folder mode %o, want 700", m)
	}
	if m := mode(t, path); m != 0o600 {
		t.Fatalf("secret file mode %o, want 600", m)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("left behind: %v (the temporary file must be renamed)", entries)
	}
	if v, err := f.Get(ctx, "plex"); err != nil || v != testToken {
		t.Fatalf("Get = %q, %v", v, err)
	}
	// Loose permissions on our own folder and file are tightened.
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if v, err := f.Get(ctx, "plex"); err != nil || v != testToken {
		t.Fatalf("Get after chmod = %q, %v", v, err)
	}
	if mode(t, dir) != 0o700 || mode(t, path) != 0o600 {
		t.Fatalf("modes not tightened: %o %o", mode(t, dir), mode(t, path))
	}
	if err := f.Delete(ctx, "plex"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file survived Delete: %v", err)
	}
	if err := f.Delete(ctx, "plex"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete = %v", err)
	}
}

func TestFileRefusesSymlinks(t *testing.T) {
	ctx := context.Background()
	// The folder is a link (to a folder someone else chose).
	base := t.TempDir()
	elsewhere := filepath.Join(base, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "secrets")
	if err := os.Symlink(elsewhere, dir); err != nil {
		t.Fatal(err)
	}
	f := NewFile(dir, "plex-")
	for name, err := range map[string]error{
		"Set":    f.Set(ctx, "plex", testToken, ""),
		"Get":    func() error { _, err := f.Get(ctx, "plex"); return err }(),
		"Delete": f.Delete(ctx, "plex"),
	} {
		if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "is a link to somewhere else") {
			t.Errorf("%s through a linked folder = %v", name, err)
		}
	}
	if ok, reason := f.Available(ctx); ok || !strings.Contains(reason, "is a link to somewhere else") {
		t.Errorf("Available = %v %q", ok, reason)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("wrote through the link: %v", entries)
	}

	// The file is a link.
	g, gdir := fileStore(t)
	if ok, _ := g.Available(ctx); !ok {
		t.Fatal("fresh store unavailable")
	}
	target := filepath.Join(base, "target")
	if err := os.WriteFile(target, []byte("not yours"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(gdir, "plex-plex")); err != nil {
		t.Fatal(err)
	}
	if err := g.Set(ctx, "plex", testToken, ""); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "is a link to somewhere else") {
		t.Errorf("Set over a linked file = %v", err)
	}
	if _, err := g.Get(ctx, "plex"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Get of a linked file = %v", err)
	}
	if raw, _ := os.ReadFile(target); string(raw) != "not yours" {
		t.Fatalf("the link's target changed: %q", raw)
	}
}

func TestFileRefusesOddReferences(t *testing.T) {
	f, _ := fileStore(t)
	for _, ref := range []string{"", "../x", "a/b", ".hidden", strings.Repeat("a", 65)} {
		if err := f.Set(context.Background(), ref, testToken, ""); !errors.Is(err, ErrUnavailable) {
			t.Errorf("ref %q: %v", ref, err)
		}
	}
}

func lockedKeyring() *DBus { b := newFakeBus(); b.locked = true; return NewDBus(b) }

func TestFallbackUsesTheFileWhenTheKeyringIsLockedOrMissing(t *testing.T) {
	ctx := context.Background()
	for name, kr := range map[string]Store{
		"locked":  lockedKeyring(),
		"missing": Unavailable{Reason: "no D-Bus session bus"},
		"no default collection": func() Store {
			b := newFakeBus()
			b.noDefault = true
			return NewDBus(b)
		}(),
	} {
		f, dir := fileStore(t)
		s := NewFallback(kr, f)
		if ok, reason := s.Available(ctx); !ok {
			t.Fatalf("%s: unavailable: %s", name, reason)
		}
		if err := s.Set(ctx, "plex", testToken, "Bear Den TV: Plex sign-in"); err != nil {
			t.Fatalf("%s: Set = %v", name, err)
		}
		if s.StoredIn("plex") != InFile {
			t.Fatalf("%s: stored in %q", name, s.StoredIn("plex"))
		}
		raw, err := os.ReadFile(filepath.Join(dir, "plex-plex"))
		if err != nil || string(raw) != testToken {
			t.Fatalf("%s: file holds %q, %v", name, raw, err)
		}
		// A new process (restart) finds it again.
		s2 := NewFallback(kr, f)
		if v, err := s2.Get(ctx, "plex"); err != nil || v != testToken || s2.StoredIn("plex") != InFile {
			t.Fatalf("%s: Get = %q, %v, %q", name, v, err, s2.StoredIn("plex"))
		}
		if err := s2.Delete(ctx, "plex"); err != nil {
			t.Fatalf("%s: Delete = %v", name, err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "plex-plex")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: sign-out left the file: %v", name, err)
		}
		if s2.StoredIn("plex") != "" {
			t.Fatalf("%s: still says %q", name, s2.StoredIn("plex"))
		}
	}
}

func TestFallbackUsesAnUnlockedKeyring(t *testing.T) {
	ctx := context.Background()
	bus := newFakeBus()
	kr := NewDBus(bus)
	f, dir := fileStore(t)
	// A copy left in the file by an earlier locked session goes away once
	// the keyring holds the value: one place only.
	if err := f.Set(ctx, "plex", "old-"+testToken, ""); err != nil {
		t.Fatal(err)
	}
	s := NewFallback(kr, f)
	if err := s.Set(ctx, "plex", testToken, "Bear Den TV: Plex sign-in"); err != nil {
		t.Fatal(err)
	}
	if s.StoredIn("plex") != InKeyring {
		t.Fatalf("stored in %q", s.StoredIn("plex"))
	}
	if v, err := kr.Get(ctx, "plex"); err != nil || v != testToken {
		t.Fatalf("keyring holds %q, %v", v, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "plex-plex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file copy kept: %v", err)
	}
	if v, err := s.Get(ctx, "plex"); err != nil || v != testToken {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if err := s.Delete(ctx, "plex"); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Get(ctx, "plex"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("keyring kept it after sign-out: %v", err)
	}
	if err := s.Delete(ctx, "plex"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete = %v", err)
	}
}

// A value in a keyring that is locked now cannot be read or deleted; the
// store says so instead of pretending it is gone.
func TestFallbackSaysWhenTheLockedKeyringMayHoldIt(t *testing.T) {
	ctx := context.Background()
	f, _ := fileStore(t)
	kr := NewMemory()
	if err := kr.Set(ctx, "plex", testToken, ""); err != nil {
		t.Fatal(err)
	}
	kr.SetLocked(true)
	s := NewFallback(kr, f)
	if _, err := s.Get(ctx, "plex"); !errors.Is(err, ErrLocked) {
		t.Fatalf("Get = %v, want locked", err)
	}
	if err := s.Delete(ctx, "plex"); !errors.Is(err, ErrLocked) {
		t.Fatalf("Delete = %v, want locked", err)
	}
}

func TestSecretNeverInErrors(t *testing.T) {
	ctx := context.Background()
	f, dir := fileStore(t)
	if err := f.Set(ctx, "plex", testToken, ""); err != nil {
		t.Fatal(err)
	}
	// Make every operation fail and read the errors.
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	var errs []error
	errs = append(errs, f.Set(ctx, "plex", testToken, ""))
	_, err := f.Get(ctx, "plex")
	errs = append(errs, err, f.Delete(ctx, "plex"))
	for _, err := range errs {
		if err != nil && strings.Contains(err.Error(), testToken) {
			t.Fatalf("the value leaked into an error: %v", err)
		}
	}
}
