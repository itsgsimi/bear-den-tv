// state.onboarding and state.autostart: the TV's first-run setup and whether
// Bear Den starts with this PC (contracts/state.schema.json#/properties/onboarding
// and #/properties/autostart, contracts/ipc.md onboarding.complete and
// autostart.configure). Shell view only.

package contract

// Onboarding is state.onboarding: config onboarding.completed.
type Onboarding struct {
	Completed bool `json:"completed"`
}

// Autostart is state.autostart: the user's XDG autostart entry.
type Autostart struct {
	// Enabled: the entry exists.
	Enabled bool `json:"enabled"`
	// Available: the session supports it and the start script was found.
	Available bool `json:"available"`
	// Reason says why it is not available (never a path); empty when Available.
	Reason string `json:"reason,omitempty"`
}
