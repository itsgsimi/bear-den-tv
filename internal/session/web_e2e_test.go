// End-to-end: the coordinator launches the toolchain's Playwright Chromium
// (not Flatpak) as the Browser tile against a local fixture page, with the
// real web manager, the DevTools pipe and the injected navigation script,
// and a phone drives it with named actions. The desktop is the fake one
// (a window with the adapter's class stands in for Chromium's). Skips,
// saying why, without a Playwright Chromium (BDTV_TEST_CHROMIUM or
// `npx playwright install --with-deps chromium` in apps/web-nav); fails
// with Chromium's own error when it is installed but cannot start (webtest).

package session

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bear-den-tv/internal/applications"
	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/web"
	"bear-den-tv/internal/applications/web/webtest"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform"
	"bear-den-tv/internal/platform/fake"
)

// windowedWeb is the real manager plus a fake window for the fake desktop.
type windowedWeb struct {
	*web.Manager
	desk *fake.Desktop
}

func (w windowedWeb) Launch(ctx context.Context, app config.Application, spec adapters.WebSpec) (applications.Instance, error) {
	inst, err := w.Manager.Launch(ctx, app, spec)
	if err == nil {
		win := w.desk.AddWindow(platform.WindowInfo{PID: inst.PID, Class: []string{"fixtures.test", spec.Class}})
		w.desk.SetActive(win)
	}
	return inst, err
}

func TestE2ECoordinatorDrivesChromiumWithPhoneActions(t *testing.T) {
	bin := webtest.Require(t)
	dir, _ := filepath.Abs("../../apps/web-nav/tests/fixtures")
	srv := httptest.NewUnstartedServer(http.FileServer(http.Dir(dir)))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	defer srv.Close()
	data, _ := os.MkdirTemp("", "bdtv-session-e2e-")
	defer func() { time.Sleep(200 * time.Millisecond); _ = os.RemoveAll(data) }()

	var mgr *web.Manager
	h := newHarness(t, func(o *Options) {
		fd := o.Desktop.(*fake.Desktop)
		mgr = web.NewManager(web.Options{DataHome: data, Starter: web.ExecStarter{Env: os.Environ(), Prefix: []string{bin,
			"--headless=new", "--no-sandbox", "--ignore-certificate-errors", "--window-size=1280,720",
			"--host-resolver-rules=MAP fixtures.test " + strings.TrimPrefix(srv.URL, "https://"),
		}}})
		o.Web = windowedWeb{Manager: mgr, desk: fd}
	})
	defer func() { _ = mgr.Close(context.Background(), "browser", true) }()
	if _, err := h.c.opts.Config.Update(func(c *config.Config) error {
		for i := range c.Applications {
			if c.Applications[i].ID == "browser" {
				c.Applications[i].Web = &config.Web{URL: "https://fixtures.test/grid.html"}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	launchWeb(h, "browser")
	h.eventually("the page's first report", func() bool {
		st, ok := mgr.Status("browser")
		return ok && st.Viewport.W > 0
	})
	h.eventually("the web capabilities", func() bool {
		c := h.c.buildState(viewShell).Capabilities
		return c[contract.ActionNavRight].Available && c[contract.ActionPointerMove].Available
	})
	// The grid may still be loading: retry the first move until the page
	// has focus targets.
	var res contract.ActionResult
	h.eventually("a first move on the grid", func() bool {
		res = h.submit(h.ctl, h.req(contract.ActionNavRight, nil))
		return res.Outcome == contract.OutcomeObserved
	})
	if res.Detail["page"] != "moved" || res.Detail["focus_role"] != "link" {
		t.Fatalf("nav.right: %+v", res)
	}
	res = h.submit(h.ctl, h.req(contract.ActionNavDown, nil))
	if res.Outcome != contract.OutcomeObserved || res.Detail["focus_index"] == nil {
		t.Fatalf("nav.down: %+v", res)
	}
	if res = h.submit(h.ctl, h.req(contract.ActionSelect, nil)); res.Outcome != contract.OutcomeDelivered || res.Detail["page"] != "click" {
		t.Fatalf("select: %+v", res)
	}
	h.eventually("back closes the overlay the click opened", func() bool {
		res = h.submit(h.ctl, h.req(contract.ActionBack, nil))
		return res.Detail["page"] == "closing_overlay"
	})
	if res = h.submit(h.ctl, h.req(contract.ActionPointerMove, map[string]any{"dx": 40.0, "dy": 20.0})); res.Outcome != contract.OutcomeDelivered {
		t.Fatalf("pointer.move: %+v", res)
	}
	if res = h.submit(h.ctl, h.req(contract.ActionPointerScroll, map[string]any{"dy": 200.0})); res.Outcome != contract.OutcomeDelivered {
		t.Fatalf("pointer.scroll: %+v", res)
	}
	if res = h.submit(h.ctl, h.req(contract.ActionMediaPause, nil)); res.Outcome != contract.OutcomeFailed {
		t.Fatalf("media.pause on a page without video: %+v", res)
	}
	// Home: nothing is playing, so nothing is pressed; the shell comes back.
	h.submit(h.ctl, h.req(contract.ActionHome, nil))
	h.eventually("shell in front", func() bool { return h.c.Target().Kind == "shell" })
	// Close ends Chromium; the app stops running.
	if res = h.c.doClose(context.Background(), h.req(contract.ActionAppClose, map[string]any{"app_id": "browser", "force": true})); res.Outcome != contract.OutcomeDelivered {
		t.Fatalf("close: %+v", res)
	}
	h.eventually("Chromium to exit", func() bool { return !mgr.Running("browser") })
}
