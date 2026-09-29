// Tests for GET /api/v1/apps/{adapter}/icon (routes.go handleAppIcon;
// contracts/http.md#app-icons) through the real server: authentication,
// guests allowed, unknown and malformed ids refused before the source sees
// them, only PNG within the size cap ever leaves, and the headers.
package remote_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/remote/testutil"
)

var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR-DEMO")

type iconSource struct {
	mu    sync.Mutex
	asked []string
	icons map[string][]byte
}

func (s *iconSource) AppIcon(_ context.Context, adapter string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = append(s.asked, adapter)
	if adapter == "not-an-app" {
		return nil, remote.ErrUnknownApp
	}
	b, ok := s.icons[adapter]
	if !ok {
		return nil, remote.ErrNoIcon
	}
	return b, nil
}

func (s *iconSource) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.asked...)
}

func TestAppIconRoute(t *testing.T) {
	src := &iconSource{icons: map[string][]byte{
		"plex-htpc": tinyPNG,
		"spotify":   []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), // a source bug must not leak SVG
		"jellyfin":  append(append([]byte{}, tinyPNG...), make([]byte, 1<<20)...),
	}}
	backend, err := testutil.NewFakeBackend()
	if err != nil {
		t.Fatal(err)
	}
	devices := testutil.NewFakeDevices(testutil.NewFakeClock())
	ts := httptest.NewUnstartedServer(nil)
	srv, err := remote.New(remote.Options{Backend: backend, Devices: devices, Transport: remote.TransportTrustedLANHTTP,
		AllowedHosts: []string{ts.Listener.Addr().String()}, AppIcons: src})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ts.Config.Handler = srv
	ts.Start()
	defer ts.Close()

	_, family, _ := devices.Pair("Family", contract.PermController)
	_, guest, _ := devices.Pair("Visitor", contract.PermGuest)
	get := func(path, session string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		if session != "" {
			req.AddCookie(&http.Cookie{Name: "bdtv_session", Value: session})
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// Unauthenticated: 401, the source is never asked.
	if resp := get("/api/v1/apps/plex-htpc/icon", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no cookie: %d", resp.StatusCode)
	}
	if len(src.calls()) != 0 {
		t.Fatalf("source asked without a session: %v", src.calls())
	}
	// A family phone and a guest pass both get the PNG, with the headers.
	for name, session := range map[string]string{"family": family, "guest": guest} {
		resp := get("/api/v1/apps/plex-htpc/icon?icons=app", session)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || string(body) != string(tinyPNG) {
			t.Fatalf("%s: %d %q", name, resp.StatusCode, body)
		}
		h := resp.Header
		if h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" ||
			h.Get("Content-Security-Policy") != "default-src 'none'" || h.Get("Cache-Control") != "private, max-age=300" {
			t.Fatalf("%s: headers %v", name, h)
		}
	}
	// Refusals: 404 with a code, never an image.
	for _, tc := range []struct{ path, code string }{
		{"/api/v1/apps/not-an-app/icon", "unknown_app"},
		{"/api/v1/apps/Plex/icon", "unknown_app"},
		{"/api/v1/apps/..%2F..%2Fetc/icon", "not_found"}, // decoded to more segments: no route
		{"/api/v1/apps/-x/icon", "unknown_app"},
		{"/api/v1/apps/moonlight/icon", "no_icon"}, // Bear Den's icon applies
		{"/api/v1/apps/spotify/icon", "no_icon"},   // not PNG: never served
		{"/api/v1/apps/jellyfin/icon", "no_icon"},  // over 1 MiB
	} {
		resp := get(tc.path, family)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(body), `"`+tc.code+`"`) || strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
			t.Fatalf("%s: %d %s (%d bytes) %.80q", tc.path, resp.StatusCode, resp.Header.Get("Content-Type"), len(body), body)
		}
	}
	for _, a := range src.calls() {
		if a != "plex-htpc" && a != "not-an-app" && a != "moonlight" && a != "spotify" && a != "jellyfin" {
			t.Fatalf("a malformed id reached the source: %q", a)
		}
	}
	// Only GET.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/apps/plex-htpc/icon", nil)
	req.AddCookie(&http.Cookie{Name: "bdtv_session", Value: family})
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %v %v", resp.StatusCode, err)
	}
}
