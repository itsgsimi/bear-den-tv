// The sign-in link message (contracts/ipc.md link.open): `bear-den-tv
// open-url`, the desktop's handler for http and https links while Bear Den
// is set up for them (bear-den-tv links), hands every link an app opens to
// the coordinator. The coordinator answers with Result: data.handled true
// when it opened the link on the TV (the Browser tile), false when the
// command should give it to the desktop's own browser. Only the cli sends it.

package shellipc

// TypeLinkOpen is the sign-in link message type name.
const TypeLinkOpen = "link.open"

// LinkOpen asks the coordinator to open URL on the TV.
type LinkOpen struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	URL       string `json:"url"`
}

// Kind implements Message.
func (LinkOpen) Kind() string { return TypeLinkOpen }

// decodeLinks returns an empty message for the link type, or nil.
func decodeLinks(t string) Message {
	if t == TypeLinkOpen {
		return &LinkOpen{}
	}
	return nil
}

// derefLinks returns the value form of a link message, or nil.
func derefLinks(m Message) Message {
	if t, ok := m.(*LinkOpen); ok {
		return *t
	}
	return nil
}
