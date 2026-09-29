// config.cec: TV control over HDMI-CEC (contracts/config.md "TV control over
// HDMI-CEC"). Optional: an absent block is off with the PC's volume.

package config

import "bear-den-tv/internal/contract"

// CEC is config.cec.
type CEC struct {
	// Enabled lets Bear Den send HDMI-CEC messages to the TV.
	Enabled bool `json:"enabled"`
	// VolumeTarget is contract.VolumeTargetPC or VolumeTargetTV: which volume
	// the phone's volume buttons change while Enabled.
	VolumeTarget string `json:"volume_target"`
}

// CECSettings returns the cec block; an absent block is off, PC volume.
func (c Config) CECSettings() CEC {
	if c.CEC == nil {
		return CEC{VolumeTarget: contract.VolumeTargetPC}
	}
	return *c.CEC
}

// TVVolume reports whether the phone's volume buttons drive the TV: CEC
// enabled and volume_target tv.
func (c CEC) TVVolume() bool { return c.Enabled && c.VolumeTarget == contract.VolumeTargetTV }

func validateCEC(c Config, errs *Errors) {
	if c.CEC == nil {
		return
	}
	if t := c.CEC.VolumeTarget; t != contract.VolumeTargetPC && t != contract.VolumeTargetTV {
		errs.add("cec.volume_target must be pc or tv")
	}
}
