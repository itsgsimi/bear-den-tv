// Den badge messages (contracts/ipc.md, achievements.*): TV Themes → Den badges
// turns counting on or off and resets it (answered with Result); the shell
// reports celebrations it showed and events only it sees (no reply, shell
// clients only).

package shellipc

// Den badge message type names.
const (
	TypeAchievementsConfigure  = "achievements.configure"
	TypeAchievementsReset      = "achievements.reset"
	TypeAchievementsCelebrated = "achievements.celebrated"
	TypeAchievementsEvent      = "achievements.event"
)

// AchievementEventParade is achievements.event "parade": the remote's secret
// code started the bear parade.
const AchievementEventParade = "parade"

// AchievementsConfigure turns Den badges on or off (config
// achievements.enabled). Answered with Result.
type AchievementsConfigure struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Enabled   bool   `json:"enabled"`
}

// Kind implements Message.
func (AchievementsConfigure) Kind() string { return TypeAchievementsConfigure }

// AchievementsReset deletes every counter and earned badge. Answered with
// Result.
type AchievementsReset struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

// Kind implements Message.
func (AchievementsReset) Kind() string { return TypeAchievementsReset }

// AchievementsCelebrated says the shell showed these badges' celebration on
// Home. No reply.
type AchievementsCelebrated struct {
	Type string   `json:"type"`
	IDs  []string `json:"ids"`
}

// Kind implements Message.
func (AchievementsCelebrated) Kind() string { return TypeAchievementsCelebrated }

// AchievementsEvent reports something only the shell sees, for a badge
// counter (AchievementEventParade). No reply.
type AchievementsEvent struct {
	Type  string `json:"type"`
	Event string `json:"event"`
}

// Kind implements Message.
func (AchievementsEvent) Kind() string { return TypeAchievementsEvent }

// decodeAchievements returns an empty message for an achievements.* type, or
// nil.
func decodeAchievements(t string) Message {
	switch t {
	case TypeAchievementsConfigure:
		return &AchievementsConfigure{}
	case TypeAchievementsReset:
		return &AchievementsReset{}
	case TypeAchievementsCelebrated:
		return &AchievementsCelebrated{}
	case TypeAchievementsEvent:
		return &AchievementsEvent{}
	}
	return nil
}

// derefAchievements returns the value form of an achievements.* message, or
// nil.
func derefAchievements(m Message) Message {
	switch t := m.(type) {
	case *AchievementsConfigure:
		return *t
	case *AchievementsReset:
		return *t
	case *AchievementsCelebrated:
		return *t
	case *AchievementsEvent:
		return *t
	}
	return nil
}
