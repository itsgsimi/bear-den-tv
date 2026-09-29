// Key to keysym to keycode mapping for XTEST input (a closed table).

package x11

import (
	"github.com/jezek/xgb/xproto"

	"bear-den-tv/internal/platform"
)

// X keysyms (keysymdef.h / XF86keysym.h) for the closed set of logical keys.
const (
	keysymLeft         xproto.Keysym = 0xff51
	keysymUp           xproto.Keysym = 0xff52
	keysymRight        xproto.Keysym = 0xff53
	keysymDown         xproto.Keysym = 0xff54
	keysymReturn       xproto.Keysym = 0xff0d
	keysymEscape       xproto.Keysym = 0xff1b
	keysymAudioPlay    xproto.Keysym = 0x1008ff14
	keysymAudioPause   xproto.Keysym = 0x1008ff31
	keysymAudioRewind  xproto.Keysym = 0x1008ff3e
	keysymAudioForward xproto.Keysym = 0x1008ff97
	keysymX            xproto.Keysym = 0x0078
	keysymZ            xproto.Keysym = 0x007a
	keysymP            xproto.Keysym = 0x0070
)

// keysyms is the only key → keysym table; anything absent cannot be injected.
var keysyms = map[platform.Key]xproto.Keysym{
	platform.KeyUp:        keysymUp,
	platform.KeyDown:      keysymDown,
	platform.KeyLeft:      keysymLeft,
	platform.KeyRight:     keysymRight,
	platform.KeySelect:    keysymReturn,
	platform.KeyBack:      keysymEscape,
	platform.KeyPlayPause: keysymAudioPlay,
	platform.KeyPlay:      keysymAudioPlay,
	platform.KeyPause:     keysymAudioPause,
	platform.KeyRewind:    keysymAudioRewind,
	platform.KeyForward:   keysymAudioForward,
	platform.KeyLetterX:   keysymX,
	platform.KeyLetterZ:   keysymZ,
	platform.KeyLetterP:   keysymP,
}

// KeysymFor returns the X keysym for a logical key, or false when the key is
// not in the closed injection set.
func KeysymFor(k platform.Key) (xproto.Keysym, bool) {
	s, ok := keysyms[k]
	return s, ok
}

// keymap is a snapshot of the server's keyboard mapping.
type keymap struct {
	min        xproto.Keycode
	perKeycode int
	keysyms    []xproto.Keysym // len = count*perKeycode, row-major by keycode
}

// newKeymap builds a keymap from a GetKeyboardMapping reply for keycodes
// starting at min.
func newKeymap(min xproto.Keycode, reply *xproto.GetKeyboardMappingReply) *keymap {
	return &keymap{min: min, perKeycode: int(reply.KeysymsPerKeycode), keysyms: reply.Keysyms}
}

// keycodeFor resolves keysym to the lowest keycode that produces it,
// preferring an unshifted (column 0) binding over any other column.
func (m *keymap) keycodeFor(keysym xproto.Keysym) (xproto.Keycode, bool) {
	if m == nil || m.perKeycode <= 0 {
		return 0, false
	}
	count := len(m.keysyms) / m.perKeycode
	var fallback xproto.Keycode
	found := false
	for i := 0; i < count; i++ {
		row := m.keysyms[i*m.perKeycode : (i+1)*m.perKeycode]
		code := xproto.Keycode(int(m.min) + i)
		if int(m.min)+i > 255 {
			break
		}
		if row[0] == keysym {
			return code, true
		}
		if !found {
			for _, s := range row[1:] {
				if s == keysym {
					fallback, found = code, true
					break
				}
			}
		}
	}
	return fallback, found
}
