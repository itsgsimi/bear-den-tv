// Tests for the install that turns its app on (install.go startInstall's
// enable, enableAfterInstall): several apps share one Flatpak (the
// streaming sites share Google Chrome), so Install pressed on one
// streaming site's own card turns that site on, and only that one, once
// the install is done and before its tile lights up; a shared card (Add
// apps, IPC without "enable") and the phone's app.install turn nothing on;
// and the site then opens in its own browser, profile and page, never the
// Browser tile.

package session

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/install"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/shellipc"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func appEnabled(h *harness, id string) bool {
	a, _ := h.c.opts.Config.Current().Application(id)
	return a.IsEnabled()
}

func TestInstallFromASitesCardTurnsOnlyThatSiteOn(t *testing.T) {
	var fw *fakeWeb
	logs := &syncBuffer{}
	h, fi, sl := installHarness(t, func(o *Options) {
		fw = newFakeWeb(o.Desktop.(*fake.Desktop))
		o.Web = fw
		o.Logger = slog.New(slog.NewTextHandler(logs, nil))
	})
	// Netflix's own card: Install with "enable".
	if r := h.shellSend(shellipc.AppInstall{Type: shellipc.TypeAppInstall, RequestID: "i1", AppID: "netflix", Enable: true}, "i1"); !r.OK {
		t.Fatalf("app.install refused: %+v", r)
	}
	if starts, _, _ := fi.calls(); fmt.Sprint(starts) != "["+adapters.ChromeFlatpakID+"]" {
		t.Fatalf("installs %v", starts)
	}
	if appEnabled(h, "netflix") {
		t.Fatal("turned on before the install finished")
	}
	// Every snapshot that shows Netflix installed must show it on: the
	// shell opens it on the first one.
	var sawOff bool
	var mu sync.Mutex
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
			st := h.c.buildState(viewShell)
			a := appState(st, "netflix")
			if a.Installed && (a.Enabled == nil || !*a.Enabled) {
				mu.Lock()
				sawOff = true
				mu.Unlock()
			}
		}
	}()
	sl.setScope(adapters.ChromeFlatpakID, "user")
	fi.set(adapters.ChromeFlatpakID, install.Status{State: contract.InstallDone, Progress: 100})
	h.eventually("netflix installed and on", func() bool {
		a := appState(h.c.buildState(viewShell), "netflix")
		return a.Installed && appEnabled(h, "netflix")
	})
	close(stop)
	<-done
	mu.Lock()
	if sawOff {
		t.Error("a snapshot showed Netflix installed but still off: the shell's open would be refused")
	}
	mu.Unlock()
	for _, id := range []string{"disney-plus", "hulu"} {
		if appEnabled(h, id) {
			t.Errorf("%s was turned on too", id)
		}
	}
	if !strings.Contains(logs.String(), "turned on after its install") || !strings.Contains(logs.String(), "app=netflix") {
		t.Errorf("not logged: %s", logs)
	}
	// The shell opens it: Netflix, in Chrome, never the Browser tile.
	res := h.submit(h.ctl, h.req(contract.ActionAppLaunch, map[string]any{"app_id": "netflix"}))
	if res.Outcome == contract.OutcomeFailed {
		t.Fatalf("launch %+v", res)
	}
	h.eventually("netflix in front", func() bool { return h.c.Target().AppID != nil && *h.c.Target().AppID == "netflix" })
	fw.mu.Lock()
	launched := fmt.Sprint(fw.launched)
	fw.mu.Unlock()
	if launched != "[netflix "+adapters.ChromeFlatpakID+"]" {
		t.Fatalf("launched %s", launched)
	}
}

func TestSharedInstallCardTurnsNothingOn(t *testing.T) {
	h, fi, sl := installHarness(t, func(o *Options) { o.Web = newFakeWeb(o.Desktop.(*fake.Desktop)) })
	// Add apps' shared browser card (no "enable"), then the phone.
	if r := h.shellSend(shellipc.AppInstall{Type: shellipc.TypeAppInstall, RequestID: "i1", AppID: "netflix"}, "i1"); !r.OK {
		t.Fatalf("app.install refused: %+v", r)
	}
	sl.setScope(adapters.ChromeFlatpakID, "user")
	fi.set(adapters.ChromeFlatpakID, install.Status{State: contract.InstallDone, Progress: 100})
	h.eventually("chrome discovered", func() bool { return appState(h.c.buildState(viewShell), "netflix").Installed })
	for _, id := range []string{"netflix", "disney-plus", "hulu"} {
		if appEnabled(h, id) {
			t.Errorf("%s turned on by a shared card", id)
		}
	}
	// The phone's owner-only app.install carries no such intent.
	expectOutcome(t, h.submit(h.owner, h.req(contract.ActionAppInstall, map[string]any{"app_id": "browser"})), contract.OutcomeDelivered, contract.CodeOK)
	sl.setScope(adapters.BraveFlatpakID, "user")
	fi.set(adapters.BraveFlatpakID, install.Status{State: contract.InstallDone, Progress: 100})
	h.eventually("brave discovered", func() bool { return appState(h.c.buildState(viewShell), "browser").Installed })
	if appEnabled(h, "netflix") {
		t.Error("netflix turned on by another install")
	}
}
