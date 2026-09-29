// Tests for the Plex Media Server client (client.go) against the local fake:
// libraries, Continue Watching with its onDeck fallback, artwork confined to
// the configured host, and the token kept to the request header.

package plex

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bear-den-tv/internal/providers/plex/plexfake"
)

func newClient(t *testing.T, serverURL, token string) *Client {
	t.Helper()
	c, err := NewClient(ClientOptions{ServerURL: serverURL, ClientIdentifier: testClientID, Retries: -1})
	if err != nil {
		t.Fatal(err)
	}
	c.SetToken(token)
	return c
}

func containsToken(s, token string) bool { return token != "" && strings.Contains(s, token) }

func TestLibrariesListsEverySection(t *testing.T) {
	f := newFake(t, plexfake.Options{})
	libs, err := newClient(t, f.URL, f.Token()).Libraries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 3 || string(libs[0].Key) != "1" || libs[0].Type != "movie" || libs[2].Type != "artist" {
		t.Fatalf("libraries = %+v", libs)
	}
}

func TestContinueWatchingUsesTheHub(t *testing.T) {
	f := newFake(t, plexfake.Options{})
	items, err := newClient(t, f.URL, f.Token()).ContinueWatching(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("items = %d, want the 4 in-progress DEMO items", len(items))
	}
	if f.Count("/library/onDeck") != 0 {
		t.Fatal("fell back to onDeck although the hub answered")
	}
}

func TestContinueWatchingFallsBackToOnDeckOn404(t *testing.T) {
	f := newFake(t, plexfake.Options{NoContinueWatchingHub: true})
	items, err := newClient(t, f.URL, f.Token()).ContinueWatching(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || f.Count("/library/onDeck") != 1 {
		t.Fatalf("items = %d, onDeck calls = %d; want 4 items from one onDeck call", len(items), f.Count("/library/onDeck"))
	}
}

func TestContinueWatchingDoesNotFallBackOnOtherErrors(t *testing.T) {
	f := newFake(t, plexfake.Options{})
	f.SetDown(true)
	_, err := newClient(t, f.URL, f.Token()).ContinueWatching(context.Background())
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusServiceUnavailable {
		t.Fatalf("err = %v, want 503", err)
	}
	if f.Count("/library/onDeck") != 0 {
		t.Fatal("a 503 must not trigger the onDeck fallback")
	}
}

func TestTokenTravelsOnlyInTheHeader(t *testing.T) {
	f := newFake(t, plexfake.Options{})
	c := newClient(t, f.URL, f.Token())
	ctx := context.Background()
	if _, err := c.Libraries(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RecentlyAdded(ctx, "1", 0, 5); err != nil {
		t.Fatal(err)
	}
	_, body, err := c.Photo(ctx, "/library/metadata/101/thumb/1", 120, 180)
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	for _, r := range f.Requests() {
		if r.TokenInQuery || r.HeaderToken != f.Token() {
			t.Fatalf("request %s: token in query=%v header=%q", r.Path, r.TokenInQuery, r.HeaderToken)
		}
	}
}

func TestPhotoRefusesRedirectToAnotherHost(t *testing.T) {
	var otherHits int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(plexfake.Poster("1", 10, 15))
	}))
	defer other.Close()
	f := newFake(t, plexfake.Options{})
	f.SetPhotoHandler(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
	})
	_, _, err := newClient(t, f.URL, f.Token()).Photo(context.Background(), "/library/metadata/101/thumb/1", 120, 180)
	if !errors.Is(err, ErrRedirectRefused) {
		t.Fatalf("err = %v, want ErrRedirectRefused", err)
	}
	if otherHits != 0 {
		t.Fatalf("the other host received %d requests", otherHits)
	}
	if containsToken(err.Error(), f.Token()) {
		t.Fatalf("error carries the token: %v", err)
	}
}

func TestPhotoFollowsSameHostRedirects(t *testing.T) {
	f := newFake(t, plexfake.Options{})
	f.SetPhotoHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("moved") == "" {
			http.Redirect(w, r, r.URL.Path+"?moved=1", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(plexfake.Poster("1", 10, 15))
	})
	ct, body, err := newClient(t, f.URL, f.Token()).Photo(context.Background(), "/library/metadata/101/thumb/1", 120, 180)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if ct != "image/png" {
		t.Fatalf("content type %q", ct)
	}
}

func TestPhotoRejectsUnsafeThumbsAndNonImages(t *testing.T) {
	f := newFake(t, plexfake.Options{})
	c := newClient(t, f.URL, f.Token())
	for _, thumb := range []string{"http://elsewhere.example/x.png", "//elsewhere.example/x", "relative/x", "/ok\n/x"} {
		if _, _, err := c.Photo(context.Background(), thumb, 10, 10); !errors.Is(err, ErrInvalidArtworkPath) {
			t.Fatalf("thumb %q: err = %v, want ErrInvalidArtworkPath", thumb, err)
		}
	}
	f.SetPhotoHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>")
	})
	if _, _, err := c.Photo(context.Background(), "/library/metadata/1/thumb", 10, 10); !errors.Is(err, ErrNotImage) {
		t.Fatalf("err = %v, want ErrNotImage", err)
	}
}

func TestParseServerURLRejectsCredentialsAndQueries(t *testing.T) {
	for _, bad := range []string{"ftp://h", "http://", "http://user:pw@h:32400", "http://h:32400/?X-Plex-Token=x", "http://h#frag"} {
		if _, err := ParseServerURL(bad); !errors.Is(err, ErrInvalidServerURL) {
			t.Fatalf("%q: err = %v, want ErrInvalidServerURL", bad, err)
		}
	}
	if u, err := ParseServerURL("https://h.example:32400/"); err != nil || u.String() != "https://h.example:32400" {
		t.Fatalf("u = %v err = %v", u, err)
	}
}
