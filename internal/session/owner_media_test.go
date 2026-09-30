// The TV observation end to end through the coordinator: VacuumTube's
// player exactly as seen on a real TV (org.mpris.MediaPlayer2.chromium.instance3,
// Identity "VacuumTube", no DesktopEntry, PlaybackStatus "Playing") on an
// in-memory bus, read by the real mpris.Locator with only the pid-to-Flatpak
// lookup faked. Media capabilities, Now playing and Home's verified pause
// must all work for it, and must not for an app whose Flatpak does not own it.

package session

import (
	"context"
	"errors"
	"sync"
	"testing"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/dbusx"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/platform/mpris"
)

const observedBusName = "org.mpris.MediaPlayer2.chromium.instance3"

// observedBus serves one player shaped like the observation; Pause and Play
// change PlaybackStatus like the real player.
type observedBus struct {
	mu     sync.Mutex
	status string
	calls  []string
}

func (o *observedBus) bus() *dbusx.Fake {
	return &dbusx.Fake{
		NamesFn: func(context.Context) ([]string, error) {
			return []string{"org.freedesktop.DBus", ":1.40", observedBusName}, nil
		},
		PropertyFn: func(_ context.Context, dest, _, _, name string) (any, error) {
			if dest != observedBusName {
				return nil, errors.New("org.freedesktop.DBus.Error.ServiceUnknown")
			}
			o.mu.Lock()
			defer o.mu.Unlock()
			switch name {
			case "Identity":
				return "VacuumTube", nil
			case "PlaybackStatus":
				return o.status, nil
			case "CanControl", "CanPause", "CanPlay", "CanSeek":
				return true, nil
			case "Metadata":
				return map[string]any{"xesam:title": "DEMO Video: Building a Cabin", "xesam:artist": []string{"DEMO Channel"}}, nil
			}
			// DesktopEntry among others: the TV's Get returned an error.
			return nil, errors.New("org.freedesktop.DBus.Error.InvalidArgs: No such property " + name)
		},
		CallFn: func(_ context.Context, dest, _, method string, args ...any) ([]any, error) {
			switch method {
			case "org.freedesktop.DBus.GetConnectionUnixProcessID":
				if len(args) == 1 && args[0] == observedBusName {
					return []any{uint32(4242)}, nil
				}
				return nil, errors.New("org.freedesktop.DBus.Error.NameHasNoOwner")
			case "org.freedesktop.DBus.GetNameOwner":
				return []any{":1.40"}, nil
			}
			if dest != observedBusName {
				return nil, errors.New("org.freedesktop.DBus.Error.ServiceUnknown")
			}
			o.mu.Lock()
			defer o.mu.Unlock()
			o.calls = append(o.calls, method)
			switch method {
			case "org.mpris.MediaPlayer2.Player.Pause":
				o.status = "Paused"
			case "org.mpris.MediaPlayer2.Player.Play":
				o.status = "Playing"
			}
			return nil, nil
		},
	}
}

// ownerProcs is the pid-to-Flatpak lookup (proc.Table on the TV).
type ownerProcs map[int]string

func (p ownerProcs) FlatpakID(pid int) (string, error) { return p[pid], nil }
func (p ownerProcs) DescendsFrom(int, int) (bool, error) {
	return false, nil
}

