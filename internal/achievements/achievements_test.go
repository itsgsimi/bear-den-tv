// Tests for the Den badge tracker (achievements.go) on a fake clock and an
// in-memory database: every counter moves at its event, day counters count
// distinct local calendar days (across midnight and daylight saving time),
// goals award once, off counts nothing, reset clears, and nothing but ids,
// counts and days is ever stored.

package achievements

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Berlin without the host's zoneinfo

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/storage"
)

type rig struct {
	t       *testing.T
	db      *storage.DB
	clk     *clock.Fake
	tr      *Tracker
	on      bool
	changes int
}

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// newRig starts at local time y-m-d h:min in Berlin.
func newRig(t *testing.T, y int, m time.Month, d, h, min int) *rig {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	loc := berlin(t)
	r := &rig{t: t, db: db, clk: clock.NewFake(time.Date(y, m, d, h, min, 0, 0, loc)), on: true}
	r.tr = New(Options{DB: db, Clock: r.clk, Location: loc, Enabled: func() bool { return r.on }, OnChange: func() { r.changes++ },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	return r
}

func (r *rig) count(name string) int {
	r.t.Helper()
	c, err := r.db.Counter(context.Background(), name)
	if err != nil {
		r.t.Fatal(err)
	}
	return c.Count
}

func (r *rig) earned() map[string]string {
	r.t.Helper()
	out := map[string]string{}
	for _, e := range r.tr.Snapshot().Earned {
		out[e.ID] = e.Day
	}
	return out
}

func (r *rig) progress(id string) (int, int) {
	for _, p := range r.tr.Snapshot().Progress {
		if p.ID == id {
			return p.Count, p.Goal
		}
	}
	r.t.Fatalf("no progress for %s", id)
	return 0, 0
}

// Each event moves its own counters.
func TestEveryEventMovesItsCounter(t *testing.T) {
	r := newRig(t, 2026, time.September, 28, 15, 0)
	r.tr.Launched("plex-htpc", "plex-htpc")
	r.tr.Launched("youtube", "vacuumtube")
	r.tr.Launched("plex-htpc", "plex-htpc")
	r.tr.PassIssued()
	r.tr.SleepTimerSet()
	r.tr.SleepTimerSet()
	r.tr.Parade()
	r.tr.Paired(1)
	r.tr.Paired(3)
	r.tr.Paired(2) // a phone was removed: the most at once stays
	r.tr.HomeShown(Home{Weather: "drizzle", Style: "classic", Theme: "forest"})
	for name, want := range map[string]int{
		CounterLaunches: 3, PrefixLaunch + "plex-htpc": 2, PrefixLaunch + "vacuumtube": 1,
		PrefixApp + "plex-htpc": 1, PrefixApp + "youtube": 1,
		CounterGuestPasses: 1, CounterSleepTimers: 2, CounterParades: 1, CounterFamilyPhones: 3,
		CounterDays: 1, CounterRain: 1, CounterSnow: 0, CounterThunder: 0, CounterNightOwl: 0, CounterEarlyCub: 0,
		PrefixSeason + "autumn": 1, PrefixStyle + "classic": 1, PrefixTheme + "forest": 1,
	} {
		if got := r.count(name); got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	r.tr.HomeShown(Home{Weather: "snow", Theme: "forest"}) // empty style is pixel
	r.tr.HomeShown(Home{Weather: "thunder", Theme: "forest"})
	if r.count(CounterSnow) != 1 || r.count(CounterThunder) != 1 || r.count(PrefixStyle+"pixel") != 1 || r.count(CounterDays) != 1 {
		t.Fatalf("snow %d thunder %d pixel %d days %d", r.count(CounterSnow), r.count(CounterThunder), r.count(PrefixStyle+"pixel"), r.count(CounterDays))
	}
	e := r.earned()
	for _, id := range []string{"first-night-in", "good-host", "parade-spotter", "family-den", "snow-day", "thunder-buddy", "style-switcher"} {
		if e[id] != "2026-09-28" {
			t.Errorf("%s not earned today: %v", id, e)
		}
	}
	if _, ok := e["movie-night"]; ok {
		t.Fatal("movie night after 2 Plex launches")
	}
	if r.changes == 0 {
		t.Fatal("no OnChange")
	}
}

// Day counters count local calendar days: two visits a minute apart across
// midnight are two days, but the same night for the night owl; the 25-hour
// day when summer time ends is one day, though 24 hours passed.
func TestDaysAreLocalCalendarDays(t *testing.T) {
	r := newRig(t, 2026, time.October, 23, 23, 59)
	r.tr.HomeShown(Home{})
	r.clk.Advance(2 * time.Minute) // 00:01 on the 24th
	r.tr.HomeShown(Home{})
	if got := r.count(CounterDays); got != 2 {
		t.Fatalf("days across midnight = %d, want 2", got)
	}
	if got := r.count(CounterNightOwl); got != 1 {
		t.Fatalf("night owl across midnight = %d, want 1 (one night)", got)
	}
	// 00:30 on Sunday the 25th (still summer time), then 24 hours later:
	// 23:30 the same Sunday, because the clocks went back at 03:00.
	r.clk.Advance(24*time.Hour + 29*time.Minute)
	r.tr.HomeShown(Home{})
	if got := r.count(CounterDays); got != 3 {
		t.Fatalf("days = %d, want 3", got)
	}
	r.clk.Advance(24 * time.Hour)
	if now := r.clk.Now().In(berlin(t)); now.Day() != 25 || now.Hour() != 23 {
		t.Fatalf("the test's clock is off: %v", now)
	}
	r.tr.HomeShown(Home{Style: "classic"}) // a new look, so the visit is not skipped
	if got := r.count(CounterDays); got != 3 {
		t.Fatalf("the 25-hour day counted twice: days = %d", got)
	}
	if got := r.count(CounterNightOwl); got != 3 {
		t.Fatalf("night owl = %d, want 3 (the nights of the 23rd, 24th and 25th)", got)
	}
	// Spring: 23:30 on 28 March, 24 hours later is 00:30 on the 30th.
	s := newRig(t, 2026, time.March, 28, 23, 30)
	s.tr.HomeShown(Home{})
	s.clk.Advance(24 * time.Hour)
	s.tr.HomeShown(Home{})
	if got := s.count(CounterDays); got != 2 {
		t.Fatalf("spring days = %d, want 2", got)
	}
	// Early cub: 05:00 and 06:59 the same morning are one.
	m := newRig(t, 2026, time.June, 1, 5, 0)
	m.tr.HomeShown(Home{})
	m.clk.Advance(119 * time.Minute)
	m.tr.HomeShown(Home{Theme: "den"})
	m.clk.Advance(time.Minute) // 07:00
	m.tr.HomeShown(Home{})
	if got := m.count(CounterEarlyCub); got != 1 {
		t.Fatalf("early cub = %d, want 1", got)
	}
}

// A clock set back never counts a day twice.
func TestClockGoingBackCountsNothing(t *testing.T) {
	r := newRig(t, 2026, time.September, 28, 12, 0)
	r.tr.HomeShown(Home{})
	r.clk.Advance(-48 * time.Hour)
	r.tr.HomeShown(Home{})
	r.clk.Advance(48 * time.Hour)
	r.tr.HomeShown(Home{Theme: "den"})
	if got := r.count(CounterDays); got != 1 {
		t.Fatalf("days = %d, want 1", got)
	}
}

// A goal awards once, on the day it is reached; later events never move the
// day or award it again, and progress stops at the goal.
func TestGoalsAwardOnce(t *testing.T) {
	r := newRig(t, 2026, time.September, 1, 20, 0)
	for i := 0; i < 9; i++ {
		r.tr.Launched("plex-htpc", "plex-htpc")
	}
	if n, g := r.progress("movie-night"); n != 9 || g != 10 {
		t.Fatalf("movie night %d/%d", n, g)
	}
	if _, ok := r.earned()["movie-night"]; ok {
		t.Fatal("earned before the goal")
	}
	r.clk.Advance(24 * time.Hour)
	r.tr.Launched("plex-htpc", "plex-htpc")
	r.clk.Advance(72 * time.Hour)
	r.tr.Launched("plex-htpc", "plex-htpc")
	if d := r.earned()["movie-night"]; d != "2026-09-02" {
		t.Fatalf("movie night earned on %q, want 2026-09-02", d)
	}
	if n, g := r.progress("movie-night"); n != 10 || g != 10 {
		t.Fatalf("progress past the goal %d/%d", n, g)
	}
	earned := r.tr.Snapshot().Earned
	seen := map[string]int{}
	for _, e := range earned {
		seen[e.ID]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("%s earned %d times", id, n)
		}
	}
	if got := r.tr.Snapshot().Celebrate; len(got) != 2 {
		t.Fatalf("celebrate %v, want first-night-in and movie-night", got)
	}
	r.tr.Celebrated([]string{"movie-night", "first-night-in"})
	if got := r.tr.Snapshot().Celebrate; len(got) != 0 {
		t.Fatalf("celebrate after celebrating %v", got)
	}
	if got := r.tr.Phone().Celebrate; got != nil {
		t.Fatalf("phones got celebrate %v", got)
	}
}

// Sets: every member must be seen, and an empty set never awards.
func TestSetsNeedEveryMember(t *testing.T) {
	r := newRig(t, 2026, time.September, 28, 15, 0)
	r.tr.Launched("plex-htpc", "plex-htpc")
	if _, ok := r.earned()["couch-explorer"]; ok {
		t.Fatal("couch explorer with no installed apps known")
	}
	if n, g := r.progress("couch-explorer"); n != 0 || g != 1 {
		t.Fatalf("empty set progress %d/%d", n, g)
	}
	r.tr.SetUniverse(PrefixApp, []string{"plex-htpc", "youtube"})
	if n, g := r.progress("couch-explorer"); n != 1 || g != 2 {
		t.Fatalf("couch explorer %d/%d", n, g)
	}
	r.tr.Launched("youtube", "vacuumtube")
	if r.earned()["couch-explorer"] != "2026-09-28" {
		t.Fatal("couch explorer not earned after every app")
	}
	r.tr.SetUniverse(PrefixTheme, []string{"den", "forest"})
	r.tr.HomeShown(Home{Theme: "den"})
	r.tr.HomeShown(Home{Theme: "den", Style: "classic"})
	if _, ok := r.earned()["theme-tourist"]; ok {
		t.Fatal("theme tourist after one theme")
	}
	r.tr.HomeShown(Home{Theme: "forest"})
	if _, ok := r.earned()["theme-tourist"]; !ok {
		t.Fatal("theme tourist not earned")
	}
	seasons := newRig(t, 2026, time.January, 10, 12, 0)
	for i := 0; i < 3; i++ {
		seasons.tr.HomeShown(Home{})
		seasons.clk.Advance(92 * 24 * time.Hour)
	}
	if _, ok := seasons.earned()["all-seasons"]; ok {
		t.Fatal("all seasons after three")
	}
	seasons.tr.HomeShown(Home{})
	if _, ok := seasons.earned()["all-seasons"]; !ok {
		t.Fatal("all seasons not earned after four")
	}
}

// Choosing a look in Settings counts as trying it.
func TestLookSeenMarksStyleAndTheme(t *testing.T) {
	r := newRig(t, 2026, time.September, 28, 15, 0)
	r.tr.LookSeen("classic", "midnight")
	r.tr.LookSeen("", "")
	if r.count(PrefixStyle+"classic") != 1 || r.count(PrefixTheme+"midnight") != 1 || r.count(PrefixStyle+"pixel") != 1 || r.count(CounterDays) != 0 {
		t.Fatalf("classic %d midnight %d pixel %d days %d", r.count(PrefixStyle+"classic"), r.count(PrefixTheme+"midnight"), r.count(PrefixStyle+"pixel"), r.count(CounterDays))
	}
	if _, ok := r.earned()["style-switcher"]; !ok {
		t.Fatal("style switcher not earned")
	}
}

// Off counts nothing at all; reset clears everything.
func TestOffCountsNothingAndResetClears(t *testing.T) {
	r := newRig(t, 2026, time.September, 28, 23, 30)
	r.on = false
	r.tr.Launched("plex-htpc", "plex-htpc")
	r.tr.HomeShown(Home{Weather: "rain", Theme: "den"})
	r.tr.PassIssued()
	r.tr.SleepTimerSet()
	r.tr.Parade()
	r.tr.Paired(2)
	if c, _ := r.db.Counters(context.Background()); len(c) != 0 {
		t.Fatalf("counted while off: %v", c)
	}
	if s := r.tr.Snapshot(); s.Enabled || len(s.Earned) != 0 {
		t.Fatalf("snapshot while off %+v", s)
	}
	r.on = true
	r.tr.HomeShown(Home{Weather: "rain", Theme: "den"}) // the same visit, now counted
	r.tr.Parade()
	if r.count(CounterDays) != 1 || len(r.earned()) != 1 {
		t.Fatalf("after turning on: days %d earned %v", r.count(CounterDays), r.earned())
	}
	if err := r.tr.Reset(); err != nil {
		t.Fatal(err)
	}
	if c, _ := r.db.Counters(context.Background()); len(c) != 0 {
		t.Fatalf("counters after reset %v", c)
	}
	if s := r.tr.Snapshot(); len(s.Earned) != 0 || len(s.Celebrate) != 0 {
		t.Fatalf("badges after reset %+v", s)
	}
	r.tr.HomeShown(Home{Weather: "rain", Theme: "den"})
	if r.count(CounterDays) != 1 {
		t.Fatal("the first visit after a reset was skipped")
	}
}

// Nothing stored is anything but an id, a count or a day: a name that is not
// an id is refused before it reaches the database.
func TestStoredRowsAreIdsCountsAndDays(t *testing.T) {
	r := newRig(t, 2026, time.September, 28, 21, 0)
	r.tr.SetUniverse(PrefixApp, []string{"plex-htpc"})
	r.tr.Launched("plex-htpc", "plex-htpc")
	r.tr.Launched("DEMO Movie Title", "plex htpc") // never a real id; refused
	r.tr.HomeShown(Home{Weather: "rain", Style: "classic", Theme: "den"})
	r.tr.PassIssued()
	counters, err := r.db.Counters(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range counters {
		if !nameRe.MatchString(name) || strings.Contains(strings.ToLower(name), "movie") {
			t.Errorf("stored counter name %q", name)
		}
		for _, d := range []*string{c.FirstDay, c.LastDay} {
			if d == nil || len(*d) != 10 || strings.ContainsAny(*d, "T:") {
				t.Errorf("%s: stored day %v", name, d)
			}
		}
	}
	if _, ok := counters[PrefixApp+"DEMO Movie Title"]; ok {
		t.Fatal("a title became a counter")
	}
	for _, e := range r.tr.Snapshot().Earned {
		if len(e.Day) != 10 || !known(e.ID) {
			t.Fatalf("earned %+v", e)
		}
	}
}

// The catalogue is data: unique ids that fit the contract's pattern, a goal
// for every count badge, and the sixteen badges the docs describe.
func TestCatalogue(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range Badges {
		if seen[b.ID] || len(b.ID) < 2 || len(b.ID) > 32 || !nameRe.MatchString(b.ID) || strings.Contains(b.ID, ":") {
			t.Errorf("bad id %q", b.ID)
		}
		seen[b.ID] = true
		if b.Kind == Count && b.Goal < 1 {
			t.Errorf("%s has no goal", b.ID)
		}
		if !nameRe.MatchString(b.Counter) {
			t.Errorf("%s counter %q", b.ID, b.Counter)
		}
	}
	if len(Badges) != 16 || len(IDs()) != 16 {
		t.Fatalf("%d badges", len(Badges))
	}
	if Season(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) != "winter" || Season(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) != "autumn" {
		t.Fatal("seasons")
	}
}
