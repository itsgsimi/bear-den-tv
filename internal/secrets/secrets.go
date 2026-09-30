// Package secrets stores optional connector tokens (the Plex content
// connector's account token) outside config.json, exports, logs, and phone
// payloads. Configuration only ever holds an opaque connection_ref; the value
// behind it lives in the desktop Secret Service when one is usable (dbus.go),
// otherwise in a private file only this user can read (file.go), chosen by
// Fallback (fallback.go): a TV that logs in automatically never unlocks its
// login keyring. Memory and Unavailable serve tests and dev runs.
//
// Every failure is explained: callers get ErrLocked or ErrUnavailable with
// a user-facing reason. Nothing here logs values or puts them in errors.
// Spec: docs/security.md "Plex sign-in".
package secrets

import (
	"context"
	"errors"
)

var (
	// ErrNotFound reports that no secret is stored under the reference.
	ErrNotFound = errors.New("secret not found")
	// ErrUnavailable reports that no secret store can be reached; the wrapped
	// message names why (no session bus, no Secret Service, no default
	// collection).
	ErrUnavailable = errors.New("secret store unavailable")
	// ErrLocked reports that the store exists but its collection is locked and
	// this process does not drive unlock prompts.
	ErrLocked = errors.New("secret store locked")
)

// Store is the connector-token store. Values are opaque strings; ref is the
// config connection_ref. Implementations never log ref values or secrets.
type Store interface {
	// Get returns the secret behind ref, ErrNotFound when none is stored,
	// ErrLocked when it exists in a locked collection, or ErrUnavailable.
	Get(ctx context.Context, ref string) (string, error)
	// Set stores value under ref, replacing an existing entry. label is the
	// human-readable name shown by keyring UIs.
	Set(ctx context.Context, ref, value, label string) error
	// Delete removes the secret behind ref; ErrNotFound when none exists.
	Delete(ctx context.Context, ref string) error
	// Available reports whether Set/Get can work right now and, when not, a
	// user-facing reason ("the default keyring is locked").
	Available(ctx context.Context) (bool, string)
}
