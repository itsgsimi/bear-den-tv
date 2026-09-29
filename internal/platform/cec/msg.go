// HDMI-CEC message encoding (HDMI 1.4b Supplement 1, CEC 1.4): the header
// byte is initiator << 4 | destination, then the opcode and its operands.
// Only the messages Bear Den sends are here (ADR 0008); msg_test.go checks
// each byte layout against the specification.

package cec

import "bear-den-tv/internal/platform"

// Logical addresses (CEC 1.4 table 5).
const (
	addrTV          = 0x0
	addrAudioSystem = 0x5
	addrBroadcast   = 0xf
)

// Opcodes.
const (
	opImageViewOn           = 0x04
	opStandby               = 0x36
	opActiveSource          = 0x82
	opUserControlPressed    = 0x44
	opUserControlReleased   = 0x45
	opGiveDevicePowerStatus = 0x8f
	opReportPowerStatus     = 0x90
)

// User Control codes (CEC 1.4 table 27).
const (
	uiVolumeUp              = 0x41
	uiVolumeDown            = 0x42
	uiMuteFunction          = 0x65
	uiRestoreVolumeFunction = 0x66
)

// Power status operands of Report Power Status.
const (
	powerOn          = 0
	powerStandby     = 1
	powerToOn        = 2
	powerToStandby   = 3
	reportPowerBytes = 3 // header, opcode, status
)

func header(from, to byte) byte { return from<<4 | to&0x0f }

// poll is the header-only message that asks whether address to is taken.
func poll(from, to byte) []byte { return []byte{header(from, to)} }

// imageViewOn turns the TV on (and out of a menu or text view).
func imageViewOn(from byte) []byte { return []byte{header(from, addrTV), opImageViewOn} }

// standby puts the TV into standby (sent to the TV only, never broadcast:
// a broadcast Standby turns off every device on the bus).
func standby(from byte) []byte { return []byte{header(from, addrTV), opStandby} }

// activeSource tells every device that physical address pa is the source
// being watched; the TV switches to that input.
func activeSource(from byte, pa uint16) []byte {
	return []byte{header(from, addrBroadcast), opActiveSource, byte(pa >> 8), byte(pa)}
}

func userControlPressed(from, to, code byte) []byte {
	return []byte{header(from, to), opUserControlPressed, code}
}

func userControlReleased(from, to byte) []byte {
	return []byte{header(from, to), opUserControlReleased}
}

func giveDevicePowerStatus(from byte) []byte {
	return []byte{header(from, addrTV), opGiveDevicePowerStatus}
}

// uiCode maps a volume key to its User Control code.
func uiCode(k platform.TVKey) (byte, bool) {
	switch k {
	case platform.TVVolumeUp:
		return uiVolumeUp, true
	case platform.TVVolumeDown:
		return uiVolumeDown, true
	case platform.TVMute:
		return uiMuteFunction, true
	case platform.TVUnmute:
		return uiRestoreVolumeFunction, true
	}
	return 0, false
}

// parsePowerStatus reads a Report Power Status reply; a transition counts
// as the state it is heading to.
func parsePowerStatus(msg []byte) platform.TVPower {
	if len(msg) < reportPowerBytes || msg[1] != opReportPowerStatus {
		return platform.TVPowerUnknown
	}
	switch msg[2] {
	case powerOn, powerToOn:
		return platform.TVPowerOn
	case powerStandby, powerToStandby:
		return platform.TVPowerStandby
	}
	return platform.TVPowerUnknown
}
