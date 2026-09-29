// Tests for plex.tv sign-in and server discovery (account.go) against the
// local fake in plexfake; nothing here reaches the real plex.tv.

package plex

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bear-den-tv/internal/providers/plex/plexfake"
)

const testClientID = "0123456789abcdef0123456789abcdef"

func newFake(t *testing.T, opts plexfake.Options) *plexfake.Fake {
	t.Helper()
	f, err := plexfake.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
}

func newAccount(t *testing.T, base string, now func() time.Time) *Account {
	t.Helper()
	a, err := NewAccount(AccountOptions{BaseURL: base, ClientIdentifier: testClientID, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestPINFlowLinksAndReturnsToken(t *testing.T) {
	f := newFake(t, plexfake.Options{})
	a := newAccount(t, f.URL, nil)
	ctx := context.Background()
	pin, err := a.StartPIN(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pin.Code) != 4 || pin.ID == 0 || pin.ExpiresAt.IsZero() {
		t.Fatalf("pin = %+v, want a 4-character code, an id and an expiry", pin)
	}
	token, done, err := a.PollPIN(ctx, pin.ID)
	if err != nil || done || token != "" {
		t.Fatalf("before linking: token=%q done=%v err=%v", token, done, err)
	}
	f.Link()
	token, done, err = a.PollPIN(ctx, pin.ID)
	if err != nil || !done || token != f.Token() {
		t.Fatalf("after linking: token=%q done=%v err=%v", token, done, err)
	}
	for _, r := range f.Requests() {
		if r.TokenInQuery {
			t.Fatalf("token sent in a query string: %+v", r)
		}
	}
}

func TestPINExpiresByTheClock(t *testing.T) {
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	now := t0
	f := newFake(t, plexfake.Options{PINTTL: time.Minute, Now: func() time.Time { return t0 }})
	a := newAccount(t, f.URL, func() time.Time { return now })
	ctx := context.Background()
	pin, err := a.StartPIN(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now = t0.Add(59 * time.Second)
	if _, _, err := a.PollPIN(ctx, pin.ID); err != nil {
		t.Fatalf("still valid at 59 s: %v", err)
	}
	now = t0.Add(61 * time.Second)
	if _, _, err := a.PollPIN(ctx, pin.ID); !errors.Is(err, ErrPINExpired) {
		t.Fatalf("at 61 s err = %v, want ErrPINExpired", err)
	}
}

func TestPINGoneIsExpired(t *testing.T) {
	f := newFake(t, plexfake.Options{})
	a := newAccount(t, f.URL, nil)
	pin, err := a.StartPIN(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.ForgetPINs()
	if _, _, err := a.PollPIN(context.Background(), pin.ID); !errors.Is(err, ErrPINExpired) {
		t.Fatalf("err = %v, want ErrPINExpired for a 404", err)
	}
}

func TestDiscoverServersOrdersConnectionsAndSkipsRelays(t *testing.T) {
	f := newFake(t, plexfake.Options{})
	f.SetResources([]plexfake.Resource{
		{Name: "Player", ClientIdentifier: "p1", Provides: "player,controller"},
		{Name: "Server A", ClientIdentifier: "a1", Provides: "server", Owned: true, Connections: []plexfake.Connection{
			{Protocol: "https", URI: "https://remote-a.example:32400", Local: false},
			{Protocol: "http", URI: "http://192.0.2.10:32400", Local: true},
			{Protocol: "https", URI: "https://relay-a.example:443", Local: false, Relay: true},
			{Protocol: "https", URI: "https://local-a.example:32400", Local: true},
			{Protocol: "http", URI: "ftp://bad.example", Local: true},
			{Protocol: "http", URI: "http://remote-a.example:32400", Local: false},
		}},
		{Name: "Server B", ClientIdentifier: "b1", Provides: "client, server", Connections: []plexfake.Connection{
			{Protocol: "http", URI: "http://192.0.2.20:32400", Local: true},
		}},
	})
	a := newAccount(t, f.URL, nil)
	servers, err := a.DiscoverServers(context.Background(), f.Token())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || servers[0].Name != "Server A" || servers[1].Name != "Server B" {
		t.Fatalf("servers = %+v, want Server A and Server B only", servers)
	}
	want := []string{"https://local-a.example:32400", "http://192.0.2.10:32400", "https://remote-a.example:32400", "http://remote-a.example:32400"}
	got := servers[0].Connections
	if len(got) != len(want) {
		t.Fatalf("connections = %+v, want %v", got, want)
	}
	for i := range want {
		if got[i].URI != want[i] || got[i].Relay {
			t.Fatalf("connection %d = %+v, want %s (order: local https, local http, remote https, remote http; no relay)", i, got[i], want[i])
		}
	}
	if !servers[0].Owned || servers[0].MachineIdentifier != "a1" {
		t.Fatalf("server A = %+v", servers[0])
	}
}

func TestDiscoverServersNeedsAValidToken(t *testing.T) {
	f := newFake(t, plexfake.Options{})
	a := newAccount(t, f.URL, nil)
	if _, err := a.DiscoverServers(context.Background(), ""); err == nil {
		t.Fatal("discovery without a token succeeded")
	}
	_, err := a.DiscoverServers(context.Background(), "wrong-token-value")
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v, want a 401 StatusError", err)
	}
	if containsToken(err.Error(), "wrong-token-value") {
		t.Fatalf("error text carries the token: %v", err)
	}
}

func TestAccountRefusesRedirectToAnotherHost(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the other host was contacted: %s", r.URL.Path)
	}))
	defer other.Close()
	plexTV := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/v2/pins", http.StatusFound)
	}))
	defer plexTV.Close()
	a := newAccount(t, plexTV.URL, nil)
	if _, err := a.StartPIN(context.Background()); !errors.Is(err, ErrRedirectRefused) {
		t.Fatalf("err = %v, want ErrRedirectRefused", err)
	}
}
