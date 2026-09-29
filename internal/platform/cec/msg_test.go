// Tests for msg.go against the byte layouts of the HDMI 1.4b CEC
// specification: header = initiator << 4 | destination, then the opcode and
// its operands. Bear Den as playback device 1 is logical address 4.

package cec

import (
	"bytes"
	"testing"

	"bear-den-tv/internal/platform"
)

func TestMessageBytesFollowTheSpec(t *testing.T) {
	const pb1 = 4 // Playback Device 1
	cases := []struct {
		name string
		got  []byte
		want []byte
	}{
		// <Image View On> 0x04, directly addressed to the TV (0).
		{"Image View On", imageViewOn(pb1), []byte{0x40, 0x04}},
		// <Standby> 0x36 to the TV only (never broadcast).
		{"Standby", standby(pb1), []byte{0x40, 0x36}},
		// <Active Source> 0x82 [Physical Address], broadcast (15); 1.0.0.0.
		{"Active Source 1.0.0.0", activeSource(pb1, 0x1000), []byte{0x4f, 0x82, 0x10, 0x00}},
		{"Active Source 2.1.0.0", activeSource(pb1, 0x2100), []byte{0x4f, 0x82, 0x21, 0x00}},
		// <User Control Pressed> 0x44 [UI Command], <User Control Released> 0x45.
		{"Volume Up to the TV", userControlPressed(pb1, addrTV, uiVolumeUp), []byte{0x40, 0x44, 0x41}},
		{"Volume Down to the audio system", userControlPressed(pb1, addrAudioSystem, uiVolumeDown), []byte{0x45, 0x44, 0x42}},
		{"Mute Function", userControlPressed(pb1, addrTV, uiMuteFunction), []byte{0x40, 0x44, 0x65}},
		{"Restore Volume Function", userControlPressed(pb1, addrTV, uiRestoreVolumeFunction), []byte{0x40, 0x44, 0x66}},
		{"Released", userControlReleased(pb1, addrAudioSystem), []byte{0x45, 0x45}},
		// <Give Device Power Status> 0x8F to the TV.
		{"Give Device Power Status", giveDevicePowerStatus(pb1), []byte{0x40, 0x8f}},
		// A polling message is the header block alone.
		{"Poll the audio system", poll(pb1, addrAudioSystem), []byte{0x45}},
		// Another claimed address (Playback Device 2 = 8) changes the initiator only.
		{"Image View On from 8", imageViewOn(8), []byte{0x80, 0x04}},
	}
	for _, c := range cases {
		if !bytes.Equal(c.got, c.want) {
			t.Errorf("%s: % x, want % x", c.name, c.got, c.want)
		}
	}
}

func TestVolumeKeysMapToUserControlCodes(t *testing.T) {
	want := map[platform.TVKey]byte{platform.TVVolumeUp: 0x41, platform.TVVolumeDown: 0x42, platform.TVMute: 0x65, platform.TVUnmute: 0x66}
	for k, code := range want {
		if got, ok := uiCode(k); !ok || got != code {
			t.Errorf("%s: %#x %v, want %#x", k, got, ok, code)
		}
	}
	if _, ok := uiCode("power"); ok {
		t.Error("an unknown key mapped to a code")
	}
}

func TestReportPowerStatus(t *testing.T) {
	cases := map[byte]platform.TVPower{0: platform.TVPowerOn, 1: platform.TVPowerStandby, 2: platform.TVPowerOn, 3: platform.TVPowerStandby, 9: platform.TVPowerUnknown}
	for status, want := range cases {
		if got := parsePowerStatus([]byte{0x04, 0x90, status}); got != want {
			t.Errorf("status %d: %s, want %s", status, got, want)
		}
	}
	if got := parsePowerStatus([]byte{0x04, 0x00, 0x8f}); got != platform.TVPowerUnknown { // Feature Abort
		t.Errorf("a Feature Abort read as %s", got)
	}
	if got := parsePowerStatus([]byte{0x04, 0x90}); got != platform.TVPowerUnknown {
		t.Errorf("a short reply read as %s", got)
	}
}
