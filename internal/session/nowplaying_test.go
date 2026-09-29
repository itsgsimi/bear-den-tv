// Session tests for Now playing (nowplaying.go, state.go): state.now_playing
// maps the foreground app's own player for controller phones only, is absent
// while locked, when switched off and for every other viewer, follows the
// foreground, re-reads on signals, actions and a slow poll (fake clock), and
// never reaches logs, diagnostics or config.json.

package session

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/doctor"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/shellipc"
)

// lockedBuffer is a log sink safe to read while the coordinator writes.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type npHarness struct {
	*harness
	clk   *clock.Fake
	media *fake.Media
	logs  *lockedBuffer
}

func newNPHarness(t *testing.T) *npHarness {
	t.Helper()
	clk := clock.NewFake(time.Unix(1_800_000_000, 0))
	media := fake.NewMedia()
	logs := &lockedBuffer{}
	h := newHarness(t, func(o *Options) {
		o.Clock, o.Media = clk, media
		o.Logger = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})
	return &npHarness{harness: h, clk: clk, media: media, logs: logs}
}

// front brings a window of the given WM_CLASS to the front.
func (h *npHarness) front(class string) platform.WindowID {
	w := h.desk.AddWindow(platform.WindowInfo{PID: 9100, Class: []string{class, class}, Mapped: true})
	h.desk.SetActive(w)
	return w
}

func (h *npHarness) np(v *remote.Viewer) *contract.NowPlaying {
	return h.phones.Snapshot(context.Background(), v).NowPlaying
}

func (h *npHarness) waitNP(what string, cond func(np *contract.NowPlaying) bool) *contract.NowPlaying {
	h.t.Helper()
	var np *contract.NowPlaying
	h.eventually(what, func() bool {
		np = h.np(&h.ctl)
		return cond(np)
	})
	return np
}

// settle waits until the now-playing watcher has been idle for a moment.
func (h *npHarness) settle() {
	h.t.Helper()
	last := h.c.np.iterations.Load()
	for stable := 0; stable < 4; {
		time.Sleep(10 * time.Millisecond)
		if n := h.c.np.iterations.Load(); n != last {
			last, stable = n, 0
		} else {
			stable++
		}
	}
}

func demoPlex(clk clock.Clock) *fake.Player {
	return fake.NewPlayer(clk, platform.MediaInfo{
		Status: "Playing", Title: "DEMO Secret Episode", Artists: []string{"DEMO Show", "DEMO Guest"}, Album: "DEMO Season",
		Length: 44 * time.Minute, Position: 90 * time.Second, HasPosition: true, Rate: 1,
	})
}

func TestNowPlayingMapsTheForegroundPlayerForControllers(t *testing.T) {
	h := newNPHarness(t)
	plex := demoPlex(h.clk)
	h.media.Add(adapters.PlexHTPCFlatpakID, plex)
	h.front("plexhtpc")

	np := h.waitNP("now playing", func(np *contract.NowPlaying) bool { return np != nil })
	if np.AppID != "plex-htpc" || np.Title != "DEMO Secret Episode" || np.Subtitle != "DEMO Show, DEMO Guest" || np.Status != "playing" || np.Rate != 1 {
		t.Fatalf("mapped %+v", np)
	}
	if np.LengthMs == nil || *np.LengthMs != 2_640_000 || np.PositionMs == nil || *np.PositionMs != 90_000 {
		t.Fatalf("times %+v", np)
	}
	st := h.phones.Snapshot(context.Background(), &h.ctl)
	if st.NowPlaying.PositionAt > st.GeneratedAtMs || st.Remote.NowPlaying == nil || !*st.Remote.NowPlaying {
		t.Fatalf("position_at %d vs generated_at_ms %d, remote %+v", st.NowPlaying.PositionAt, st.GeneratedAtMs, st.Remote.NowPlaying)
	}
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatal(err)
	}

	// A signalled change is re-read with a new position_at on the coordinator clock.
	h.clk.Advance(10 * time.Second)
	plex.Signal()
	np = h.waitNP("re-read after a signal", func(np *contract.NowPlaying) bool { return np != nil && np.PositionAt == 10_000 })
	if *np.PositionMs != 100_000 {
		t.Fatalf("position after 10 s: %d", *np.PositionMs)
	}

	// Every other viewer: owner yes (above controller); no permission, anonymous and the shell never.
	if h.np(&h.owner) == nil {
		t.Fatal("an owner phone lacks now_playing")
	}
	none := remote.Viewer{DeviceID: "dev-none", DeviceName: "Guest", Permissions: []contract.Permission{}}
	if h.np(&none) != nil {
		t.Fatal("a phone without the controller permission sees now_playing")
	}
	if h.phones.Snapshot(context.Background(), nil).NowPlaying != nil {
		t.Fatal("an anonymous viewer sees now_playing")
	}
	if sh := h.c.buildState(viewShell); sh.NowPlaying != nil || sh.Remote.NowPlaying == nil || !*sh.Remote.NowPlaying {
		t.Fatalf("shell view: now_playing %+v, remote.now_playing %v", sh.NowPlaying, sh.Remote.NowPlaying)
	}
}

