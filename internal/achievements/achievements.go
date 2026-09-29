// Package achievements turns local counters into Den badges
// (state.achievements, contracts/http.md#den-badges-stateachievements). The
// badge catalogue is data (Badges); events from the coordinator move named
// counters in internal/storage (schema 3) on the injected clock, and a badge
// is awarded once, on the local calendar day its goal is reached. Only ids,
// counts and days are ever stored or shown: never titles, never times of day
// (docs/security.md#den-badges). While config achievements.enabled is false
// nothing is counted.
package achievements

import (
	"context"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/storage"
)

// Kind says how a badge reads its counter.
type Kind int

const (
	// Count: the counter named Counter reaches Goal.
	Count Kind = iota
	// All: every member of the set named by the Counter prefix has been seen
	// (a counter "<prefix><member>" exists for each). The members come from
	// SetUniverse; an empty set never awards.
	All
)

// Badge is one entry of the catalogue.
type Badge struct {
	ID      string
	Kind    Kind
	Counter string // counter name (Count) or set prefix (All)
	Goal    int    // Count only; All uses the size of the set
}

// Counter names moved by the events below. Names are lowercase ids; set
// members are "<prefix><id>".
const (
	CounterLaunches     = "launches"      // every app launch that succeeded
	PrefixLaunch        = "launch:"       // launches per adapter (launch:plex-htpc)
	PrefixApp           = "app:"          // apps opened at least once (app:<app id>)
	CounterDays         = "days"          // local days Home was shown
	CounterNightOwl     = "night-owl"     // nights Home was shown 23:00-03:59
	CounterEarlyCub     = "early-cub"     // mornings Home was shown 04:00-06:59
	CounterRain         = "rain"          // days Home was shown during rain or drizzle
	CounterSnow         = "snow"          // days Home was shown during snow
	CounterThunder      = "thunder"       // days Home was shown during a thunderstorm
	PrefixSeason        = "season:"       // seasons Home was shown in
	PrefixStyle         = "style:"        // art styles Home was shown in
	PrefixTheme         = "theme:"        // built-in themes Home was shown in
	CounterFamilyPhones = "family-phones" // most family phones paired at once
	CounterGuestPasses  = "guest-passes"  // guest passes issued
	CounterSleepTimers  = "sleep-timers"  // sleep timers set
	CounterParades      = "parades"       // bear parades started with the secret code
)

// Badges is the catalogue, in display order. Adding a badge is a row here
// plus its name, hint and art in the shell and the phone (keyed by ID).
var Badges = []Badge{
	{ID: "first-night-in", Kind: Count, Counter: CounterLaunches, Goal: 1},
	{ID: "movie-night", Kind: Count, Counter: PrefixLaunch + "plex-htpc", Goal: 10},
	{ID: "couch-explorer", Kind: All, Counter: PrefixApp},
	{ID: "night-owl", Kind: Count, Counter: CounterNightOwl, Goal: 5},
	{ID: "early-cub", Kind: Count, Counter: CounterEarlyCub, Goal: 5},
	{ID: "rainy-day", Kind: Count, Counter: CounterRain, Goal: 3},
	{ID: "snow-day", Kind: Count, Counter: CounterSnow, Goal: 1},
	{ID: "thunder-buddy", Kind: Count, Counter: CounterThunder, Goal: 1},
	{ID: "all-seasons", Kind: All, Counter: PrefixSeason},
	{ID: "style-switcher", Kind: All, Counter: PrefixStyle},
	{ID: "theme-tourist", Kind: All, Counter: PrefixTheme},
	{ID: "family-den", Kind: Count, Counter: CounterFamilyPhones, Goal: 2},
	{ID: "good-host", Kind: Count, Counter: CounterGuestPasses, Goal: 1},
	{ID: "sleepy-bear", Kind: Count, Counter: CounterSleepTimers, Goal: 5},
	{ID: "parade-spotter", Kind: Count, Counter: CounterParades, Goal: 1},
	{ID: "loyal-den", Kind: Count, Counter: CounterDays, Goal: 30},
}

