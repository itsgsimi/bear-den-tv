// Setup messages (contracts/ipc.md onboarding.complete, autostart.configure):
// the TV's first-run setup marks itself done (config onboarding.completed),
// and its "Start with this PC" toggle writes or removes the user's autostart
// entry. Both are answered with Result. The shell and the cli are the
// trusted senders; the toggle on the TV is the owner's consent.

package shellipc

// Setup message type names.
const (
	TypeOnboardingComplete = "onboarding.complete"
	TypeAutostartConfigure = "autostart.configure"
)

// OnboardingComplete sets config onboarding.completed to true.
type OnboardingComplete struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

// Kind implements Message.
func (OnboardingComplete) Kind() string { return TypeOnboardingComplete }

// AutostartConfigure writes (Enabled) or removes the user's XDG autostart
// entry, the same one `bear-den-tv autostart enable` writes.
type AutostartConfigure struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Enabled   bool   `json:"enabled"`
}

// Kind implements Message.
func (AutostartConfigure) Kind() string { return TypeAutostartConfigure }

// decodeSetup returns an empty message for a setup type, or nil.
func decodeSetup(t string) Message {
	switch t {
	case TypeOnboardingComplete:
		return &OnboardingComplete{}
	case TypeAutostartConfigure:
		return &AutostartConfigure{}
	}
	return nil
}

// derefSetup returns the value form of a setup message, or nil.
func derefSetup(m Message) Message {
	switch t := m.(type) {
	case *OnboardingComplete:
		return *t
	case *AutostartConfigure:
		return *t
	}
	return nil
}