func TestNowPlayingMissingFields(t *testing.T) {
	h := newNPHarness(t)
	plex := fake.NewPlayer(h.clk, platform.MediaInfo{Status: "Paused", Title: "DEMO Bare"})
	h.media.Add(adapters.PlexHTPCFlatpakID, plex)
	h.front("plexhtpc")
	np := h.waitNP("bare player", func(np *contract.NowPlaying) bool { return np != nil })
	if np.Subtitle != "" || np.LengthMs != nil || np.PositionMs != nil || np.Status != "paused" || np.Rate != 1 {
		t.Fatalf("bare %+v", np)
	}
	st := h.phones.Snapshot(context.Background(), &h.ctl)
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatal(err)
	}

	plex.Set(platform.MediaInfo{Status: "Stopped", Title: "DEMO Album Only", Album: "DEMO Album"})
	np = h.waitNP("album subtitle", func(np *contract.NowPlaying) bool { return np != nil && np.Title == "DEMO Album Only" })
	if np.Subtitle != "DEMO Album" || np.Status != "stopped" {
		t.Fatalf("album only %+v", np)
	}

	long := strings.Repeat("é", 300)
	plex.Set(platform.MediaInfo{Status: "Playing", Title: long, Artists: []string{long}, Position: 5 * time.Minute, HasPosition: true, Length: time.Minute})
	np = h.waitNP("long title", func(np *contract.NowPlaying) bool { return np != nil && np.Status == "playing" })
	if n := len([]rune(np.Title)); n != contract.NowPlayingTextMax || !strings.HasSuffix(np.Title, "…") || len([]rune(np.Subtitle)) != contract.NowPlayingTextMax {
		t.Fatalf("title cut to %d runes", n)
	}
	if *np.PositionMs != 60_000 {
		t.Fatalf("position past the length not capped: %d", *np.PositionMs)
	}
	if _, err := contract.MarshalAndValidateState(h.phones.Snapshot(context.Background(), &h.ctl)); err != nil {
		t.Fatal(err)
	}

	plex.Set(platform.MediaInfo{Status: "Playing", Title: "   "})
	h.waitNP("no title means nothing", func(np *contract.NowPlaying) bool { return np == nil })
	plex.Set(platform.MediaInfo{Status: "Unknown", Title: "DEMO"})
	h.settle()
	if h.np(&h.ctl) != nil {
		t.Fatal("an unknown status was shown")
	}
	plex.Set(platform.MediaInfo{Status: "Playing", Title: "DEMO Back"})
	h.waitNP("back", func(np *contract.NowPlaying) bool { return np != nil })
	plex.Fail(os.ErrDeadlineExceeded)
	plex.Signal()
	h.waitNP("a failed read fails closed", func(np *contract.NowPlaying) bool { return np == nil })
}

func TestNowPlayingAbsentWhileLocked(t *testing.T) {
	h := newNPHarness(t)
	h.media.Add(adapters.PlexHTPCFlatpakID, demoPlex(h.clk))
	h.front("plexhtpc")
	h.waitNP("now playing", func(np *contract.NowPlaying) bool { return np != nil })

	h.lock.set(true)
	h.eventually("locked", func() bool { return h.c.Target().Kind == "locked" })
	for _, v := range []*remote.Viewer{&h.ctl, &h.owner} {
		if st := h.phones.Snapshot(context.Background(), v); st.NowPlaying != nil {
			t.Fatalf("locked snapshot carries now_playing: %+v", st.NowPlaying)
		}
	}
	h.c.mu.Lock()
	kept := h.c.np.cur != nil
	h.c.mu.Unlock()
	if kept {
		t.Fatal("the reading is kept in memory while locked")
	}
	h.lock.set(false)
	h.waitNP("back after unlock", func(np *contract.NowPlaying) bool { return np != nil && np.Title == "DEMO Secret Episode" })
}

