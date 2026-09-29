// Tests for Home's verified pause of a native app (homepause.go): MPRIS
// Pause only when the app's own player reports Playing first and Paused
// after; never a key, never another app's player, never a claim without
// the second reading.
package session

import (
	"context"
	"slices"
	"testing"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
)

// TestHomePause: short subtest names keep the harness socket path under the
// Unix limit.
func TestHomePause(t *testing.T) {
	playing := platform.MediaInfo{Status: "Playing", Title: "DEMO Film"}
	for _, tc := range []struct {
		name       string
		class      string // WM_CLASS of the app in front
		match      string // where its player is registered ("" none)
		info       platform.MediaInfo
		setup      func(h *harness, p *fake.Player)
		wantPause  bool // Pause reached the player
		wantDetail any  // detail["paused"]; nil means absent
	}{
		{name: "plex", class: "plexhtpc", match: adapters.PlexHTPCFlatpakID, info: playing, wantPause: true, wantDetail: true},
		{name: "jellyfin", class: "org.jellyfin.JellyfinDesktop", match: adapters.JellyfinFlatpakID, info: playing, wantPause: true, wantDetail: true},
		{name: "already-paused", class: "org.jellyfin.JellyfinDesktop", match: adapters.JellyfinFlatpakID,
			info: platform.MediaInfo{Status: "Paused", Title: "DEMO Film"}, wantDetail: false},
		{name: "ignores-pause", class: "org.jellyfin.JellyfinDesktop", match: adapters.JellyfinFlatpakID, info: playing,
			setup: func(_ *harness, p *fake.Player) { p.SetStuck(true) }, wantPause: true, wantDetail: false},
		{name: "no-control", class: "plexhtpc", match: adapters.PlexHTPCFlatpakID, info: playing,
			setup: func(_ *harness, p *fake.Player) { p.SetCanControl(false) }, wantDetail: false},
		{name: "other-player", class: "vacuumtube", match: adapters.PlexHTPCFlatpakID, info: playing, wantDetail: false},
		{name: "retroarch-toggle", class: "retroarch", wantDetail: false},
		{name: "spotify", class: "spotify", match: adapters.SpotifyFlatpakID, info: playing},
		{name: "leave-running", class: "plexhtpc", match: adapters.PlexHTPCFlatpakID, info: playing,
			setup: func(h *harness, _ *fake.Player) {
				if _, err := h.c.opts.Config.Update(func(c *config.Config) error {
					for i := range c.Applications {
						if c.Applications[i].Adapter == adapters.PlexHTPCName {
							c.Applications[i].HomePolicy = "leave-running"
						}
					}
					return nil
				}); err != nil {
					h.t.Fatal(err)
				}
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			media := fake.NewMedia()
			h := newHarness(t, func(o *Options) { o.Media = media })
			player := fake.NewPlayer(clock.Real{}, tc.info)
			if tc.match != "" {
				media.Add(tc.match, player)
			}
			if tc.setup != nil {
				tc.setup(h, player)
			}
			w := h.desk.AddWindow(platform.WindowInfo{PID: 9200, Class: []string{tc.class, tc.class}, Mapped: true})
			h.desk.SetActive(w)
			h.eventually("app in front", func() bool { return h.c.Target().Kind == "app" })

			req := contract.ActionRequest{Protocol: 1, RequestID: "home-" + randomID(), Target: "shell", Action: contract.ActionHome, Args: map[string]any{}}
			res := h.c.doHome(context.Background(), sender{key: shellSender}, req)
			if res.Outcome != contract.OutcomeObserved {
				t.Fatalf("home: %+v", res)
			}
			if got := slices.Contains(player.Calls(), "Pause"); got != tc.wantPause {
				t.Fatalf("Pause sent = %v, want %v (calls %v)", got, tc.wantPause, player.Calls())
			}
			if slices.Contains(player.Calls(), "Play") {
				t.Fatalf("Home resumed the player: %v", player.Calls())
			}
			if keys := h.desk.Keys(); len(keys) != 0 {
				t.Fatalf("Home sent keys to the app: %v", keys)
			}
			if got := res.Detail["paused"]; got != tc.wantDetail {
				t.Fatalf("detail.paused = %v, want %v (detail %v)", got, tc.wantDetail, res.Detail)
			}
		})
	}
}
