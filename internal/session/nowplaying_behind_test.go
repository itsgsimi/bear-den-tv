// Session tests for Now playing behind Home (nowplaying.go, route.go
// doMedia, state.go): the app that was in front keeps its card, marked
// foreground false, while its own player plays or is paused behind the
// shell; play/pause/seek reach it only when a request names it; the privacy
// rules stay those of Now playing (controller phones and guests, never
// while locked, never the shell view, never logged).

package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
)

func behindHome(np *contract.NowPlaying) bool {
	return np != nil && np.Foreground != nil && !*np.Foreground
}

func TestNowPlayingBehindHome(t *testing.T) {
	h := newNPHarness(t)
	yt := fake.NewPlayer(h.clk, platform.MediaInfo{Status: "Playing", Title: "DEMO Video: Building a Cabin", Artists: []string{"DEMO Channel"},
		Length: 18 * time.Minute, Position: 3 * time.Minute, HasPosition: true, Rate: 1})
	h.media.Add(adapters.VacuumTubeFlatpakID, yt)
	ytWin := h.front("vacuumtube")
	np := h.waitNP("youtube in front", func(np *contract.NowPlaying) bool { return np != nil && np.AppID == "youtube" })
	if np.Foreground == nil || !*np.Foreground {
		t.Fatalf("in front: foreground %v", np.Foreground)
	}

	// Home with YouTube left playing (its Home pause could not be verified).
	h.desk.SetActive(h.shellW)
	np = h.waitNP("youtube behind home", behindHome)
	if np.AppID != "youtube" || np.Status != contract.NowPlayingPlaying || np.Title != "DEMO Video: Building a Cabin" {
		t.Fatalf("behind Home: %+v", np)
	}
	if _, err := contract.MarshalAndValidateState(h.phones.Snapshot(context.Background(), &h.ctl)); err != nil {
		t.Fatalf("snapshot with a behind-Home card: %v", err)
	}
	if st := h.c.buildState(viewShell); st.NowPlaying != nil {
		t.Fatal("the shell view carries now_playing")
	}
	guest := guestViewer(time.Now().Add(time.Hour))
	if np := h.phones.Snapshot(context.Background(), &guest).NowPlaying; !behindHome(np) {
		t.Fatalf("a guest (same screen) does not see it: %+v", np)
	}
	cp := h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPause]
	if !cp.Available || cp.Backend != backendMPRIS {
		t.Fatalf("media.pause behind Home: %+v", cp)
	}

	// "active" is the shell: refused, and nothing reaches the player.
	res := h.submit(h.ctl, h.req(contract.ActionMediaPause, nil))
	expectOutcome(t, res, contract.OutcomeFailed, contract.CodeTargetUnfocused)
	if len(yt.Calls()) != 0 {
		t.Fatalf("an \"active\" request reached the player behind Home: %v", yt.Calls())
	}
	// Named, it pauses YouTube behind Home, observed.
	req := h.req(contract.ActionMediaPause, nil)
	req.Target = "youtube"
	res = h.submit(h.ctl, req)
	expectOutcome(t, res, contract.OutcomeObserved, contract.CodeOK)
	np = h.waitNP("paused behind home", func(np *contract.NowPlaying) bool { return behindHome(np) && np.Status == contract.NowPlayingPaused })
	req = h.req(contract.ActionMediaSeek, map[string]any{"seconds": 10})
	req.Target = "youtube"
	expectOutcome(t, h.submit(h.ctl, req), contract.OutcomeDelivered, contract.CodeOK)
	req = h.req(contract.ActionMediaPlay, nil)
	req.Target = "plex-htpc"
	expectOutcome(t, h.submit(h.ctl, req), contract.OutcomeFailed, contract.CodeTargetUnfocused)
	if got := strings.Join(yt.Calls(), ","); got != "Pause,Seek" {
		t.Fatalf("player calls %s", got)
	}

	// Locked: nothing.
	h.lock.set(true)
	h.waitNP("locked hides it", func(np *contract.NowPlaying) bool { return np == nil })
	h.lock.set(false)
	h.waitNP("back after unlock", behindHome)

	// Stopped behind Home: nothing, and no media controls from the shell.
	yt.Set(platform.MediaInfo{Status: "Stopped", Title: "DEMO Video: Building a Cabin"})
	h.waitNP("stopped hides it", func(np *contract.NowPlaying) bool { return np == nil })
	if cp := h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPlay]; cp.Available {
		t.Fatalf("media.play with nothing behind Home: %+v", cp)
	}
	yt.Set(platform.MediaInfo{Status: "Paused", Title: "DEMO Video: Building a Cabin"})
	h.waitNP("paused again", behindHome)

	// The app exits: nothing.
	h.desk.RemoveWindow(ytWin)
	yt.Fail(errors.New("org.freedesktop.DBus.Error.ServiceUnknown"))
	yt.Signal()
	h.waitNP("exit clears it", func(np *contract.NowPlaying) bool { return np == nil })

	if logs := h.logs.String(); strings.Contains(logs, "DEMO Video") || strings.Contains(logs, "DEMO Channel") {
		t.Fatalf("a title reached the log:\n%s", logs)
	}
}

func TestAnotherAppInFrontEndsBehindHome(t *testing.T) {
	h := newNPHarness(t)
	h.media.Add(adapters.VacuumTubeFlatpakID, fake.NewPlayer(h.clk, platform.MediaInfo{Status: "Playing", Title: "DEMO Video"}))
	h.front("vacuumtube")
	h.waitNP("youtube", func(np *contract.NowPlaying) bool { return np != nil })
	h.desk.SetActive(h.shellW)
	h.waitNP("behind home", behindHome)
	// Moonlight (no player) in front, then Home again: YouTube is no longer
	// the app behind Home, though its player still plays.
	h.front("moonlight")
	h.waitNP("moonlight clears it", func(np *contract.NowPlaying) bool { return np == nil })
	// Through a window Bear Den does not know, which leaves "behind" alone.
	h.front("some.other.Program")
	h.eventually("unknown window in front", func() bool { return h.c.Target().Kind == "unknown" })
	h.desk.SetActive(h.shellW)
	h.eventually("shell in front", func() bool { return h.c.Target().Kind == "shell" })
	h.settle()
	if np := h.np(&h.ctl); np != nil {
		t.Fatalf("an app that is not the one behind Home was shown: %+v", np)
	}
}

func TestWebAppBehindHomeIsReadOnly(t *testing.T) {
	media := fake.NewMedia()
	h, fw, _ := webHarness(t, func(o *Options) { o.Media = media })
	if r := enable(h, "netflix", true); !r.OK {
		t.Fatal(r.Error)
	}
	launchWeb(h, "netflix")
	film := fake.NewPlayer(clock.Real{}, platform.MediaInfo{Status: "Playing", Title: "DEMO Film"})
	media.Add(fake.ProcessKey(fw.PID("netflix")), film)
	h.c.probeMedia(context.Background())
	h.desk.SetActive(h.shellW)
	h.eventually("netflix behind home", func() bool {
		return behindHome(h.phones.Snapshot(context.Background(), &h.ctl).NowPlaying)
	})
	cp := h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPause]
	if cp.Available || cp.Reason != "Open Netflix to control it." {
		t.Fatalf("a web page's video behind Home: %+v", cp)
	}
	req := h.req(contract.ActionMediaPause, nil)
	req.Target = "netflix"
	expectOutcome(t, h.submit(h.ctl, req), contract.OutcomeFailed, contract.CodeUnsupported)
	if len(film.Calls()) != 0 {
		t.Fatalf("MPRIS reached a web page's video: %v", film.Calls())
	}
}
