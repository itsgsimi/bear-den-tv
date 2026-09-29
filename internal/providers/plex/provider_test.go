// Tests for the Plex ContentProvider (provider.go, artwork.go, redact.go)
// against the local fake, an in-memory secret store and a temporary artwork
// cache.

package plex

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/providers"
	"bear-den-tv/internal/providers/plex/plexfake"
	"bear-den-tv/internal/secrets"
)

// logBuffer is a concurrency-safe log sink.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type providerRig struct {
	fake    *plexfake.Fake
	store   *secrets.Memory
	cache   *ArtworkCache
	logs    *logBuffer
	p       *Provider
	artRoot string
}

func newProvider(t *testing.T, fopts plexfake.Options, libraryIDs []string) *providerRig {
	t.Helper()
	f := newFake(t, fopts)
	store := secrets.NewMemory()
	if err := store.Set(context.Background(), "plex", f.Token(), "Plex"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cache, err := NewArtworkCache(root, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	logs := &logBuffer{}
	p, err := New(Options{
		ServerURL: f.URL, ConnectionRef: "plex", LibraryIDs: libraryIDs, ClientIdentifier: testClientID,
		Secrets: store, Artwork: cache, Client: ClientOptions{Retries: -1},
		Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &providerRig{fake: f, store: store, cache: cache, logs: logs, p: p, artRoot: root}
}

func TestProviderMapsContinueWatchingWithProgress(t *testing.T) {
	r := newProvider(t, plexfake.Options{}, nil)
	ctx := context.Background()
	if err := r.p.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	items, _, err := r.p.FetchItems(ctx, providers.KindContinueWatching, providers.SectionConfig{ID: "plex-continue"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	byTitle := map[string]float64{}
	for _, it := range items {
		if it.Progress == nil || it.Demo || it.OpenAction != providers.OpenApp || !strings.HasPrefix(it.ID, "plex:") {
			t.Fatalf("item = %+v", it)
		}
		byTitle[it.Title] = *it.Progress
	}
	// An episode shows its show as the title and S·E·title as the subtitle.
	if p, ok := byTitle["DEMO Northern Lights"]; !ok || p < 0.41 || p > 0.43 {
		t.Fatalf("progress by title = %v", byTitle)
	}
	for _, it := range items {
		if it.Title == "DEMO Northern Lights" && it.Subtitle != "S1 · E3 · The Long Night" {
			t.Fatalf("episode subtitle = %q", it.Subtitle)
		}
	}
}

func TestProviderRecentlyAddedDefaultsToVideoLibraries(t *testing.T) {
	r := newProvider(t, plexfake.Options{}, nil)
	ctx := context.Background()
	if err := r.p.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	items, _, err := r.p.FetchItems(ctx, providers.KindRecentlyAdded, providers.SectionConfig{ID: "plex-recent"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 7 {
		t.Fatalf("items = %d, want the 7 movie and show items (music skipped)", len(items))
	}
	if items[0].Title != "DEMO Salt & Stone" {
		t.Fatalf("first = %q, want the newest (DEMO Salt & Stone)", items[0].Title)
	}
	if r.fake.Count("/library/sections/3/recentlyAdded") != 0 {
		t.Fatal("the music library was fetched")
	}
}

func TestProviderHonoursConfiguredLibraries(t *testing.T) {
	r := newProvider(t, plexfake.Options{}, []string{"2"})
	ctx := context.Background()
	if err := r.p.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	items, _, err := r.p.FetchItems(ctx, providers.KindContinueWatching, providers.SectionConfig{ID: "c"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("continue watching = %d items, want the 2 from library 2 only", len(items))
	}
	bad := newProvider(t, plexfake.Options{}, []string{"99"})
	if err := bad.p.Connect(ctx); err == nil {
		t.Fatal("a configured library missing on the server connected")
	}
}

func TestProviderArtworkIsCachedLocally(t *testing.T) {
	r := newProvider(t, plexfake.Options{}, nil)
	ctx := context.Background()
	if err := r.p.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	items, _, err := r.p.FetchItems(ctx, providers.KindContinueWatching, providers.SectionConfig{ID: "c"}, "")
	if err != nil {
		t.Fatal(err)
	}
	path, err := r.p.ResolveArtwork(ctx, items[0])
	if err != nil || path == "" {
		t.Fatalf("path=%q err=%v", path, err)
	}
	if !strings.HasPrefix(path, r.artRoot+string(filepath.Separator)) {
		t.Fatalf("artwork %s is outside the cache %s", path, r.artRoot)
	}
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	if _, err := png.DecodeConfig(fh); err != nil {
		t.Fatalf("cached artwork is not a PNG: %v", err)
	}
	before := r.fake.Count("/photo/:/transcode")
	if again, err := r.p.ResolveArtwork(ctx, items[0]); err != nil || again != path {
		t.Fatalf("second resolve = %q, %v", again, err)
	}
	if r.fake.Count("/photo/:/transcode") != before {
		t.Fatal("a cached poster was downloaded again")
	}
}

func TestProviderConnectFailsClosedWithReasons(t *testing.T) {
	ctx := context.Background()

	r := newProvider(t, plexfake.Options{}, nil)
	_ = r.store.Delete(ctx, "plex")
	if err := r.p.Connect(ctx); err == nil || !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("no token: err = %v", err)
	}
	if st, msg := r.p.Status(); st != providers.StatusError || msg != "Plex account is not linked" {
		t.Fatalf("status = %s %q", st, msg)
	}

	r = newProvider(t, plexfake.Options{}, nil)
	r.store.SetLocked(true)
	if err := r.p.Connect(ctx); !errors.Is(err, secrets.ErrLocked) {
		t.Fatalf("locked keyring: err = %v", err)
	}

	r = newProvider(t, plexfake.Options{}, nil)
	r.fake.SetDown(true)
	if err := r.p.Connect(ctx); err == nil {
		t.Fatal("connected to a server that is down")
	}
	if _, msg := r.p.Status(); msg != MsgUnreachable {
		t.Fatalf("message = %q, want %q", msg, MsgUnreachable)
	}

	r = newProvider(t, plexfake.Options{}, nil)
	_ = r.store.Set(ctx, "plex", "revoked-token-value", "Plex")
	if err := r.p.Connect(ctx); err == nil {
		t.Fatal("a rejected token connected")
	}
	if _, msg := r.p.Status(); msg != MsgTokenRejected {
		t.Fatalf("message = %q, want %q", msg, MsgTokenRejected)
	}
}

func TestProviderFetchErrorsAreFriendlyAndRedacted(t *testing.T) {
	r := newProvider(t, plexfake.Options{}, nil)
	ctx := context.Background()
	if err := r.p.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	r.fake.SetDown(true)
	_, _, err := r.p.FetchItems(ctx, providers.KindContinueWatching, providers.SectionConfig{ID: "c"}, "")
	if err == nil || err.Error() != MsgUnreachable {
		t.Fatalf("err = %v, want %q", err, MsgUnreachable)
	}
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 503 {
		t.Fatalf("the cause is lost: %v", err)
	}
	if !strings.Contains(r.logs.String(), "plex fetch failed") {
		t.Fatalf("the failure was not logged: %s", r.logs.String())
	}
}

func TestTokenNeverReachesLogsOrErrors(t *testing.T) {
	r := newProvider(t, plexfake.Options{}, nil)
	ctx := context.Background()
	var errs []string
	collect := func(err error) {
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	collect(r.p.Connect(ctx))
	for _, kind := range []string{providers.KindContinueWatching, providers.KindRecentlyAdded} {
		items, _, err := r.p.FetchItems(ctx, kind, providers.SectionConfig{ID: kind}, "")
		collect(err)
		for _, it := range items {
			_, err := r.p.ResolveArtwork(ctx, it)
			collect(err)
		}
	}
	r.fake.SetDown(true)
	_, _, err := r.p.FetchItems(ctx, providers.KindRecentlyAdded, providers.SectionConfig{ID: "r"}, "")
	collect(err)
	r.p.Disconnect()
	collect(r.p.Connect(ctx))
	// A hostile log line that quotes the token is redacted by the handler.
	r.p.logger.Warn("echo", "detail", "server said X-Plex-Token="+r.fake.Token(), "raw", r.fake.Token())

	all := r.logs.String() + strings.Join(errs, "\n")
	if strings.Contains(all, r.fake.Token()) {
		t.Fatalf("token leaked:\n%s", all)
	}
	if !strings.Contains(r.logs.String(), "[token]") {
		t.Fatalf("the echoed token was not replaced with [token]:\n%s", r.logs.String())
	}
}

func TestRedactorStripsTokensAndTitles(t *testing.T) {
	red := NewRedactor()
	red.AddSecret("abcd1234secret")
	red.AddTitle("DEMO Private Title")
	in := "GET /x?X-Plex-Token=zzz&a=1 header X-Plex-Token: yyy body abcd1234secret about DEMO Private Title"
	out := red.Redact(in)
	for _, leak := range []string{"zzz", "yyy", "abcd1234secret", "DEMO Private Title"} {
		if strings.Contains(out, leak) {
			t.Fatalf("%q survived redaction: %s", leak, out)
		}
	}
	wrapped := red.Error(errors.Join(ErrRedirectRefused, errors.New("abcd1234secret")))
	if strings.Contains(wrapped.Error(), "abcd1234secret") || !errors.Is(wrapped, ErrRedirectRefused) {
		t.Fatalf("wrapped = %v", wrapped)
	}
}

func TestResolveOpenActionStaysHonest(t *testing.T) {
	r := newProvider(t, plexfake.Options{}, nil)
	a := r.p.ResolveOpenAction(context.Background(), contract.ContentItem{ID: "plex:x:1"})
	if a.Kind != providers.OpenApp || a.AppID != DefaultAppID || !strings.Contains(a.Reason, "PLEX-02") {
		t.Fatalf("open action = %+v; exact-item playback must not be claimed", a)
	}
}

func TestSignOutDeletesTheTokenAndPurgesArtwork(t *testing.T) {
	r := newProvider(t, plexfake.Options{}, nil)
	ctx := context.Background()
	if err := r.p.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	items, _, _ := r.p.FetchItems(ctx, providers.KindContinueWatching, providers.SectionConfig{ID: "c"}, "")
	path, err := r.p.ResolveArtwork(ctx, items[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := r.p.SignOut(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.store.Get(ctx, "plex"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("token still stored: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("artwork survived sign-out: %v", err)
	}
	if _, _, err := r.p.FetchItems(ctx, providers.KindContinueWatching, providers.SectionConfig{ID: "c"}, ""); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("fetch after sign-out: %v", err)
	}
}
