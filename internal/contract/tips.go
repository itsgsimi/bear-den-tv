// state.tips: the TV's bear tips (contracts/state.schema.json#/properties/tips,
// config tips in contracts/config.md, IPC tips.* in contracts/ipc.md). Shell
// view only.

package contract

// TipIDs are the bear tips, in the order the TV offers them
// (config.schema.json#/$defs/tipId; their words are in the shell's Tips.qml).
var TipIDs = []string{"themes", "add-apps", "phone-remote", "now-playing", "sleep-timer", "badges", "guest-pass"}

// IsTipID reports whether id is one of TipIDs.
func IsTipID(id string) bool {
	for _, t := range TipIDs {
		if t == id {
			return true
		}
	}
	return false
}

// Tips is state.tips: config tips as the shell needs it.
type Tips struct {
	// Enabled is config tips.enabled (Settings → Home screen → Bear tips).
	Enabled bool `json:"enabled"`
	// Done lists the tips already shown; never nil.
	Done []string `json:"done"`
	// Stopped: three "Not now" answers in a row.
	Stopped bool `json:"stopped"`
	// LastDay is the local calendar day (YYYY-MM-DD) a tip was last shown.
	LastDay string `json:"last_day,omitempty"`
}
