// Fallback: the desktop keyring first, a private file when the keyring is
// locked or missing. A TV that logs in automatically never unlocks its
// login keyring (no password is typed), so the keyring stays locked and
// the Plex sign-in would be impossible; Plex HTPC keeps its own account
// token in a plain file for the same reason. Rules:
//
//   - Set: into the keyring when it is usable (and then any file copy is
//     deleted, so exactly one place holds the value); into the private file
//     when the keyring is locked or missing.
//   - Get: the keyring when it holds the value, else the file.
//   - Delete: both places; ErrNotFound only when neither held it.
//   - StoredIn: where the value was last seen, from memory only (no D-Bus
//     call), for the TV's "where is my sign-in kept" line.
//
// Spec: docs/security.md "Plex sign-in".

package secrets

import (
	"context"
	"errors"
	"sync"
)

// Where a value is kept (Fallback.StoredIn; state.plex.stored_in).
const (
	InKeyring = "keyring"
	InFile    = "file"
)

// Locator is implemented by stores that know where a reference's value
// was last seen, without any I/O ("" when unknown).
type Locator interface {
	StoredIn(ref string) string
}

// Fallback is a Store over a keyring and a private file.
type Fallback struct {
	Keyring Store
	File    Store

	mu    sync.Mutex
	where map[string]string
}

// NewFallback returns the keyring-then-file store.
func NewFallback(keyring, file Store) *Fallback {
	return &Fallback{Keyring: keyring, File: file, where: map[string]string{}}
}

func (f *Fallback) note(ref, where string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if where == "" {
		delete(f.where, ref)
		return
	}
	f.where[ref] = where
}

// StoredIn implements Locator.
func (f *Fallback) StoredIn(ref string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.where[ref]
}

// keyringUnusable reports a locked or missing keyring (the file's turn).
func keyringUnusable(err error) bool {
	return errors.Is(err, ErrLocked) || errors.Is(err, ErrUnavailable)
}

// Get implements Store.
func (f *Fallback) Get(ctx context.Context, ref string) (string, error) {
	v, kerr := f.Keyring.Get(ctx, ref)
	if kerr == nil {
		f.note(ref, InKeyring)
		return v, nil
	}
	if !errors.Is(kerr, ErrNotFound) && !keyringUnusable(kerr) {
		return "", kerr
	}
	v, ferr := f.File.Get(ctx, ref)
	if ferr == nil {
		f.note(ref, InFile)
		return v, nil
	}
	if errors.Is(ferr, ErrNotFound) {
		f.note(ref, "")
		if keyringUnusable(kerr) {
			return "", kerr // it may be in the locked keyring
		}
		return "", ErrNotFound
	}
	return "", ferr
}

// Set implements Store.
func (f *Fallback) Set(ctx context.Context, ref, value, label string) error {
	if ok, _ := f.Keyring.Available(ctx); ok {
		err := f.Keyring.Set(ctx, ref, value, label)
		if err == nil {
			f.note(ref, InKeyring)
			if derr := f.File.Delete(ctx, ref); derr != nil && !errors.Is(derr, ErrNotFound) {
				return derr
			}
			return nil
		}
		if !keyringUnusable(err) {
			return err
		}
	}
	if err := f.File.Set(ctx, ref, value, label); err != nil {
		return err
	}
	f.note(ref, InFile)
	return nil
}

// Delete implements Store: both places. A locked keyring cannot be
// checked; when the file held the value that is enough (Set never leaves
// a copy in both), otherwise the keyring's reason is returned.
func (f *Fallback) Delete(ctx context.Context, ref string) error {
	kerr := f.Keyring.Delete(ctx, ref)
	ferr := f.File.Delete(ctx, ref)
	f.note(ref, "")
	switch {
	case ferr != nil && !errors.Is(ferr, ErrNotFound):
		return ferr
	case kerr == nil || ferr == nil:
		return nil
	case errors.Is(kerr, ErrNotFound):
		return ErrNotFound
	case keyringUnusable(kerr):
		return kerr
	}
	return kerr
}

// Available implements Store: the keyring, or else the private file.
// The reason, when neither works, is the file's (the keyring's is
// expected on a TV that logs in automatically).
func (f *Fallback) Available(ctx context.Context) (bool, string) {
	if ok, _ := f.Keyring.Available(ctx); ok {
		return true, ""
	}
	return f.File.Available(ctx)
}