// Seasons and Styles are fixed sets; themes and apps come from the
// coordinator (SetUniverse).
var (
	Seasons = []string{"spring", "summer", "autumn", "winter"}
	Styles  = []string{contract.ArtPixel, contract.ArtClassic}
)

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9:-]{0,47}$`)

// Options configures a Tracker.
type Options struct {
	DB    *storage.DB
	Clock clock.Clock
	// Location is the local time zone for calendar days; nil is time.Local.
	Location *time.Location
	// Enabled reports config achievements.enabled; nil means on.
	Enabled func() bool
	// OnChange runs after anything visible changed (a badge earned, a
	// counter moved, a reset) so the coordinator republishes.
	OnChange func()
	Logger   *slog.Logger
}

// Tracker moves counters and awards badges. Methods are safe for concurrent
// use; storage errors are logged and never reach the caller (badges must
// never break the TV).
type Tracker struct {
	o Options

	mu        sync.Mutex
	universes map[string][]string
	cached    *contract.Achievements // last Snapshot; nil after a write
	lastHome  string                 // HomeShown's last key: repeats within it count nothing
}

// New builds a tracker with the fixed sets (seasons, styles).
func New(o Options) *Tracker {
	if o.Clock == nil {
		o.Clock = clock.Real{}
	}
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	t := &Tracker{o: o, universes: map[string][]string{}}
	t.universes[PrefixSeason] = append([]string(nil), Seasons...)
	t.universes[PrefixStyle] = append([]string(nil), Styles...)
	return t
}

// SetUniverse sets the members of an All set (PrefixApp: installed apps,
// PrefixTheme: built-in themes) and re-checks the badges that use it.
func (t *Tracker) SetUniverse(prefix string, members []string) {
	t.mu.Lock()
	t.universes[prefix] = append([]string(nil), members...)
	t.cached = nil
	t.mu.Unlock()
	t.update(func(ctx context.Context, day string) error { return nil })
}

// Enabled reports whether counting is on.
func (t *Tracker) Enabled() bool { return t.o.Enabled == nil || t.o.Enabled() }

// day is the local calendar day of now.
func (t *Tracker) day(now time.Time) string { return now.In(t.o.Location).Format("2006-01-02") }

// update runs one change under the lock when counting is on, then awards
// every badge whose goal is now met.
func (t *Tracker) update(change func(ctx context.Context, day string) error) {
	if !t.Enabled() || t.o.DB == nil {
		return
	}
	ctx := context.Background()
	t.mu.Lock()
	day := t.day(t.o.Clock.Now())
	err := change(ctx, day)
	earned := 0
	if err == nil {
		earned, err = t.awardLocked(ctx, day)
	}
	t.cached = nil
	t.mu.Unlock()
	if err != nil {
		t.o.Logger.Warn("achievements: could not update the badge counters", "err", err)
	}
	if earned > 0 {
		t.o.Logger.Info("achievements: badge earned", "count", earned)
	}
	if t.o.OnChange != nil {
		t.o.OnChange()
	}
}

// bump adds one to a counter.
func (t *Tracker) bump(ctx context.Context, name, day string) error {
	return t.move(ctx, name, day, func(c *storage.Counter) bool { c.Count++; return true })
}

// bumpDay adds one to a counter at most once per local day. A day earlier
// than the last one counted (the clock went back) counts nothing.
func (t *Tracker) bumpDay(ctx context.Context, name, day string) error {
	return t.move(ctx, name, day, func(c *storage.Counter) bool {
		if c.LastDay != nil && day <= *c.LastDay {
			return false
		}
		c.Count++
		return true
	})
}

// mark records a set member once.
func (t *Tracker) mark(ctx context.Context, name, day string) error {
	return t.move(ctx, name, day, func(c *storage.Counter) bool {
		if c.Count > 0 {
			return false
		}
		c.Count = 1
		return true
	})
}

// atLeast raises a counter to n.
func (t *Tracker) atLeast(ctx context.Context, name, day string, n int) error {
	return t.move(ctx, name, day, func(c *storage.Counter) bool {
		if n <= c.Count {
			return false
		}
		c.Count = n
		return true
	})
}

func (t *Tracker) move(ctx context.Context, name, day string, f func(c *storage.Counter) bool) error {
	if !nameRe.MatchString(name) {
		t.o.Logger.Warn("achievements: refused a counter name that is not an id")
		return nil
	}
	c, err := t.o.DB.Counter(ctx, name)
	if err != nil {
		return err
	}
	if !f(&c) {
		return nil
	}
	if c.FirstDay == nil {
		d := day
		c.FirstDay = &d
	}
	d := day
	c.LastDay = &d
	return t.o.DB.SaveCounter(ctx, c)
}

// progressLocked is each badge's count and goal from the counters.
func (t *Tracker) progressLocked(counters map[string]storage.Counter) []contract.BadgeProgress {
	out := make([]contract.BadgeProgress, 0, len(Badges))
	for _, b := range Badges {
		p := contract.BadgeProgress{ID: b.ID, Goal: b.Goal}
		switch b.Kind {
		case Count:
			p.Count = counters[b.Counter].Count
		case All:
			members := t.universes[b.Counter]
			p.Goal = len(members)
			for _, m := range members {
				if counters[b.Counter+m].Count > 0 {
					p.Count++
				}
			}
		}
		if p.Goal < 1 {
			p.Goal = 1 // an empty set is never met: 0 of 1
		}
		if p.Count > p.Goal {
			p.Count = p.Goal
		}
		out = append(out, p)
	}
	return out
}

// awardLocked earns every badge whose goal is met and that is not earned
// yet, on day. It returns how many were new.
func (t *Tracker) awardLocked(ctx context.Context, day string) (int, error) {
	counters, err := t.o.DB.Counters(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for i, p := range t.progressLocked(counters) {
		b := Badges[i]
		if p.Count < p.Goal { // an empty set is 0 of 1: never met
			continue
		}
		fresh, err := t.o.DB.EarnBadge(ctx, b.ID, day)
		if err != nil {
			return n, err
		}
		if fresh {
			n++
		}
	}
	return n, nil
}

// Snapshot is state.achievements for the shell (with celebrate); Phone drops
// celebrate. Storage errors give an empty shelf.
func (t *Tracker) Snapshot() contract.Achievements {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cached == nil {
		a := contract.Achievements{Earned: []contract.EarnedBadge{}}
		if t.o.DB != nil {
			ctx := context.Background()
			counters, err := t.o.DB.Counters(ctx)
			if err != nil {
				counters = map[string]storage.Counter{}
			}
			a.Progress = t.progressLocked(counters)
			if earned, err := t.o.DB.EarnedBadges(ctx); err == nil {
				for _, e := range earned {
					if !known(e.ID) {
						continue // a badge from a newer build: not shown
					}
					a.Earned = append(a.Earned, contract.EarnedBadge{ID: e.ID, Day: e.Day})
					if !e.Celebrated {
						a.Celebrate = append(a.Celebrate, e.ID)
					}
				}
			}
		} else {
			a.Progress = t.progressLocked(map[string]storage.Counter{})
		}
		t.cached = &a
	}
	out := *t.cached
	out.Enabled = t.Enabled()
	out.Earned = append([]contract.EarnedBadge{}, t.cached.Earned...)
	out.Progress = append([]contract.BadgeProgress{}, t.cached.Progress...)
	out.Celebrate = append([]string(nil), t.cached.Celebrate...)
	return out
}

// Phone is the snapshot a controller phone gets: no celebrate.
func (t *Tracker) Phone() contract.Achievements {
	a := t.Snapshot()
	a.Celebrate = nil
	return a
}

func known(id string) bool {
	for _, b := range Badges {
		if b.ID == id {
			return true
		}
	}
	return false
}

// Celebrated marks badges the shell celebrated on Home. It works while
// counting is off too: it only clears the queue.
func (t *Tracker) Celebrated(ids []string) {
	if t.o.DB == nil || len(ids) == 0 {
		return
	}
	t.mu.Lock()
	err := t.o.DB.MarkCelebrated(context.Background(), ids)
	t.cached = nil
	t.mu.Unlock()
	if err != nil {
		t.o.Logger.Warn("achievements: could not mark badges celebrated", "err", err)
	}
	if t.o.OnChange != nil {
		t.o.OnChange()
	}
}

// Reset deletes every counter and earned badge.
func (t *Tracker) Reset() error {
	if t.o.DB == nil {
		return nil
	}
	t.mu.Lock()
	err := t.o.DB.ResetAchievements(context.Background())
	t.cached = nil
	t.lastHome = ""
	t.mu.Unlock()
	if t.o.OnChange != nil {
		t.o.OnChange()
	}
	return err
}

// Launched: an app launch succeeded (doLaunch).
func (t *Tracker) Launched(appID, adapter string) {
	t.update(func(ctx context.Context, day string) error {
		if err := t.bump(ctx, CounterLaunches, day); err != nil {
			return err
		}
		if err := t.bump(ctx, PrefixLaunch+adapter, day); err != nil {
			return err
		}
		return t.mark(ctx, PrefixApp+appID, day)
	})
}

// Home describes what the TV looked like when Home was shown.
type Home struct {
	// Weather is state.weather.current.condition, or "" when weather is off
	// or has no fresh reading.
	Weather string
	Style   string // layout.ui.art_style (pixel when empty)
	Theme   string // canonical theme id (layout.ui.background)
}

// HomeShown: the shell showed Home to someone (a focus report on Home or
// Home coming back to the front). It moves the day-based counters, the
// weather, season, style and theme.
func (t *Tracker) HomeShown(h Home) {
	now := t.o.Clock.Now().In(t.o.Location)
	// Focus moves on Home report often; within one day, part of the day,
	// weather and look nothing new can count, so skip the database.
	part := "day"
	switch hr := now.Hour(); {
	case hr >= 23 || hr < 4:
		part = "night"
	case hr < 7:
		part = "early"
	}
	key := t.day(now) + "|" + part + "|" + h.Weather + "|" + h.Style + "|" + h.Theme
	t.mu.Lock()
	same := key == t.lastHome
	t.lastHome = key
	if !t.Enabled() {
		t.lastHome = "" // counting again later starts fresh
	}
	t.mu.Unlock()
	if same {
		return
	}
	t.update(func(ctx context.Context, day string) error {
		steps := []func() error{
			func() error { return t.bumpDay(ctx, CounterDays, day) },
			func() error { return t.mark(ctx, PrefixSeason+Season(now), day) },
		}
		switch hr := now.Hour(); {
		case hr >= 23 || hr < 4:
			// A night belongs to the evening it started: 00:30 on the 2nd
			// is still the night of the 1st.
			night := day
			if hr < 4 {
				night = t.day(time.Date(now.Year(), now.Month(), now.Day()-1, 12, 0, 0, 0, t.o.Location))
			}
			steps = append(steps, func() error { return t.bumpDay(ctx, CounterNightOwl, night) })
		case hr < 7:
			steps = append(steps, func() error { return t.bumpDay(ctx, CounterEarlyCub, day) })
		}
		switch h.Weather {
		case "rain", "drizzle":
			steps = append(steps, func() error { return t.bumpDay(ctx, CounterRain, day) })
		case "snow":
			steps = append(steps, func() error { return t.bumpDay(ctx, CounterSnow, day) })
		case "thunder":
			steps = append(steps, func() error { return t.bumpDay(ctx, CounterThunder, day) })
		}
		style := h.Style
		if style == "" {
			style = contract.ArtPixel
		}
		steps = append(steps, func() error { return t.mark(ctx, PrefixStyle+style, day) })
		if h.Theme != "" {
			steps = append(steps, func() error { return t.mark(ctx, PrefixTheme+h.Theme, day) })
		}
		for _, s := range steps {
			if err := s(); err != nil {
				return err
			}
		}
		return nil
	})
}

// Paired: a phone was paired; family is how many family phones (not guest
// passes) are paired now.
func (t *Tracker) Paired(family int) {
	t.update(func(ctx context.Context, day string) error { return t.atLeast(ctx, CounterFamilyPhones, day, family) })
}

// PassIssued: a guest pass code was issued on the TV.
func (t *Tracker) PassIssued() {
	t.update(func(ctx context.Context, day string) error { return t.bump(ctx, CounterGuestPasses, day) })
}

// SleepTimerSet: a sleep timer was set (not cancelled).
func (t *Tracker) SleepTimerSet() {
	t.update(func(ctx context.Context, day string) error { return t.bump(ctx, CounterSleepTimers, day) })
}

// Parade: the remote's secret code started the bear parade.
func (t *Tracker) Parade() {
	t.update(func(ctx context.Context, day string) error { return t.bump(ctx, CounterParades, day) })
}

// Season is the meteorological season of t's month. The badge wants all
// four, so the hemisphere does not matter.
func Season(t time.Time) string {
	switch t.Month() {
	case time.March, time.April, time.May:
		return "spring"
	case time.June, time.July, time.August:
		return "summer"
	case time.September, time.October, time.November:
		return "autumn"
	}
	return "winter"
}

// IDs lists the catalogue's badge ids in display order.
func IDs() []string {
	out := make([]string, 0, len(Badges))
	for _, b := range Badges {
		out = append(out, b.ID)
	}
	return out
}
