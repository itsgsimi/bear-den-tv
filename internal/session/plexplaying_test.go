// Session tests for Now playing from the Plex server (plexplaying.go): Plex
// HTPC has no MPRIS player (as on a real TV), so the connector is asked
// what this TV's Plex HTPC plays, only while a controller phone is connected
// and Plex HTPC is in front or behind Home, every PlexPlayingPoll on the fake
// clock, backing off after errors. The card is read-only (source
// plex_server, no media controls); an unidentified player is said so in the
// capability reason and the owners' diagnostics; titles never reach logs.

package session

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/plexlink"
)

// fakePlexPlaying answers NowPlaying with what the test set.
type fakePlexPlaying struct {
	mu  sync.Mutex
	p   plexlink.Playing
	err error
}

func (f *fakePlexPlaying) set(p plexlink.Playing, err error) {
	f.mu.Lock()
	f.p, f.err = p, err
	f.mu.Unlock()
}

func (f *fakePlexPlaying) NowPlaying(context.Context) (plexlink.Playing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.p, f.err
}

type plexNPHarness struct {
	*npHarness
	plex *fakePlexPlaying
}

func newPlexNPHarness(t *testing.T) *plexNPHarness {
	t.Helper()
	clk := clock.NewFake(time.Unix(1_800_000_000, 0))
	media := fake.NewMedia() // Plex HTPC has no player, as on the TV
	logs := &lockedBuffer{}
	fp := &fakePlexPlaying{p: plexlink.Playing{Status: plexlink.PlayingIdle}}
	h := newHarness(t, func(o *Options) {
		o.Clock, o.Media, o.PlexPlaying = clk, media, fp
		o.Logger = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})
	return &plexNPHarness{npHarness: &npHarness{harness: h, clk: clk, media: media, logs: logs}, plex: fp}
}

// settle waits until the Plex loop has handled every kick sent so far (the
// foreground change and the phone opening each kick it) and waits with
// nothing due, without kicking it again or moving the clock.
func (h *plexNPHarness) settle() {
	h.t.Helper()
	h.eventually("the Plex loop settled", func() bool {
		return len(h.c.plexNP.kick) == 0 && h.c.plexNP.loop.idle(h.clk.Now())
	})
}

// pass kicks (or, with d > 0, advances the clock by d), waits until the
// Plex loop is idle again (parked, no kick pending, nothing due) and returns
// the asks sent so far. It first waits for the loop to be idle before moving
// the clock: advancing while the loop is still between an ask and re-arming
// its timer would start that timer after the jump, and the poll the test
// expects would never come.
func (h *plexNPHarness) pass(d time.Duration) int64 {
	h.t.Helper()
	h.eventually("the Plex loop parked before the clock moves", func() bool {
		return len(h.c.plexNP.kick) == 0 && h.c.plexNP.loop.idle(h.clk.Now())
	})
	if d > 0 {
		h.clk.Advance(d)
	} else {
		h.c.kickPlexPlaying()
	}
	h.eventually("the Plex loop idle", func() bool {
		return len(h.c.plexNP.kick) == 0 && h.c.plexNP.loop.idle(h.clk.Now())
	})
	return h.c.plexNP.asks.Load()
}

// openPhone opens a controller phone's state stream until the test ends.
func (h *plexNPHarness) openPhone() context.CancelFunc {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := h.phones.Subscribe(ctx, &h.ctl)
	if err != nil {
		h.t.Fatal(err)
	}
	go func() {
		for range stream {
		}
	}()
	h.t.Cleanup(cancel)
	return cancel
}

var demoPlexServer = plexlink.Playing{Status: plexlink.PlayingFound, Title: "DEMO Episode 3: The Long Winter", Subtitle: "DEMO Show",
	State: "playing", Offset: 754 * time.Second, Duration: 44 * time.Minute}