func TestNowPlayingSettingOff(t *testing.T) {
	h := newNPHarness(t)
	plex := demoPlex(h.clk)
	h.media.Add(adapters.PlexHTPCFlatpakID, plex)
	h.front("plexhtpc")
	h.waitNP("now playing", func(np *contract.NowPlaying) bool { return np != nil })

	set := func(id string, on bool) {
		t.Helper()
		if err := h.shell.Send(shellipc.RemoteNowPlaying{Type: shellipc.TypeRemoteNowPlaying, RequestID: id, Enabled: on}); err != nil {
			t.Fatal(err)
		}
		if r := h.shellResult(id); !r.OK {
			t.Fatalf("remote.now_playing %v: %+v", on, r)
		}
	}
	set("np-off", false)
	if h.np(&h.ctl) != nil || h.np(&h.owner) != nil {
		t.Fatal("now_playing shown after the owner turned it off")
	}
	st := h.phones.Snapshot(context.Background(), &h.ctl)
	if st.Remote.NowPlaying == nil || *st.Remote.NowPlaying || h.c.opts.Config.Current().Remote.ShowNowPlaying() {
		t.Fatalf("setting not stored or mirrored: %v", st.Remote.NowPlaying)
	}
	// Off means the player is not even watched or read.
	h.eventually("watch dropped", func() bool { return plex.Watchers() == 0 })
	h.settle()
	reads := plex.Reads()
	plex.Signal()
	h.clk.Advance(NowPlayingPoll)
	h.settle()
	if plex.Reads() != reads {
		t.Fatalf("the player was read %d times while switched off", plex.Reads()-reads)
	}

	set("np-on", true)
	h.waitNP("back on", func(np *contract.NowPlaying) bool { return np != nil })
	if sh := h.c.buildState(viewShell); sh.Remote.NowPlaying == nil || !*sh.Remote.NowPlaying {
		t.Fatal("the shell does not see the setting back on")
	}
}

func TestNowPlayingOnlyTheForegroundAppsOwnPlayer(t *testing.T) {
	h := newNPHarness(t)
	yt := fake.NewPlayer(h.clk, platform.MediaInfo{Status: "Playing", Title: "DEMO Background Video"})
	h.media.Add(adapters.VacuumTubeFlatpakID, yt)

	// Plex in front has no player: YouTube's (playing behind it) is never used.
	plexWin := h.front("plexhtpc")
	h.eventually("media probed", func() bool {
		st := h.phones.Snapshot(context.Background(), &h.ctl)
		return st.Target.Kind == "app" && !strings.HasPrefix(st.Capabilities[contract.ActionMediaPause].Reason, "Checking")
	})
	h.settle()
	if np := h.np(&h.ctl); np != nil {
		t.Fatalf("another app's player shown: %+v", np)
	}
	if cp := h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPause]; cp.Available {
		t.Fatal("another app's player drives media controls")
	}

	ytWin := h.front("vacuumtube")
	h.waitNP("youtube's own", func(np *contract.NowPlaying) bool { return np != nil && np.AppID == "youtube" })

	h.media.Add(adapters.PlexHTPCFlatpakID, fake.NewPlayer(h.clk, platform.MediaInfo{Status: "Paused", Title: "DEMO Plex Title"}))
	h.desk.SetActive(plexWin)
	np := h.waitNP("plex's own", func(np *contract.NowPlaying) bool { return np != nil && np.AppID == "plex-htpc" })
	if np.Title != "DEMO Plex Title" {
		t.Fatalf("plex in front shows %q", np.Title)
	}
	// YouTube changing behind Plex changes nothing.
	yt.Set(platform.MediaInfo{Status: "Playing", Title: "DEMO Other"})
	h.settle()
	if np := h.np(&h.ctl); np == nil || np.Title != "DEMO Plex Title" {
		t.Fatalf("after a background change: %+v", np)
	}

	// Home (the shell in front): Plex, the app that was in front, paused
	// behind Home; never YouTube's player, though it plays.
	h.desk.SetActive(h.shellW)
	np = h.waitNP("plex behind home", func(np *contract.NowPlaying) bool {
		return np != nil && np.Foreground != nil && !*np.Foreground
	})
	if np.AppID != "plex-htpc" || np.Title != "DEMO Plex Title" {
		t.Fatalf("behind Home: %+v", np)
	}

	// The app exits (its window goes away): nothing.
	h.desk.SetActive(ytWin)
	h.waitNP("youtube again", func(np *contract.NowPlaying) bool { return np != nil && np.AppID == "youtube" })
	h.desk.RemoveWindow(ytWin)
	h.waitNP("exit clears it", func(np *contract.NowPlaying) bool { return np == nil })
}

