// End-to-end guest pass expiry through the real LAN server: a phone redeems
// a guest code over HTTP, opens the events WebSocket, and when the pass ends
// on the fake clock it receives "revoked", the socket closes with 4001 and
// its cookie stops working (guest.go, internal/remote/ws.go).

package pairing_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/pairing"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/remote/testutil"
	"bear-den-tv/internal/storage"
)

func TestGuestPassEndClosesSocketWith4001(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	clk := clock.NewFake(time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC))
	svc, err := pairing.New(pairing.Options{DB: db, Clock: clk, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	backend, err := testutil.NewFakeBackend()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(nil)
	host := ts.Listener.Addr().String()
	srv, err := remote.New(remote.Options{Backend: backend, Devices: svc, Transport: remote.TransportTrustedLANHTTP, AllowedHosts: []string{host}})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ts.Config.Handler = srv
	ts.Start()
	defer ts.Close()
	origin := "http://" + host

	iss, err := svc.IssuePass(ctx, contract.Pass24h)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"invitation":null,"code":"` + iss.Code + `","device_name":"Visitor"}`
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/v1/pair/claim", strings.NewReader(body))
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var claim struct {
		Permissions []string `json:"permissions"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&claim)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(claim.Permissions) != 1 || claim.Permissions[0] != "guest" {
		t.Fatalf("claim: %d %v", resp.StatusCode, claim.Permissions)
	}
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "bdtv_session" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie")
	}

	hdr := http.Header{}
	hdr.Set("Origin", origin)
	hdr.Set("Cookie", cookie.Name+"="+cookie.Value)
	conn, _, err := websocket.Dial(ctx, "ws://"+host+"/api/v1/events", &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	// The socket is registered once the first state arrives.
	if _, _, err := conn.Read(ctx); err != nil {
		t.Fatal(err)
	}

	clk.Advance(24 * time.Hour) // the pass ends; its timer revokes the device

	sawRevoked := false
	for {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			if code := websocket.CloseStatus(err); code != 4001 {
				t.Fatalf("socket closed with %v (%v), want 4001", code, err)
			}
			break
		}
		var m struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(msg, &m)
		if m.Type == "revoked" {
			sawRevoked = true
		}
	}
	if !sawRevoked {
		t.Fatal(`no "revoked" message before the close`)
	}

	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/session", nil)
	req.AddCookie(cookie)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session after the pass ended: %d, want 401", resp.StatusCode)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("timed out")
	}
}
