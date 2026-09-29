// The web apps' browser (browsers.go, IPC apps.browser): the choice moves
// the Browser tile or the streaming sites to that browser's Flatpak in
// config.json and nowhere else, the streaming sites stay in Chromium until
// the owner moves them too, installs and launches follow the row, Widevine
// is prepared in the new browser, and a browser outside the table is
// refused with nothing changed.

package session

import (
	"fmt"
	"testing"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/shellipc"
)

func setBrowsers(h *harness, browser, streaming string) shellipc.Result {
	h.t.Helper()
	id := "browser-" + randomID()
	return h.shellSend(shellipc.AppsBrowser{Type: shellipc.TypeAppsBrowser, RequestID: id, Browser: browser, StreamingBrowser: streaming}, id)
}

func rowBrowser(h *harness, appID string) string {
	a, _ := h.c.opts.Config.Current().Application(appID)
	return a.Launch.AppID
}

func TestBrowserChoiceMovesOnlyItsWebApps(t *testing.T) {
	fd := &fakeDRM{ready: map[string]bool{}, release: make(chan string, 4)}
	var fw *fakeWeb
	h, fi, sl := installHarness(t, func(o *Options) {
		fw = newFakeWeb(o.Desktop.(*fake.Desktop))
		o.Web, o.DRM = fw, fd
	})
	sl.setScope(adapters.ChromiumFlatpakID, "user")
	h.c.rediscover(t.Context(), adapters.ChromiumFlatpakID)
	h.eventually("chromium discovered", func() bool { return appState(h.c.buildState(viewShell), "browser").Installed })
	if r := enable(h, "netflix", true); !r.OK {
		t.Fatal(r.Error)
	}
	h.eventually("netflix preparing in chromium", func() bool { return drm(h, "netflix") == contract.DRMPreparing })

	// The choices and the table reach the shell.
	apps := h.c.buildState(viewShell).Apps
	if apps.Browser != "chromium" || apps.StreamingBrowser != "chromium" || len(apps.Browsers) != 2 ||
		apps.Browsers[1] != (contract.BrowserOption{ID: "brave", Label: "Brave", FlatpakID: adapters.BraveFlatpakID, StreamingUnverified: true}) ||
		apps.Browsers[0].StreamingUnverified {
		t.Fatalf("state.apps %+v", apps)
	}

	// The Browser tile moves to Brave; the streaming sites stay in Chromium.
	if r := setBrowsers(h, "brave", "chromium"); !r.OK {
		t.Fatal(r.Error)
	}
	if rowBrowser(h, "browser") != adapters.BraveFlatpakID || rowBrowser(h, "netflix") != adapters.ChromiumFlatpakID || rowBrowser(h, "hulu") != adapters.ChromiumFlatpakID {
		t.Fatalf("rows: browser %s netflix %s hulu %s", rowBrowser(h, "browser"), rowBrowser(h, "netflix"), rowBrowser(h, "hulu"))
	}
	if st := h.c.buildState(viewShell); st.Apps.Browser != "brave" || st.Apps.StreamingBrowser != "chromium" {
		t.Fatalf("state.apps %+v", st.Apps)
	}
	// Brave is not installed: the tile says so, and Install fetches Brave
	// from the table, not Chromium.
	h.eventually("browser not installed in brave", func() bool {
		a := appState(h.c.buildState(viewShell), "browser")
		return !a.Installed && a.Installation == "none"
	})
	expectOutcome(t, h.submit(h.owner, h.req(contract.ActionAppInstall, map[string]any{"app_id": "browser"})), contract.OutcomeDelivered, contract.CodeOK)
	if starts, _, _ := fi.calls(); fmt.Sprint(starts) != "["+adapters.BraveFlatpakID+"]" {
		t.Fatalf("installs %v", starts)
	}

	// Launches follow the rows.
	sl.setScope(adapters.BraveFlatpakID, "user")
	h.c.rediscover(t.Context(), adapters.BraveFlatpakID)
	for _, id := range []string{"browser", "netflix"} {
		res := h.submit(h.ctl, h.req(contract.ActionAppLaunch, map[string]any{"app_id": id}))
		if res.Outcome == contract.OutcomeFailed {
			t.Fatalf("launch %s: %+v", id, res)
		}
		h.eventually(id+" in front", func() bool { return h.c.Target().AppID != nil && *h.c.Target().AppID == id })
	}
	fw.mu.Lock()
	launched := fmt.Sprint(fw.launched)
	fw.mu.Unlock()
	if launched != "[browser "+adapters.BraveFlatpakID+" netflix "+adapters.ChromiumFlatpakID+"]" {
		t.Fatalf("launched %s", launched)
	}

	// The streaming sites move too: the enabled one is prepared in Brave.
	if r := setBrowsers(h, "brave", "brave"); !r.OK {
		t.Fatal(r.Error)
	}
	for _, id := range []string{"netflix", "disney-plus", "hulu"} {
		if rowBrowser(h, id) != adapters.BraveFlatpakID {
			t.Fatalf("%s runs in %s", id, rowBrowser(h, id))
		}
	}
	// (Turning Netflix on prepared it in Chromium; opening it stopped that run.)
	h.eventually("netflix prepared in brave", func() bool {
		fd.mu.Lock()
		defer fd.mu.Unlock()
		return fmt.Sprint(fd.prepared) == "[netflix netflix]" && fmt.Sprint(fd.browsers) == "["+adapters.ChromiumFlatpakID+" "+adapters.BraveFlatpakID+"]"
	})

	// Keep apps up to date leaves the browsers alone.
	if r := h.shellSend(shellipc.AppsConfigure{Type: shellipc.TypeAppsConfigure, RequestID: "au", AutoUpdate: false}, "au"); !r.OK {
		t.Fatal(r.Error)
	}
	if cfg := h.c.opts.Config.Current(); cfg.BrowserName() != "brave" || cfg.StreamingBrowserName() != "brave" || cfg.AutoUpdate() {
		t.Fatalf("apps %+v", cfg.Apps)
	}

	// A browser outside the table: refused, nothing changes.
	rev := h.c.opts.Config.Current().Revision
	if r := setBrowsers(h, "firefox", "chromium"); r.OK {
		t.Fatal("an unknown browser was stored")
	}
	if cfg := h.c.opts.Config.Current(); cfg.Revision != rev || rowBrowser(h, "netflix") != adapters.BraveFlatpakID {
		t.Fatalf("config changed: revision %d → %d", rev, cfg.Revision)
	}
}