func TestObservedVacuumTubeGetsMediaNowPlayingAndHomePause(t *testing.T) {
	ob := &observedBus{status: "Playing"}
	locator := mpris.NewLocator(ob.bus()).WithProcesses(ownerProcs{4242: adapters.VacuumTubeFlatpakID})
	h := newHarness(t, func(o *Options) { o.Media = locator })
	w := h.desk.AddWindow(platform.WindowInfo{PID: 9300, Class: []string{"vacuumtube", "VacuumTube"}, Mapped: true})
	h.desk.SetActive(w)
	h.eventually("VacuumTube in front", func() bool { return h.c.Target().Kind == "app" })

	h.eventually("media controls for VacuumTube", func() bool {
		return h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPause].Available
	})
	var np *contract.NowPlaying
	h.eventually("Now playing for VacuumTube", func() bool {
		np = h.phones.Snapshot(context.Background(), &h.ctl).NowPlaying
		return np != nil
	})
	if np.Title != "DEMO Video: Building a Cabin" || np.Status != contract.NowPlayingPlaying {
		t.Fatalf("now playing %+v", np)
	}

	req := contract.ActionRequest{Protocol: 1, RequestID: "home-" + randomID(), Target: "shell", Action: contract.ActionHome, Args: map[string]any{}}
	res := h.c.doHome(context.Background(), sender{key: shellSender}, req)
	if res.Outcome != contract.OutcomeObserved || res.Detail["paused"] != true {
		t.Fatalf("home: %+v", res)
	}
	ob.mu.Lock()
	defer ob.mu.Unlock()
	if ob.status != "Paused" || len(ob.calls) != 1 || ob.calls[0] != "org.mpris.MediaPlayer2.Player.Pause" {
		t.Fatalf("player after Home: %s, calls %v", ob.status, ob.calls)
	}
}

func TestPlayerOfAnotherFlatpakGivesNoMediaControls(t *testing.T) {
	ob := &observedBus{status: "Playing"}
	// The same player, but its owner runs in Plex HTPC's Flatpak.
	locator := mpris.NewLocator(ob.bus()).WithProcesses(ownerProcs{4242: adapters.PlexHTPCFlatpakID})
	h := newHarness(t, func(o *Options) { o.Media = locator })
	w := h.desk.AddWindow(platform.WindowInfo{PID: 9300, Class: []string{"vacuumtube", "VacuumTube"}, Mapped: true})
	h.desk.SetActive(w)
	h.eventually("VacuumTube in front with its media checked", func() bool {
		cp := h.phones.Snapshot(context.Background(), &h.ctl).Capabilities[contract.ActionMediaPause]
		return h.c.Target().Kind == "app" && cp.Reason == noMediaControls("YouTube")
	})
	req := contract.ActionRequest{Protocol: 1, RequestID: "home-" + randomID(), Target: "shell", Action: contract.ActionHome, Args: map[string]any{}}
	if res := h.c.doHome(context.Background(), sender{key: shellSender}, req); res.Detail["paused"] != false {
		t.Fatalf("home paused another Flatpak's player: %+v", res)
	}
	ob.mu.Lock()
	defer ob.mu.Unlock()
	if len(ob.calls) != 0 {
		t.Fatalf("calls reached a player VacuumTube does not own: %v", ob.calls)
	}
}

func TestWebAppNowPlayingOnlyFromItsOwnBrowser(t *testing.T) {
	media := fake.NewMedia()
	h, fw, _ := webHarness(t, func(o *Options) { o.Media = media })
	// Chrome's Flatpak id alone (shared by every streaming site) must never do.
	media.Add(adapters.ChromeFlatpakID, fake.NewPlayer(clock.Real{}, platform.MediaInfo{Status: "Playing", Title: "DEMO Other Tab"}))
	if r := enable(h, "netflix", true); !r.OK {
		t.Fatal(r.Error)
	}
	launchWeb(h, "netflix")
	h.eventually("media probed for Netflix", func() bool {
		h.c.mu.Lock()
		defer h.c.mu.Unlock()
		return h.c.media != nil && h.c.media.appID == "netflix"
	})
	h.c.probeMedia(context.Background())
	if np := h.phones.Snapshot(context.Background(), &h.ctl).NowPlaying; np != nil {
		t.Fatalf("a player of the shared Flatpak was shown for Netflix: %+v", np)
	}
	media.Add(fake.ProcessKey(fw.PID("netflix")), fake.NewPlayer(clock.Real{}, platform.MediaInfo{Status: "Playing", Title: "DEMO Film"}))
	h.c.probeMedia(context.Background())
	h.eventually("Netflix's own player", func() bool {
		np := h.phones.Snapshot(context.Background(), &h.ctl).NowPlaying
		return np != nil && np.Title == "DEMO Film" && np.AppID == "netflix"
	})
}
