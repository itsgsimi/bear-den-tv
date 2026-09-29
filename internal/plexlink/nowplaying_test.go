// Tests for NowPlaying (nowplaying.go): the signed-in manager asks the
// chosen server (plexfake) for its sessions and returns this TV's Plex HTPC
// playback; nothing is asked before sign-in; a refusal is an error.

package plexlink

import (
	"context"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"bear-den-tv/internal/providers/plex/plexfake"
)

func TestNowPlayingReadsThisTVsPlexHTPCFromTheServer(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	r.m.opts.LocalAddrs = func() ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("192.0.2.10")}, nil
	}
	ctx := context.Background()

	// Not signed in: nothing is asked.
	p, err := r.m.NowPlaying(ctx)
	if err != nil || p.Status != PlayingOff || r.fake.Count("/status/sessions") != 0 {
		t.Fatalf("before sign-in: %+v %v (%d requests)", p, err, r.fake.Count("/status/sessions"))
	}

	r.run()
	r.signIn("1", "2")
	r.eventually("rows", func() bool { return r.m.Content() != nil })
	p, err = r.m.NowPlaying(ctx)
	if err != nil || p.Status != PlayingIdle {
		t.Fatalf("nothing playing: %+v %v", p, err)
	}

	r.fake.SetSessions([]plexfake.Session{
		{Kind: "movie", Title: "DEMO Film", OffsetMs: 5, DurationMs: 10, Player: plexfake.SessionPlayer{Address: "192.0.2.50", Product: "Plex Web", State: "playing"}},
		{Kind: "episode", Title: "DEMO Episode 3", Show: "DEMO Show", Season: "Season 1", OffsetMs: 754_000, DurationMs: 2_640_000,
			Player: plexfake.SessionPlayer{Address: "192.0.2.10", Product: "Plex HTPC", State: "paused", Local: true}},
	})
	p, err = r.m.NowPlaying(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := Playing{Status: PlayingFound, Title: "DEMO Episode 3", Subtitle: "DEMO Show", State: "paused", Offset: 754 * time.Second, Duration: 2640 * time.Second}
	if p != want {
		t.Fatalf("got %#v, want %#v", p, want)
	}

	r.fake.SetSessions([]plexfake.Session{{Kind: "movie", Title: "DEMO Film", Player: plexfake.SessionPlayer{Address: "198.51.100.4", Product: "Plex HTPC", State: "playing"}}})
	if p, err = r.m.NowPlaying(ctx); err != nil || p.Status != PlayingUnidentified || p.Reason == "" || p.Title != "" {
		t.Fatalf("another address: %+v %v", p, err)
	}

	r.fake.SetSessionsStatus(http.StatusForbidden)
	if _, err = r.m.NowPlaying(ctx); err == nil {
		t.Fatal("a refused sessions list is not an error")
	}
}

func TestLocalAddrsIncludesLoopback(t *testing.T) {
	addrs, err := LocalAddrs()
	if err != nil {
		t.Skipf("no interfaces here: %v", err)
	}
	for _, a := range addrs {
		if a.IsLoopback() {
			return
		}
	}
	t.Fatalf("no loopback among %v", addrs)
}