func TestPlexServerNowPlaying(t *testing.T) {
	h := newPlexNPHarness(t)
	h.plex.set(demoPlexServer, nil)
	h.front("plexhtpc")
	h.eventually("Plex in front, media probed", func() bool {
		cp := h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPause]
		return h.c.Target().Kind == "app" && !strings.HasPrefix(cp.Reason, "Checking")
	})

	// No phone connected: nothing is asked.
	if asks := h.pass(0); asks != 0 {
		t.Fatalf("asked the server %d times with no phone connected", asks)
	}
	if np := h.np(&h.ctl); np != nil {
		t.Fatalf("now playing without a reading: %+v", np)
	}

	closePhone := h.openPhone()
	np := h.waitNP("the server's reading", func(np *contract.NowPlaying) bool { return np != nil })
	if np.AppID != "plex-htpc" || np.Title != demoPlexServer.Title || np.Subtitle != "DEMO Show" || np.Status != contract.NowPlayingPlaying ||
		np.Source == nil || *np.Source != contract.NowPlayingSourcePlexServer || np.Foreground == nil || !*np.Foreground ||
		np.LengthMs == nil || *np.LengthMs != 2_640_000 || np.PositionMs == nil || *np.PositionMs != 754_000 || np.Rate != 1 {
		t.Fatalf("reading %+v", np)
	}
	st := h.phones.Snapshot(context.Background(), &h.ctl)
	if _, err := contract.MarshalAndValidateState(st); err != nil {
		t.Fatal(err)
	}
	if cp := st.Capabilities[contract.ActionMediaPause]; cp.Available || !strings.Contains(cp.Reason, "read-only") {
		t.Fatalf("media controls for a Plex server reading: %+v", cp)
	}
	res := h.submit(h.ctl, h.req(contract.ActionMediaPause, nil))
	expectOutcome(t, res, contract.OutcomeFailed, contract.CodeUnsupported)
	if sh := h.c.buildState(viewShell); sh.NowPlaying != nil {
		t.Fatal("the shell view carries now_playing")
	}

	// Every PlexPlayingPoll one ask, never more.
	asks := h.c.plexNP.asks.Load()
	if got := h.pass(PlexPlayingPoll); got != asks+1 {
		t.Fatalf("one poll period asked %d times", got-asks)
	}

	// Buffering: playing at rate 0 (phones hold the position).
	b := demoPlexServer
	b.State = "buffering"
	h.plex.set(b, nil)
	h.pass(PlexPlayingPoll)
	if np := h.np(&h.ctl); np == nil || np.Status != contract.NowPlayingPlaying || np.Rate != 0 {
		t.Fatalf("buffering %+v", np)
	}

	// Home: Plex keeps playing behind it (its pause cannot be verified).
	h.plex.set(demoPlexServer, nil)
	h.desk.SetActive(h.shellW)
	np = h.waitNP("behind home", behindHome)
	if np.AppID != "plex-htpc" || *np.Source != contract.NowPlayingSourcePlexServer {
		t.Fatalf("behind Home %+v", np)
	}
	if cp := h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPlay]; cp.Available || !strings.Contains(cp.Reason, "read-only") {
		t.Fatalf("media behind Home for a Plex server reading: %+v", cp)
	}

	// Locked: nothing shown, nothing asked.
	h.lock.set(true)
	h.waitNP("locked", func(np *contract.NowPlaying) bool { return np == nil })
	asks = h.pass(0)
	if got := h.pass(PlexPlayingPoll); got != asks {
		t.Fatal("asked the server while locked")
	}
	h.lock.set(false)
	h.waitNP("unlocked", behindHome)

	// Closing the phone stops the asks.
	closePhone()
	h.eventually("stream closed", func() bool {
		h.c.mu.Lock()
		defer h.c.mu.Unlock()
		return h.c.np.phoneStreams == 0
	})
	asks = h.pass(PlexPlayingPoll)
	if got := h.pass(PlexPlayingPoll); got != asks {
		t.Fatal("asked the server after the last phone left")
	}

	if logs := h.logs.String(); strings.Contains(logs, "DEMO Episode") || strings.Contains(logs, "DEMO Show") {
		t.Fatalf("a title reached the log:\n%s", logs)
	}
}

func TestPlexServerUnidentifiedAndErrorsBackOff(t *testing.T) {
	h := newPlexNPHarness(t)
	why := "Your Plex server lists Plex HTPC playing, but not from this TV's addresses, so Bear Den cannot tell whether it is this TV."
	h.plex.set(plexlink.Playing{Status: plexlink.PlayingUnidentified, Reason: why}, nil)
	h.front("plexhtpc")
	h.openPhone()
	h.settle()
	h.eventually("the reason on the phone", func() bool {
		return h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPause].Reason == why
	})
	if np := h.np(&h.ctl); np != nil {
		t.Fatalf("an unidentified player was shown: %+v", np)
	}
	diag, err := h.phones.Diagnostics(context.Background(), h.owner)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := diag["plex_now_playing"].(map[string]any); d["status"] != plexlink.PlayingUnidentified || d["reason"] != why {
		t.Fatalf("diagnostics %v", diag["plex_now_playing"])
	}

	// The server cannot be asked: nothing shown, and the next ask waits for
	// the backoff (10 s), not the poll (5 s).
	h.settle()
	h.plex.set(plexlink.Playing{}, errors.New("plex: server returned 403 for /status/sessions"))
	h.pass(PlexPlayingPoll)
	asks := h.c.plexNP.asks.Load()
	if got := h.pass(PlexPlayingPoll); got != asks {
		t.Fatalf("asked again before the backoff; log:\n%s", h.logs.String())
	}
	if got := h.pass(PlexPlayingBackoff[0] - PlexPlayingPoll); got != asks+1 {
		t.Fatalf("no ask after the backoff (%d)", got-asks)
	}
	diag, _ = h.phones.Diagnostics(context.Background(), h.owner)
	if d, _ := diag["plex_now_playing"].(map[string]any); d["status"] != "error" {
		t.Fatalf("diagnostics after an error %v", d)
	}
	// Back: shown again on the next ask.
	h.plex.set(demoPlexServer, nil)
	h.pass(PlexPlayingBackoff[1])
	h.waitNP("back after errors", func(np *contract.NowPlaying) bool { return np != nil })
}

func TestPlexServerNotAskedForOtherApps(t *testing.T) {
	h := newPlexNPHarness(t)
	h.plex.set(demoPlexServer, nil)
	h.openPhone()
	h.front("moonlight")
	h.eventually("Moonlight in front", func() bool { return h.c.Target().Kind == "app" })
	h.pass(0)
	if got := h.pass(PlexPlayingPoll); got != 0 {
		t.Fatalf("asked the Plex server %d times with Moonlight in front", got)
	}
	if np := h.np(&h.ctl); np != nil {
		t.Fatalf("Plex shown over Moonlight: %+v", np)
	}
}
