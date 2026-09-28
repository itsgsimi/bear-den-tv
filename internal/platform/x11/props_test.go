// Tests for property decoders and the keysym table (props.go, keys.go).

package x11

import (
	"reflect"
	"testing"

	"github.com/jezek/xgb/xproto"

	"bear-den-tv/internal/platform"
)

func le32(vals ...uint32) []byte {
	out := make([]byte, 0, 4*len(vals))
	for _, v := range vals {
		out = append(out, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	return out
}

func TestDecodeWindowsAndAtoms(t *testing.T) {
	if got := decodeWindows(le32(0x2a00003, 0x1c00001)); !reflect.DeepEqual(got, []xproto.Window{0x2a00003, 0x1c00001}) {
		t.Fatalf("%v", got)
	}
	if got := decodeWindows(nil); got != nil {
		t.Fatalf("%v", got)
	}
	if got := decodeWindows([]byte{1, 2, 3}); got != nil {
		t.Fatalf("short value decoded to %v", got)
	}
	atoms := decodeAtoms(le32(300, 301))
	if !containsAtom(atoms, 301) || containsAtom(atoms, 302) {
		t.Fatalf("%v", atoms)
	}
	if v, ok := decodeCardinal(le32(4242)); !ok || v != 4242 {
		t.Fatalf("%v %v", v, ok)
	}
	if _, ok := decodeCardinal(nil); ok {
		t.Fatal("empty cardinal decoded")
	}
}

func TestDecodeWMClassAndStrings(t *testing.T) {
	if got := decodeWMClass([]byte("plex htpc\x00Plex HTPC\x00")); !reflect.DeepEqual(got, []string{"plex htpc", "Plex HTPC"}) {
		t.Fatalf("%q", got)
	}
	if got := decodeWMClass([]byte("vacuumtube\x00VacuumTube")); !reflect.DeepEqual(got, []string{"vacuumtube", "VacuumTube"}) {
		t.Fatalf("%q", got)
	}
	if got := decodeWMClass(nil); got != nil {
		t.Fatalf("%q", got)
	}
	if got := decodeWMClass([]byte("\x00\x00")); got != nil {
		t.Fatalf("%q", got)
	}
	if got := decodeString([]byte("Plex HTPC\x00\x00")); got != "Plex HTPC" {
		t.Fatalf("%q", got)
	}
}

func TestKeysymTableIsClosed(t *testing.T) {
	for _, k := range []platform.Key{platform.KeyUp, platform.KeyDown, platform.KeyLeft, platform.KeyRight, platform.KeySelect, platform.KeyBack, platform.KeyPlayPause, platform.KeyPlay, platform.KeyPause, platform.KeyRewind, platform.KeyForward} {
		if _, ok := KeysymFor(k); !ok {
			t.Errorf("%s has no keysym", k)
		}
	}
	if _, ok := KeysymFor(platform.Key("a")); ok {
		t.Fatal("text key must not resolve")
	}
	if _, ok := KeysymFor(platform.Key("")); ok {
		t.Fatal("empty key must not resolve")
	}
}

func TestKeycodeForPrefersUnshiftedColumn(t *testing.T) {
	// Keycodes 8..12, 4 keysyms per keycode; XF86AudioPause appears only in
	// column 2 of keycode 9 and in column 0 of keycode 11.
	reply := &xproto.GetKeyboardMappingReply{KeysymsPerKeycode: 4, Keysyms: []xproto.Keysym{
		0, 0, 0, 0, // 8
		keysymEscape, 0, keysymAudioPause, 0, // 9
		keysymLeft, keysymLeft, 0, 0, // 10
		keysymAudioPause, 0, 0, 0, // 11
		keysymReturn, 0, 0, 0, // 12
	}}
	m := newKeymap(8, reply)
	cases := []struct {
		sym  xproto.Keysym
		want xproto.Keycode
		ok   bool
	}{
		{keysymEscape, 9, true},
		{keysymLeft, 10, true},
		{keysymAudioPause, 11, true},
		{keysymReturn, 12, true},
		{keysymAudioForward, 0, false},
	}
	for _, tc := range cases {
		got, ok := m.keycodeFor(tc.sym)
		if got != tc.want || ok != tc.ok {
			t.Errorf("keycodeFor(%#x)=%d,%v want %d,%v", tc.sym, got, ok, tc.want, tc.ok)
		}
	}
	// Column-only binding is still usable when nothing has it unshifted.
	m2 := newKeymap(8, &xproto.GetKeyboardMappingReply{KeysymsPerKeycode: 2, Keysyms: []xproto.Keysym{0, keysymUp}})
	if got, ok := m2.keycodeFor(keysymUp); !ok || got != 8 {
		t.Fatalf("%d %v", got, ok)
	}
	var nilMap *keymap
	if _, ok := nilMap.keycodeFor(keysymUp); ok {
		t.Fatal("nil keymap resolved")
	}
}