func TestNowPlayingPollsOnlyWhilePlayingAndWatched(t *testing.T) {
	h := newNPHarness(t)
	plex := demoPlex(h.clk)
	h.media.Add(adapters.PlexHTPCFlatpakID, plex)
	h.front("plexhtpc")
	h.waitNP("now playing", func(np *contract.NowPlaying) bool { return np != nil })

	// No phone stream open: the clock passing reads nothing.
	h.settle()
	reads := plex.Reads()
	h.clk.Advance(NowPlayingPoll)
	h.settle()
	if plex.Reads() != reads {
		t.Fatal("polled with no phone connected")
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := h.phones.Subscribe(ctx, &h.ctl)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for range stream {
		}
	}()
	h.settle()
	reads = plex.Reads()
	h.clk.Advance(NowPlayingPoll)
	h.eventually("a poll while playing and watched", func() bool { return plex.Reads() > reads })
	h.settle()
	if got := plex.Reads() - reads; got != 1 {
		t.Fatalf("one poll period read the player %d times", got)
	}

	// A pause from the phone is re-read at once; a paused player is not polled.
	res := h.submit(h.ctl, h.req(contract.ActionMediaPause, nil))
	expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	h.waitNP("paused", func(np *contract.NowPlaying) bool { return np != nil && np.Status == "paused" })
	h.settle()
	reads = plex.Reads()
	h.clk.Advance(3 * NowPlayingPoll)
	h.settle()
	if plex.Reads() != reads {
		t.Fatal("polled a paused player")
	}

	// A seek is re-read at once even if the player never signals it.
	before := *h.np(&h.ctl).PositionMs
	res = h.submit(h.ctl, h.req(contract.ActionMediaSeek, map[string]any{"seconds": 30}))
	expectOutcome(t, res, contract.OutcomeDelivered, contract.CodeOK)
	h.waitNP("seek re-read", func(np *contract.NowPlaying) bool { return np != nil && *np.PositionMs == before+30_000 })

	// Closing the stream stops the poll again.
	_ = h.submit(h.ctl, h.req(contract.ActionMediaPlay, nil))
	h.waitNP("playing", func(np *contract.NowPlaying) bool { return np != nil && np.Status == "playing" })
	cancel()
	h.eventually("stream closed", func() bool {
		h.c.mu.Lock()
		defer h.c.mu.Unlock()
		return h.c.np.phoneStreams == 0
	})
	h.settle()
	reads = plex.Reads()
	h.clk.Advance(NowPlayingPoll)
	h.settle()
	if plex.Reads() != reads {
		t.Fatal("polled after the last phone left")
	}
}

func TestNowPlayingNeverLoggedOrInDiagnostics(t *testing.T) {
	h := newNPHarness(t)
	plex := demoPlex(h.clk)
	h.media.Add(adapters.PlexHTPCFlatpakID, plex)
	h.front("plexhtpc")
	h.waitNP("now playing", func(np *contract.NowPlaying) bool { return np != nil })
	for _, a := range []string{contract.ActionMediaPause, contract.ActionMediaPlay} {
		_ = h.submit(h.ctl, h.req(a, nil))
	}
	_ = h.submit(h.ctl, h.req(contract.ActionMediaSeek, map[string]any{"seconds": -10}))
	plex.Fail(os.ErrDeadlineExceeded)
	plex.Signal()
	h.waitNP("failed read", func(np *contract.NowPlaying) bool { return np == nil })
	plex.Fail(nil)
	plex.Signal()
	h.waitNP("back", func(np *contract.NowPlaying) bool { return np != nil })

	phone := h.phones.Snapshot(context.Background(), &h.ctl)
	if phone.NowPlaying == nil {
		t.Fatal("the phone should see it (the test proves the title exists)")
	}
	diag, _ := json.Marshal(map[string]any{
		"phone": doctor.Summarize(phone), "shell": doctor.Summarize(h.c.buildState(viewShell)),
	})
	shellState, _ := json.Marshal(h.c.buildState(viewShell))
	cfg, err := os.ReadFile(h.c.opts.Config.Path())
	if err != nil {
		t.Fatal(err)
	}
	for where, text := range map[string]string{"logs": h.logs.String(), "diagnostics": string(diag), "shell state": string(shellState), "config.json": string(cfg)} {
		for _, secret := range []string{"DEMO Secret Episode", "DEMO Show", "DEMO Guest", "DEMO Season"} {
			if strings.Contains(text, secret) {
				t.Errorf("%s contains %q", where, secret)
			}
		}
	}
	if h.logs.String() == "" {
		t.Fatal("no logs captured; the check proves nothing")
	}
}
