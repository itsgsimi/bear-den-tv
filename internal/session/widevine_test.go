// Playback support after Chromium is installed (widevine.go): the enabled
// streaming sites' profiles get one quiet run each, never the Browser's or a
// turned-off site's; install.drm says preparing, then ready or pending;
// opening the site stops its quiet run; turning a site on prepares it.

package session

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/install"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform/fake"
)

// fakeDRM's quiet runs wait until the test lets them finish (release) or
// they are cancelled. Readiness is per app and browser ("netflix@<flatpak
// id>"): a profile in one browser is not a profile in another.
type fakeDRM struct {
	mu        sync.Mutex
	ready     map[string]bool
	prepared  []string
	browsers  []string // the browser of each prepared run
	cancelled []string
	release   chan string
	current   map[string]string // app → browser of its run in progress
}

func (f *fakeDRM) Ready(id, browser string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ready[id+"@"+browser]
}

func (f *fakeDRM) Prepare(ctx context.Context, id, browser string) (bool, error) {
	f.mu.Lock()
	f.prepared = append(f.prepared, id)
	f.browsers = append(f.browsers, browser)
	if f.current == nil {
		f.current = map[string]string{}
	}
	f.current[id] = browser
	f.mu.Unlock()
	select {
	case <-ctx.Done():
		f.mu.Lock()
		f.cancelled = append(f.cancelled, id)
		f.mu.Unlock()
		return false, ctx.Err()
	case got := <-f.release:
		f.mu.Lock()
		f.ready[got+"@"+f.current[got]] = true
		f.mu.Unlock()
		return true, nil
	}
}

func (f *fakeDRM) seen() (prepared, cancelled []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prepared...), append([]string(nil), f.cancelled...)
}

func drm(h *harness, id string) string {
	in := appState(h.c.buildState(viewShell), id).Install
	if in == nil {
		return "<no install>"
	}
	return in.DRM
}

func TestChromiumInstallPreparesTheEnabledStreamingSites(t *testing.T) {
	fd := &fakeDRM{ready: map[string]bool{}, release: make(chan string)}
	h, fi, sl := installHarness(t, func(o *Options) {
		o.DRM = fd
		o.Web = newFakeWeb(o.Desktop.(*fake.Desktop))
	})
	if r := enable(h, "netflix", true); !r.OK {
		t.Fatal(r.Error)
	}
	if got := drm(h, "netflix"); got != "" {
		t.Fatalf("drm before Chromium is installed: %q", got)
	}
	// Chromium's install finishes.
	sl.setScope(adapters.ChromiumFlatpakID, "user")
	fi.set(adapters.ChromiumFlatpakID, install.Status{State: contract.InstallDone, Progress: 100})
	h.eventually("netflix preparing", func() bool { return drm(h, "netflix") == contract.DRMPreparing })
	if got := drm(h, "hulu"); got != contract.DRMPending {
		t.Fatalf("hulu (off) drm %q", got)
	}
	if got := drm(h, "browser"); got != "" {
		t.Fatalf("the Browser needs no Widevine, got drm %q", got)
	}
	fd.release <- "netflix"
	h.eventually("netflix ready", func() bool { return drm(h, "netflix") == contract.DRMReady })

	// Turning Hulu on prepares it; opening it stops the quiet run.
	if r := enable(h, "hulu", true); !r.OK {
		t.Fatal(r.Error)
	}
	h.eventually("hulu preparing", func() bool { return drm(h, "hulu") == contract.DRMPreparing })
	res := h.submit(h.ctl, h.req(contract.ActionAppLaunch, map[string]any{"app_id": "hulu"}))
	if res.Outcome == contract.OutcomeFailed {
		t.Fatalf("launch %+v", res)
	}
	h.eventually("hulu's quiet run stopped", func() bool { _, c := fd.seen(); return len(c) == 1 })
	h.eventually("hulu pending", func() bool { return drm(h, "hulu") == contract.DRMPending })

	prepared, cancelled := fd.seen()
	if fmt.Sprint(prepared) != "[netflix hulu]" || fmt.Sprint(cancelled) != "[hulu]" {
		t.Fatalf("prepared %v cancelled %v", prepared, cancelled)
	}
}
