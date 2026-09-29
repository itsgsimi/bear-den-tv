// Tests for /status/sessions and picking this TV's Plex HTPC (sessions.go)
// against plexfake: the fields decode, the token travels only in the
// header, a shared account's refusal is an error, and a session counts as
// this TV's only when it is Plex HTPC from one of this machine's addresses.

package plex

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"bear-den-tv/internal/providers/plex/plexfake"
)

// thisTV is the machine's own addresses in these tests.
func thisTV(a netip.Addr) bool {
	return a == netip.MustParseAddr("192.0.2.10") || a.IsLoopback()
}

func htpc(addr, state string) plexfake.Session {
	return plexfake.Session{Kind: "episode", Title: "DEMO Episode 3: The Long Winter", Show: "DEMO Show", Season: "Season 1",
		OffsetMs: 754_000, DurationMs: 2_640_000,
		Player: plexfake.SessionPlayer{Address: addr, MachineIdentifier: "demo-htpc-1", Product: "Plex HTPC", Platform: "Linux", Title: "DEMO TV", State: state, Local: true}}
}

func TestSessionsDecodeAndIdentifyThisTV(t *testing.T) {
	r := newProvider(t, plexfake.Options{}, nil)
	other := plexfake.Session{Kind: "movie", Title: "DEMO Film", OffsetMs: 1, DurationMs: 2,
		Player: plexfake.SessionPlayer{Address: "192.0.2.77", Product: "Plex Web", State: "playing"}}
	r.fake.SetSessions([]plexfake.Session{other, htpc("192.0.2.10", "paused")})
	ss, err := r.p.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 2 {
		t.Fatalf("sessions %v", ss)
	}
	m := ThisPlayer(ss, thisTV)
	s := m.Session
	if m.Status != PlayerFound || s.Title != "DEMO Episode 3: The Long Winter" || s.GrandparentTitle != "DEMO Show" || s.Type != "episode" ||
		s.ViewOffset != 754_000 || s.Duration != 2_640_000 || s.Player.State != "paused" || s.Player.MachineIdentifier != "demo-htpc-1" {
		t.Fatalf("match %+v (%s)", m, s)
	}
	if strings.Contains(s.String(), "DEMO Episode") {
		t.Fatal("Session.String carries the title")
	}
	for _, req := range r.fake.Requests() {
		if req.Path == "/status/sessions" && (req.TokenInQuery || req.HeaderToken != r.fake.Token()) {
			t.Fatalf("token not (only) in the header: %+v", req)
		}
	}
}

func TestThisPlayerNeverGuesses(t *testing.T) {
	sess := func(ps ...plexfake.Session) []Session {
		out := make([]Session, 0, len(ps))
		for _, p := range ps {
			out = append(out, Session{Type: p.Kind, Title: p.Title, Player: Player{Address: p.Player.Address, Product: p.Player.Product, State: p.Player.State}})
		}
		return out
	}
	for _, tc := range []struct {
		name string
		in   []Session
		want string
	}{
		{"nothing", nil, PlayerIdle},
		{"only other apps", sess(plexfake.Session{Player: plexfake.SessionPlayer{Address: "192.0.2.10", Product: "Plexamp", State: "playing"}}), PlayerIdle},
		{"ours", sess(htpc("192.0.2.10", "playing")), PlayerFound},
		{"ours on the server's own machine", sess(htpc("127.0.0.1", "playing")), PlayerFound},
		{"ours, IPv4-mapped", sess(htpc("::ffff:192.0.2.10", "buffering")), PlayerFound},
		{"another TV's", sess(htpc("192.0.2.99", "playing")), PlayerUnidentified},
		{"through NAT", sess(htpc("198.51.100.4", "playing")), PlayerUnidentified},
		{"two from here", sess(htpc("192.0.2.10", "playing"), htpc("127.0.0.1", "paused")), PlayerUnidentified},
		{"no address", sess(htpc("", "playing")), PlayerUnidentified},
		{"ours and another TV's", sess(htpc("192.0.2.99", "playing"), htpc("192.0.2.10", "paused")), PlayerFound},
	} {
		m := ThisPlayer(tc.in, thisTV)
		if m.Status != tc.want || (m.Status == PlayerUnidentified) != (m.Reason != "") {
			t.Errorf("%s: %+v, want %s", tc.name, m, tc.want)
		}
	}
}

func TestSessionsRefusedForASharedAccount(t *testing.T) {
	r := newProvider(t, plexfake.Options{}, nil)
	r.fake.SetSessionsStatus(http.StatusForbidden)
	_, err := r.p.Sessions(context.Background())
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusForbidden {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(err.Error(), r.fake.Token()) {
		t.Fatal("the token is in the error")
	}
}
