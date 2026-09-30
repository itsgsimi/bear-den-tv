// File: connector tokens in a private file, the fallback when the desktop
// keyring is locked or missing (a TV that logs in automatically never
// unlocks its login keyring). One file per reference,
// <Dir>/<Prefix><ref> ($XDG_DATA_HOME/bear-den-tv/secrets/plex-<ref> for
// Plex), in a folder only this user can open (0700) and a file only this
// user can read (0600), written through a temporary file in the same
// folder, fsync and a rename. A folder or file that is a symbolic link, or
// that another user owns, is refused with the reason; loose permissions on
// our own folder or file are tightened. Values are never logged, and the
// error texts never carry them. Spec: docs/security.md "Plex sign-in".

package secrets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
)

// maxFileSecret bounds what Get reads (a Plex token is about 20 bytes).
const maxFileSecret = 64 << 10

var refPattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]{0,63}$`)

// File is the private-file Store.
type File struct {
	// Dir is the secrets folder ($XDG_DATA_HOME/bear-den-tv/secrets).
	Dir string
	// Prefix starts every file name ("plex-").
	Prefix string
}

// NewFile returns a private-file store in dir with file names prefix+ref.
func NewFile(dir, prefix string) *File { return &File{Dir: dir, Prefix: prefix} }

func (f *File) path(ref string) (string, error) {
	if !refPattern.MatchString(ref) || !filepath.IsAbs(f.Dir) {
		return "", fmt.Errorf("%w: that sign-in has a name Bear Den cannot use as a file name", ErrUnavailable)
	}
	return filepath.Join(f.Dir, f.Prefix+ref), nil
}

func refusedf(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrUnavailable}, a...)...)
}

// ownedByMe reports whether fi belongs to this process's user.
func ownedByMe(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

// dir makes the secrets folder if missing (0700) and checks it: a real
// folder (not a link) owned by this user; group or other access is
// removed.
func (f *File) dir() error {
	if !filepath.IsAbs(f.Dir) {
		return refusedf("the private folder for sign-ins has no usable path")
	}
	fi, err := os.Lstat(f.Dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(f.Dir), 0o700); err != nil {
			return refusedf("the private folder for sign-ins could not be made (%v)", err)
		}
		if err := os.Mkdir(f.Dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return refusedf("the private folder for sign-ins could not be made (%v)", err)
		}
		fi, err = os.Lstat(f.Dir)
	}
	if err != nil {
		return refusedf("the private folder for sign-ins cannot be read (%v)", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return refusedf("the private folder for sign-ins (%s) is a link to somewhere else, so Bear Den will not use it", f.Dir)
	}
	if !fi.IsDir() {
		return refusedf("%s is not a folder", f.Dir)
	}
	if !ownedByMe(fi) {
		return refusedf("the private folder for sign-ins (%s) belongs to another user", f.Dir)
	}
	if fi.Mode().Perm() != 0o700 {
		if err := os.Chmod(f.Dir, 0o700); err != nil {
			return refusedf("the private folder for sign-ins could not be made private (%v)", err)
		}
	}
	return nil
}

// checkFile refuses a path that exists as anything but our own regular
// file. It reports whether the file exists.
func checkFile(path string) (bool, error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, refusedf("the sign-in file cannot be read (%v)", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return true, refusedf("the sign-in file %s is a link to somewhere else, so Bear Den will not use it", path)
	}
	if !fi.Mode().IsRegular() {
		return true, refusedf("%s is not a file", path)
	}
	if !ownedByMe(fi) {
		return true, refusedf("the sign-in file %s belongs to another user", path)
	}
	return true, nil
}

// Get implements Store.
func (f *File) Get(_ context.Context, ref string) (string, error) {
	path, err := f.path(ref)
	if err != nil {
		return "", err
	}
	if err := f.dir(); err != nil {
		return "", err
	}
	exists, err := checkFile(path)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", ErrNotFound
	}
	fd, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", refusedf("the sign-in file cannot be read (%v)", err)
	}
	defer fd.Close()
	if fi, err := fd.Stat(); err == nil && fi.Mode().Perm() != 0o600 {
		if err := fd.Chmod(0o600); err != nil {
			return "", refusedf("the sign-in file could not be made private (%v)", err)
		}
	}
	raw, err := io.ReadAll(io.LimitReader(fd, maxFileSecret+1))
	if err != nil {
		return "", refusedf("the sign-in file cannot be read (%v)", err)
	}
	if len(raw) == 0 || len(raw) > maxFileSecret {
		return "", ErrNotFound
	}
	return string(raw), nil
}

// Set implements Store: a temporary file (0600) in the same folder,
// written and synced, then renamed over the old one, and the folder synced.
func (f *File) Set(_ context.Context, ref, value, _ string) error {
	path, err := f.path(ref)
	if err != nil {
		return err
	}
	if value == "" || len(value) > maxFileSecret {
		return refusedf("that sign-in cannot be kept")
	}
	if err := f.dir(); err != nil {
		return err
	}
	if _, err := checkFile(path); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(f.Dir, ".tmp-"+f.Prefix+"*")
	if err != nil {
		return refusedf("the sign-in file could not be written (%v)", err)
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return refusedf("the sign-in file could not be written (%v)", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fail(err)
	}
	if _, err := tmp.WriteString(value); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return refusedf("the sign-in file could not be written (%v)", err)
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return refusedf("the sign-in file could not be written (%v)", err)
	}
	if d, err := os.Open(f.Dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

// Delete implements Store.
func (f *File) Delete(_ context.Context, ref string) error {
	path, err := f.path(ref)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(f.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return refusedf("the private folder for sign-ins (%s) is a link to somewhere else, so Bear Den will not use it", f.Dir)
	}
	exists, err := checkFile(path)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if err := os.Remove(path); err != nil {
		return refusedf("the sign-in file could not be deleted (%v)", err)
	}
	return nil
}

// Available implements Store: the private folder exists (or was made) and
// is ours.
func (f *File) Available(context.Context) (bool, string) {
	if err := f.dir(); err != nil {
		return false, reason(err)
	}
	return true, ""
}
