// Decoders for X11 window properties (EWMH lists, WM_CLASS, strings).

package x11

import (
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Property decoding for the EWMH/ICCCM values the adapter reads. All decoders
// take the raw GetProperty value bytes (xgb byte order) and never fail: a
// short or malformed value decodes to the empty result.

// decodeWindows decodes a format-32 WINDOW[] property value.
func decodeWindows(v []byte) []xproto.Window {
	n := len(v) / 4
	if n == 0 {
		return nil
	}
	out := make([]xproto.Window, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, xproto.Window(xgb.Get32(v[i*4:])))
	}
	return out
}

// decodeAtoms decodes a format-32 ATOM[] property value.
func decodeAtoms(v []byte) []xproto.Atom {
	n := len(v) / 4
	if n == 0 {
		return nil
	}
	out := make([]xproto.Atom, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, xproto.Atom(xgb.Get32(v[i*4:])))
	}
	return out
}

// decodeCardinal decodes the first element of a format-32 CARDINAL property.
func decodeCardinal(v []byte) (uint32, bool) {
	if len(v) < 4 {
		return 0, false
	}
	return xgb.Get32(v), true
}

// decodeWMClass splits the NUL-separated WM_CLASS value into [instance, class].
func decodeWMClass(v []byte) []string {
	if len(v) == 0 {
		return nil
	}
	parts := strings.Split(strings.TrimRight(string(v), "\x00"), "\x00")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// decodeString decodes a STRING/UTF8_STRING value, dropping NUL padding.
func decodeString(v []byte) string {
	return strings.TrimRight(string(v), "\x00")
}

// containsAtom reports whether a decoded ATOM[] holds atom.
func containsAtom(atoms []xproto.Atom, atom xproto.Atom) bool {
	for _, a := range atoms {
		if a == atom {
			return true
		}
	}
	return false
}
