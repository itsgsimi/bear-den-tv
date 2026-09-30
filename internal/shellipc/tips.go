// Bear tip messages (contracts/ipc.md, tips.*): TV Settings → Home screen →
// Bear tips turns them on or off and Show tips again resets them (answered
// with Result); the shell reports what happened to a tip on Home (no reply,
// shell clients only).

package shellipc

// Bear tip message type names.
const (
	TypeTipsConfigure = "tips.configure"
	TypeTipsReset     = "tips.reset"
	TypeTipsEvent     = "tips.event"
)

// tips.event values.
const (
	// TipEventSeen: the tip's sign reached its place on Home (it counts as
	// the day's tip and is never shown again).
	TipEventSeen = "seen"
	// TipEventOK: the owner pressed OK, "Show me".
	TipEventOK = "ok"
	// TipEventNotNow: the owner pressed Back, "Not now".
	TipEventNotNow = "not_now"
)

// TipsConfigure turns the bear tips on or off (config tips.enabled).
// Answered with Result.
type TipsConfigure struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Enabled   bool   `json:"enabled"`
}

// Kind implements Message.
func (TipsConfigure) Kind() string { return TypeTipsConfigure }

// TipsReset forgets every tip shown and every "Not now", and turns the tips
// on (Show tips again). Answered with Result.
type TipsReset struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

// Kind implements Message.
func (TipsReset) Kind() string { return TypeTipsReset }

// TipsEvent reports a tip shown or answered on Home (TipEvent*). No reply.
type TipsEvent struct {
	Type  string `json:"type"`
	Tip   string `json:"tip"`
	Event string `json:"event"`
}

// Kind implements Message.
func (TipsEvent) Kind() string { return TypeTipsEvent }

// decodeTips returns an empty message for a tips.* type, or nil.
func decodeTips(t string) Message {
	switch t {
	case TypeTipsConfigure:
		return &TipsConfigure{}
	case TypeTipsReset:
		return &TipsReset{}
	case TypeTipsEvent:
		return &TipsEvent{}
	}
	return nil
}

// derefTips returns the value form of a tips.* message, or nil.
func derefTips(m Message) Message {
	switch t := m.(type) {
	case *TipsConfigure:
		return *t
	case *TipsReset:
		return *t
	case *TipsEvent:
		return *t
	}
	return nil
}
