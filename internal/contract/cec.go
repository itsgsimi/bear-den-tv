// state.cec and the tv.power action: TV control over HDMI-CEC
// (contracts/state.schema.json#/properties/cec, contracts/http.md
// "TV control over HDMI-CEC", contracts/actions.md tv.power).

package contract

// HDMI-CEC TV power states (state.cec.tv_power).
const (
	TVPowerOn      = "on"
	TVPowerStandby = "standby"
	TVPowerUnknown = "unknown"
)

// Volume targets (state.cec.volume_target, config cec.volume_target).
const (
	VolumeTargetPC = "pc"
	VolumeTargetTV = "tv"
)

// tv.power args (args.power).
const (
	TVPowerArgOn      = "on"
	TVPowerArgStandby = "standby"
)

// CEC is state.cec: whether an HDMI-CEC adapter is usable, the owner's
// setting, and what the TV last said about its power.
type CEC struct {
	Available bool `json:"available"`
	// Reason says why it is not available; empty when Available.
	Reason       string `json:"reason,omitempty"`
	Enabled      bool   `json:"enabled"`
	VolumeTarget string `json:"volume_target"` // pc | tv
	TVPower      string `json:"tv_power"`      // on | standby | unknown
}
