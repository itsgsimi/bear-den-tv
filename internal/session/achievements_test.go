// Session tests for Den badges (achievements.go, state.go, ipc.go, route.go,
// power.go): each event point moves its counter through the real
// coordinator, state.achievements reaches the shell and controller phones
// only (never guests, anonymous viewers or a locked session), the
// achievements.* IPC messages turn counting off, reset and clear the
// celebration queue, and the stored rows hold no titles
// (docs/security.md#den-badges).

package session

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bear-den-tv/internal/achievements"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/pairing"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/shellipc"
	"bear-den-tv/internal/storage"
)

type badgeHarness struct {
	*harness
	db     *storage.DB
	dbPath string
	clk    *clock.Fake
	tr     *achievements.Tracker
}

func newBadgeHarness(t *testing.T, configure ...func(*Options)) *badgeHarness {
	t.Helper()
	b := &badgeHarness{dbPath: filepath.Join(t.TempDir(), "badges.db")}
	db, err := storage.Open(b.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	b.db = db
	b.clk = clock.NewFake(time.Date(2026, time.September, 28, 20, 0, 0, 0, time.Local))
	b.harness = newHarness(t, append([]func(*Options){func(o *Options) {
		b.tr = achievements.New(achievements.Options{DB: db, Clock: b.clk, Enabled: func() bool { return o.Config.Current().AchievementsEnabled() },
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		o.Achievements = b.tr
	}}, configure...)...)
	return b
}

func (b *badgeHarness) count(name string) int {
	c, err := b.db.Counter(context.Background(), name)
	if err != nil {
		b.t.Fatal(err)
	}
	return c.Count
}

func (b *badgeHarness) waitCount(name string, want int) {
	b.t.Helper()
	b.eventually(name+" reaching its count", func() bool { return b.count(name) == want })
}

func (b *badgeHarness) focusHome() {
	b.t.Helper()
	if err := b.shell.Send(shellipc.Focus{Type: shellipc.TypeFocus, Screen: "home"}); err != nil {
		b.t.Fatal(err)
	}
}

// Every event point in the coordinator moves its own counter.
func TestBadgeCountersMoveAtTheirEvents(t *testing.T) {
	b := newBadgeHarness(t)
	// Home shown: the shell is in front and reports focus on Home.
	b.focusHome()
	b.waitCount(achievements.CounterDays, 1)
	b.waitCount(achievements.PrefixStyle+"pixel", 1) // the look on Home
	// A focus report on another screen is not Home.
	b.clk.Advance(24 * time.Hour)
	if err := b.shell.Send(shellipc.Focus{Type: shellipc.TypeFocus, Screen: "settings"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if b.count(achievements.CounterDays) != 1 {
		t.Fatal("Settings counted as Home")
	}
	// A launch that works.
	expectOutcome(t, b.submit(b.ctl, b.req("app.launch", map[string]any{"app_id": "plex-htpc"})), contract.OutcomeAccepted, contract.CodeOK)
	b.waitCount(achievements.CounterLaunches, 1)
	b.waitCount(achievements.PrefixLaunch+"plex-htpc", 1)
	b.waitCount(achievements.PrefixApp+"plex-htpc", 1)
	b.eventually("Plex in front", func() bool { return b.c.Target().Kind == "app" })
	// Home while an app is in front counts nothing; Home coming back does.
	b.focusHome()
	time.Sleep(50 * time.Millisecond)
	if b.count(achievements.CounterDays) != 1 {
		t.Fatal("Home counted while an app was in front")
	}
	b.desk.SetActive(b.shellW)
	b.waitCount(achievements.CounterDays, 2)
	// A sleep timer set (a cancel is not counted).
	expectOutcome(t, b.submit(b.ctl, b.req(contract.ActionSleepTimer, map[string]any{"minutes": 30})), contract.OutcomeObserved, contract.CodeOK)
	expectOutcome(t, b.submit(b.ctl, b.req(contract.ActionSleepTimer, map[string]any{"minutes": 0})), contract.OutcomeObserved, contract.CodeOK)
	b.waitCount(achievements.CounterSleepTimers, 1)
	// A guest pass issued on the TV (a family code is not).
	_ = b.shell.Send(shellipc.PairIssue{Type: shellipc.TypePairIssue, RequestID: "g1", Pass: contract.PassTonight})
	_ = b.shell.Send(shellipc.PairIssue{Type: shellipc.TypePairIssue, RequestID: "f1"})
	b.shellResult("g1")
	b.shellResult("f1")
	b.waitCount(achievements.CounterGuestPasses, 1)
	// The parade, from the shell; unknown events count nothing.
	_ = b.shell.Send(shellipc.AchievementsEvent{Type: shellipc.TypeAchievementsEvent, Event: "confetti"})
	_ = b.shell.Send(shellipc.AchievementsEvent{Type: shellipc.TypeAchievementsEvent, Event: shellipc.AchievementEventParade})
	b.waitCount(achievements.CounterParades, 1)
	// A look chosen in Settings.
	l := b.c.opts.Config.Current().Layout()
	l.UI.ArtStyle = contract.ArtClassic
	_ = b.shell.Send(shellipc.SettingsUpdate{Type: shellipc.TypeSettingsUpdate, RequestID: "s1", BaseRevision: b.c.opts.Config.Revision(), Layout: l})
	b.waitCount(achievements.PrefixStyle+"classic", 1)
	// Phones paired: a family phone, a guest pass (not a family phone),
	// then a second family phone.
	ctx := context.Background()
	pair := b.c.opts.Pairing
	for i, pass := range []string{"", contract.Pass24h, ""} {
		var inv pairing.Issued
		var err error
		if pass == "" {
			inv, err = pair.Issue(ctx, nil)
		} else {
			inv, err = pair.IssuePass(ctx, pass)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pair.Claim(ctx, inv.Token, "", "Phone", "10.0.0.2:1"); err != nil {
			t.Fatal(err)
		}
		b.c.NotePaired() // pairing.Options.OnPaired in a real session
		if want := []int{1, 1, 2}[i]; b.count(achievements.CounterFamilyPhones) != want {
			t.Fatalf("family phones after pairing %d: %d, want %d", i+1, b.count(achievements.CounterFamilyPhones), want)
		}
	}

	earned := map[string]bool{}
	for _, e := range b.c.buildState(viewShell).Achievements.Earned {
		earned[e.ID] = true
	}
	for _, id := range []string{"first-night-in", "good-host", "parade-spotter", "style-switcher", "family-den"} {
		if !earned[id] {
			t.Errorf("%s not earned: %v", id, earned)
		}
	}
}

// state.achievements per view: the shell (with celebrate) and controller
// phones (without); never guests, anonymous viewers or while locked.
func TestBadgesReachOnlyTheShellAndControllerPhones(t *testing.T) {
	b := newBadgeHarness(t)
	b.tr.Parade()
	shell := b.c.buildState(viewShell)
	if shell.Achievements == nil || len(shell.Achievements.Celebrate) != 1 || len(shell.Achievements.Progress) != len(achievements.Badges) {
		t.Fatalf("shell view: %+v", shell.Achievements)
	}
	ctx := context.Background()
	ctl := b.phones.Snapshot(ctx, &b.ctl)
	if ctl.Achievements == nil || ctl.Achievements.Celebrate != nil || len(ctl.Achievements.Earned) != 1 {
		t.Fatalf("controller view: %+v", ctl.Achievements)
	}
	owner := b.phones.Snapshot(ctx, &b.owner)
	guest := guestViewer(time.Now().Add(time.Hour))
	views := map[string]contract.State{
		"shell": shell, "controller": ctl, "owner": owner,
		"guest": b.phones.Snapshot(ctx, &guest), "anonymous": b.phones.Snapshot(ctx, nil),
	}
	if owner.Achievements == nil {
		t.Fatal("owner phone has no badges")
	}
	for _, name := range []string{"guest", "anonymous"} {
		if views[name].Achievements != nil {
			t.Fatalf("%s got badges: %+v", name, views[name].Achievements)
		}
	}
	for name, st := range views {
		if _, err := contract.MarshalAndValidateState(st); err != nil {
			t.Fatalf("%s view: %v", name, err)
		}
	}
	b.lock.set(true)
	b.eventually("locked", func() bool { return b.c.Target().Kind == "locked" })
	for name, st := range map[string]contract.State{"shell": b.c.buildState(viewShell), "controller": b.phones.Snapshot(ctx, &b.ctl), "owner": b.phones.Snapshot(ctx, &b.owner)} {
		if st.Achievements != nil {
			t.Fatalf("%s got badges while locked", name)
		}
	}
	// Nothing counts on Home behind the lock.
	b.focusHome()
	time.Sleep(50 * time.Millisecond)
	if b.count(achievements.CounterDays) != 0 {
		t.Fatal("Home counted while locked")
	}
}

// achievements.configure off stops all counting and is stored; reset clears
// everything; celebrated empties the queue; phones and cli clients cannot
// send the shell-only messages.
func TestBadgesOffResetAndCelebratedOverIPC(t *testing.T) {
	b := newBadgeHarness(t)
	_ = b.shell.Send(shellipc.AchievementsConfigure{Type: shellipc.TypeAchievementsConfigure, RequestID: "off", Enabled: false})
	if r := b.shellResult("off"); !r.OK {
		t.Fatalf("configure off: %+v", r)
	}
	if b.c.opts.Config.Current().AchievementsEnabled() {
		t.Fatal("config not stored")
	}
	if st := b.c.buildState(viewShell); st.Achievements == nil || st.Achievements.Enabled {
		t.Fatalf("snapshot while off: %+v", st.Achievements)
	}
	b.focusHome()
	_ = b.shell.Send(shellipc.AchievementsEvent{Type: shellipc.TypeAchievementsEvent, Event: shellipc.AchievementEventParade})
	expectOutcome(t, b.submit(b.ctl, b.req(contract.ActionSleepTimer, map[string]any{"minutes": 15})), contract.OutcomeObserved, contract.CodeOK)
	time.Sleep(80 * time.Millisecond)
	if c, _ := b.db.Counters(context.Background()); len(c) != 0 {
		t.Fatalf("counted while off: %v", c)
	}
	_ = b.shell.Send(shellipc.AchievementsConfigure{Type: shellipc.TypeAchievementsConfigure, RequestID: "on", Enabled: true})
	b.shellResult("on")
	_ = b.shell.Send(shellipc.AchievementsEvent{Type: shellipc.TypeAchievementsEvent, Event: shellipc.AchievementEventParade})
	b.waitCount(achievements.CounterParades, 1)
	b.eventually("parade spotter to celebrate", func() bool {
		a := b.c.buildState(viewShell).Achievements
		return a != nil && len(a.Celebrate) == 1
	})
	_ = b.shell.Send(shellipc.AchievementsCelebrated{Type: shellipc.TypeAchievementsCelebrated, IDs: []string{"parade-spotter"}})
	b.eventually("celebrate to empty", func() bool { return len(b.c.buildState(viewShell).Achievements.Celebrate) == 0 })
	if len(b.c.buildState(viewShell).Achievements.Earned) != 1 {
		t.Fatal("celebrating dropped the badge")
	}
	_ = b.shell.Send(shellipc.AchievementsReset{Type: shellipc.TypeAchievementsReset, RequestID: "reset"})
	if r := b.shellResult("reset"); !r.OK {
		t.Fatalf("reset: %+v", r)
	}
	if a := b.c.buildState(viewShell).Achievements; len(a.Earned) != 0 {
		t.Fatalf("badges after reset: %+v", a)
	}
	if c, _ := b.db.Counters(context.Background()); len(c) != 0 {
		t.Fatalf("counters after reset: %v", c)
	}
}

// Without a tracker the messages fail closed with a reason and the snapshot
// has no badges.
func TestNoTrackerFailsClosed(t *testing.T) {
	h := newHarness(t)
	_ = h.shell.Send(shellipc.AchievementsReset{Type: shellipc.TypeAchievementsReset, RequestID: "r"})
	if r := h.shellResult("r"); r.OK || !strings.Contains(r.Error, "not available") {
		t.Fatalf("reset without a tracker: %+v", r)
	}
	if st := h.c.buildState(viewShell); st.Achievements != nil {
		t.Fatal("badges without a tracker")
	}
}

// With a DEMO player in front reporting a title, launching, showing Home and
// every other event store only ids, counts and days: the title appears
// nowhere in the badge tables, which hold exactly their known columns.
func TestBadgeRowsNeverHoldTitles(t *testing.T) {
	media := fake.NewMedia()
	b := newBadgeHarness(t, func(o *Options) { o.Media = media })
	const title = "DEMO Secret Midnight Movie"
	media.Add(adapters.PlexHTPCFlatpakID, fake.NewPlayer(clock.Real{}, platform.MediaInfo{Status: "Playing", Title: title, Artists: []string{"DEMO Director"}, Rate: 1}))
	expectOutcome(t, b.submit(b.ctl, b.req("app.launch", map[string]any{"app_id": "plex-htpc"})), contract.OutcomeAccepted, contract.CodeOK)
	b.waitCount(achievements.CounterLaunches, 1)
	b.desk.SetActive(b.shellW)
	b.focusHome()
	b.waitCount(achievements.CounterDays, 1)
	_ = b.shell.Send(shellipc.AchievementsEvent{Type: shellipc.TypeAchievementsEvent, Event: shellipc.AchievementEventParade})
	b.waitCount(achievements.CounterParades, 1)

	raw, err := sql.Open("sqlite", "file:"+b.dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for table, cols := range map[string]string{"achievement_counters": "name, count, first_day, last_day", "achievement_badges": "id, earned_day, celebrated"} {
		rows, err := raw.Query("SELECT " + cols + " FROM " + table)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for rows.Next() {
			var a, b2, c, d any
			dest := []any{&a, &b2, &c}
			if table == "achievement_counters" {
				dest = append(dest, &d)
			}
			if err := rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			for _, v := range []any{a, b2, c, d} {
				s := strings.ToLower(toString(v))
				for _, bad := range []string{"secret", "movie", "demo", "director", " "} {
					if strings.Contains(s, bad) {
						t.Errorf("%s holds %q", table, s)
					}
				}
			}
			n++
		}
		rows.Close()
		if n == 0 {
			t.Fatalf("%s is empty", table)
		}
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	}
	return ""
}
