// Tests for the Browser tile's start page, the coordinator's side
// (startpage.go): the cards are the streaming sites that are on and
// installed; a card opens that site as its own app, in its own browser,
// only when pressed on the Browser tile's page, never a site that is off.

package session

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/applications/install"
	"bear-den-tv/internal/applications/web"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/platform/fake"
	"bear-den-tv/internal/shellipc"
)

// startWeb is fakeWeb with the start page hooks, as web.Manager has them.
type startWeb struct {
	*fakeWeb
	mu     sync.Mutex
	cards  func() []web.StartCard
	onOpen func(from, appID string)
}

func (s *startWeb) SetStartCards(fn func() []web.StartCard) { s.mu.Lock(); s.cards = fn; s.mu.Unlock() }
func (s *startWeb) OnOpen(fn func(from, appID string))      { s.mu.Lock(); s.onOpen = fn; s.mu.Unlock() }

func TestStartPageCardsAndOpens(t *testing.T) {
	var sw *startWeb
	h, fi, sl := installHarness(t, func(o *Options) {
		sw = &startWeb{fakeWeb: newFakeWeb(o.Desktop.(*fake.Desktop))}
		o.Web = sw
	})
	sw.mu.Lock()
	cards, open := sw.cards, sw.onOpen
	sw.mu.Unlock()
	if cards == nil || open == nil {
		t.Fatal("the start page hooks were not wired")
	}
	if got := cards(); len(got) != 0 {
		t.Fatalf("cards before anything is installed: %v", got)
	}
	// Chrome installed, Netflix and Hulu on, Disney+ off.
	if r := h.shellSend(shellipc.AppInstall{Type: shellipc.TypeAppInstall, RequestID: "i-netflix", AppID: "netflix"}, "i-netflix"); !r.OK {
		t.Fatal(r.Error)
	}
	sl.setScope(adapters.ChromeFlatpakID, "user")
	fi.set(adapters.ChromeFlatpakID, install.Status{State: contract.InstallDone, Progress: 100})
	h.eventually("chrome discovered", func() bool { return appState(h.c.buildState(viewShell), "netflix").Installed })
	for _, id := range []string{"netflix", "hulu"} {
		if r := enable(h, id, true); !r.OK {
			t.Fatal(r.Error)
		}
	}
	if got := fmt.Sprint(cards()); got != "[{netflix Netflix} {hulu Hulu}]" {
		t.Fatalf("cards %s", got)
	}
	launched := func() string {
		sw.fakeWeb.mu.Lock()
		defer sw.fakeWeb.mu.Unlock()
		return strings.Join(sw.fakeWeb.launched, ",")
	}
	// Not from a streaming site's page, not a site that is off, not the
	// Browser itself.
	open("netflix", "hulu")
	open("browser", "disney-plus")
	open("browser", "browser")
	open("browser", "plex-htpc")
	if got := launched(); got != "" {
		t.Fatalf("launched %q", got)
	}
	// From the Browser tile's start page: Netflix, in Chrome.
	open("browser", "netflix")
	h.eventually("netflix in front", func() bool { return h.c.Target().AppID != nil && *h.c.Target().AppID == "netflix" })
	if got := launched(); got != "netflix "+adapters.ChromeFlatpakID {
		t.Fatalf("launched %q", got)
	}
}
