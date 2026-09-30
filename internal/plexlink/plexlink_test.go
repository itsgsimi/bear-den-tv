// Tests for Plex sign-in and the Home rows (plexlink.go) against the local
// plex.tv/server fake, a real config store in a temp dir, an in-memory
// keyring and a fake clock for polling, refresh cadence and backoff.

package plexlink

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/providers"
	"bear-den-tv/internal/providers/plex"
	"bear-den-tv/internal/providers/plex/plexfake"
	"bear-den-tv/internal/secrets"
)

type rig struct {
	t       *testing.T
	clk     *clock.Fake
	fake    *plexfake.Fake
	store   *config.Store
	keyring secrets.Store
	mem     *secrets.Memory
	m       *Manager
	changes atomic.Int64
	cfgDir  string
	artDir  string
}

func newRig(t *testing.T, fopts plexfake.Options, keyring secrets.Store) *rig {
	t.Helper()
	return newRigLogged(t, fopts, keyring, nil)
}

// newRigLogged is newRig with the manager's logger (nil: slog's default).
func newRigLogged(t *testing.T, fopts plexfake.Options, keyring secrets.Store, logger *slog.Logger) *rig {
	t.Helper()
	clk := clock.NewFake(time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC))
	fopts.Now = clk.Now
	f, err := plexfake.New(fopts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	r := &rig{t: t, clk: clk, fake: f}
	r.cfgDir = filepath.Join(t.TempDir(), "config")
	var mgr atomic.Pointer[Manager]
	r.store, err = config.Open(config.Options{Dir: r.cfgDir, Clock: clk, OnChange: func() {
		if m := mgr.Load(); m != nil {
			m.Reconfigure()
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.store.Load(); err != nil {
		t.Fatal(err)
	}
	if keyring == nil {
		r.mem = secrets.NewMemory()
		keyring = r.mem
	}
	r.keyring = keyring
	r.artDir = filepath.Join(t.TempDir(), "art")
	r.m, err = New(Options{
		Config: r.store, Secrets: keyring, Clock: clk, AccountURL: f.URL, Logger: logger,
		ClientIdentifier: "0123456789abcdef0123456789abcdef", ArtworkDir: r.artDir,
		OnChange:      func() { r.changes.Add(1) },
		ClientOptions: plex.ClientOptions{Retries: -1},
	})
	if err != nil {
		t.Fatal(err)
	}
	mgr.Store(r.m)
	return r
}

// run starts the refresh loop for the test's lifetime.
func (r *rig) run() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.m.Run(ctx); close(done) }()
	r.t.Cleanup(func() { cancel(); <-done })
}

func (r *rig) eventually(what string, cond func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			r.t.Fatalf("timed out waiting for %s (state %+v)", what, r.m.State())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// consistently asserts cond stays true for a short real-time window.
func (r *rig) consistently(what string, cond func() bool) {
	r.t.Helper()
	end := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(end) {
		if !cond() {
			r.t.Fatalf("%s did not hold", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// advanceTo waits until a timer is armed for exactly at, then advances the
// fake clock there (so the goroutine that arms it is never outrun).
func (r *rig) advanceTo(at time.Time) {
	r.t.Helper()
	r.eventually("a timer at "+at.Format(time.TimeOnly), func() bool {
		next, ok := r.clk.NextDeadline()
		return ok && !next.After(at) && !next.Before(r.clk.Now())
	})
	r.clk.Advance(at.Sub(r.clk.Now()))
}

func (r *rig) status() string { return r.m.State().Status }

// signIn walks the whole flow to connected with the given libraries.
func (r *rig) signIn(libraries ...string) {
	r.t.Helper()
	ctx := context.Background()
	if err := r.m.SignIn(ctx); err != nil {
		r.t.Fatal(err)
	}
	r.fake.Link()
	r.advanceTo(r.clk.Now().Add(DefaultPollInterval))
	r.eventually("choose_libraries", func() bool { return r.status() == contract.PlexChooseLibraries })
	if err := r.m.ChooseLibraries(ctx, libraries); err != nil {
		r.t.Fatal(err)
	}
}

func TestSignInLinksStoresTheTokenAndWritesConfig(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	ctx := context.Background()
	if st := r.m.State(); st.Status != contract.PlexSignedOut || r.m.Content() != nil {
		t.Fatalf("fresh state = %+v", st)
	}
	if n := len(r.fake.Requests()); n != 0 {
		t.Fatalf("%d requests before sign-in; nothing may leave the TV until the owner signs in", n)
	}
	if err := r.m.SignIn(ctx); err != nil {
		t.Fatal(err)
	}
	st := r.m.State()
	if st.Status != contract.PlexLinking || st.Code == nil || len(*st.Code) != 4 || st.LinkURL == nil || *st.LinkURL != "https://plex.tv/link" {
		t.Fatalf("linking state = %+v", st)
	}
	// Not linked yet: a poll changes nothing.
	r.advanceTo(r.clk.Now().Add(DefaultPollInterval))
	r.eventually("a poll", func() bool { return r.fake.Count("/api/v2/pins/"+pinID(r)) >= 1 })
	if r.status() != contract.PlexLinking {
		t.Fatalf("status = %s before the account linked", r.status())
	}
	r.fake.Link()
	r.advanceTo(r.clk.Now().Add(DefaultPollInterval))
	// One server: chosen automatically, libraries proposed.
	r.eventually("choose_libraries", func() bool { return r.status() == contract.PlexChooseLibraries })
	st = r.m.State()
	if st.Code != nil || st.Server == nil || *st.Server != "DEMO Plex Server" {
		t.Fatalf("state = %+v", st)
	}
	sel := map[string]bool{}
	for _, l := range st.Libraries {
		sel[l.ID] = l.Selected
	}
	if !sel["1"] || !sel["2"] || sel["3"] || len(st.Libraries) != 3 {
		t.Fatalf("libraries = %+v, want movie and show selected, music not", st.Libraries)
	}
	if tok, err := r.mem.Get(ctx, DefaultConnectionRef); err != nil || tok != r.fake.Token() {
		t.Fatalf("keyring holds %q, %v", tok, err)
	}
	if err := r.m.ChooseLibraries(ctx, []string{"2", "1"}); err != nil {
		t.Fatal(err)
	}
	cfg := r.store.Current()
	pc := cfg.PlexContent
	if !pc.Enabled || pc.ServerURL == nil || *pc.ServerURL != r.fake.URL || pc.ConnectionRef == nil || *pc.ConnectionRef != "plex" || strings.Join(pc.LibraryIDs, ",") != "2,1" {
		t.Fatalf("plex_content = %+v", pc)
	}
	for _, s := range cfg.Sections {
		if s.Kind != "applications" && !s.Enabled {
			t.Fatalf("section %s stayed off", s.ID)
		}
	}
	if st := r.m.State(); st.Status != contract.PlexConnected || st.Server == nil {
		t.Fatalf("state after finishing = %+v", st)
	}
	raw, err := os.ReadFile(filepath.Join(r.cfgDir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), r.fake.Token()) {
		t.Fatal("the token was written to config.json")
	}
	if r.m.Content() == nil {
		t.Fatal("no rows after sign-in")
	}
}

func pinID(r *rig) string {
	for _, q := range r.fake.Requests() {
		if strings.HasPrefix(q.Path, "/api/v2/pins/") {
			return strings.TrimPrefix(q.Path, "/api/v2/pins/")
		}
	}
	return "4100"
}

func TestSignInWithoutAKeyringFailsClosed(t *testing.T) {
	r := newRig(t, plexfake.Options{}, secrets.Unavailable{Reason: "no D-Bus session bus"})
	err := r.m.SignIn(context.Background())
	if err == nil || !strings.Contains(err.Error(), MsgNoStore) {
		t.Fatalf("err = %v", err)
	}
	st := r.m.State()
	if st.Status != contract.PlexError || !strings.HasPrefix(st.Message, MsgNoStore) || st.Code != nil {
		t.Fatalf("state = %+v", st)
	}
	if n := len(r.fake.Requests()); n != 0 {
		t.Fatalf("%d requests went out without a keyring", n)
	}
}

func TestLockedKeyringFailsClosed(t *testing.T) {
	mem := secrets.NewMemory()
	mem.SetLocked(true)
	r := newRig(t, plexfake.Options{}, mem)
	if err := r.m.SignIn(context.Background()); err == nil {
		t.Fatal("signed in with a locked keyring")
	}
	if !strings.Contains(r.m.State().Message, "locked") || len(r.fake.Requests()) != 0 {
		t.Fatalf("state = %+v, requests = %d", r.m.State(), len(r.fake.Requests()))
	}
}

// syncBuffer is a log sink safe for the loop's goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// The TV logs in automatically, so its keyring stays locked: the sign-in
// goes into a private file (0700 folder, 0600 file), the screen says so,
// the rows read it back, sign-out deletes it, and the token never reaches
// a log line (at debug level), config.json or the state.
func TestLockedKeyringKeepsTheSignInInAPrivateFile(t *testing.T) {
	keyring := secrets.NewMemory()
	keyring.SetLocked(true)
	dir := filepath.Join(t.TempDir(), "bear-den-tv", "secrets")
	store := secrets.NewFallback(keyring, secrets.NewFile(dir, "plex-"))
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := newRigLogged(t, plexfake.Options{}, store, logger)
	r.run()
	r.m.Observe(true, true)
	r.signIn("1")
	r.eventually("rows", func() bool { c := r.m.Content(); return c != nil && c.Status == providers.StatusReady })

	file := filepath.Join(dir, "plex-"+DefaultConnectionRef)
	raw, err := os.ReadFile(file)
	if err != nil || string(raw) != r.fake.Token() {
		t.Fatalf("the private file holds %q, %v", raw, err)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, file: 0o600} {
		fi, err := os.Lstat(path)
		if err != nil || fi.Mode().Perm() != want {
			t.Fatalf("%s: mode %v, %v; want %o", path, fi.Mode().Perm(), err, want)
		}
	}
	st := r.m.State()
	if st.Status != contract.PlexConnected || st.StoredIn != secrets.InFile {
		t.Fatalf("state = %+v, want connected with stored_in file", st)
	}
	cfgRaw, err := os.ReadFile(filepath.Join(r.cfgDir, config.FileName))
	if err != nil || strings.Contains(string(cfgRaw), r.fake.Token()) {
		t.Fatalf("config.json holds the token (or cannot be read: %v)", err)
	}

	if err := r.m.SignOut(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sign-out left the private file: %v", err)
	}
	if st := r.m.State(); st.Status != contract.PlexSignedOut || st.StoredIn != "" {
		t.Fatalf("state after sign-out = %+v", st)
	}
	out := logs.String()
	if !strings.Contains(out, "plex") {
		t.Fatalf("nothing was logged, so the check below proves nothing:\n%s", out)
	}
	if strings.Contains(out, r.fake.Token()) {
		t.Fatalf("the token reached the log:\n%s", out)
	}
}

// An unlocked keyring is used, and the state says so.
func TestUnlockedKeyringKeepsTheSignIn(t *testing.T) {
	keyring := secrets.NewMemory()
	dir := filepath.Join(t.TempDir(), "secrets")
	store := secrets.NewFallback(keyring, secrets.NewFile(dir, "plex-"))
	r := newRig(t, plexfake.Options{}, store)
	r.signIn("1")
	if tok, err := keyring.Get(context.Background(), DefaultConnectionRef); err != nil || tok != r.fake.Token() {
		t.Fatalf("keyring holds %q, %v", tok, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "plex-"+DefaultConnectionRef)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a file copy was written too: %v", err)
	}
	if st := r.m.State(); st.StoredIn != secrets.InKeyring {
		t.Fatalf("stored_in = %q", st.StoredIn)
	}
}

func TestLinkCodeExpires(t *testing.T) {
	r := newRig(t, plexfake.Options{PINTTL: 5 * time.Second}, nil)
	if err := r.m.SignIn(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3 && r.status() == contract.PlexLinking; i++ {
		r.advanceTo(r.clk.Now().Add(DefaultPollInterval))
		polls := i + 1
		r.eventually("poll", func() bool {
			return r.fake.Count("/api/v2/pins/"+pinID(r)) >= polls || r.status() != contract.PlexLinking
		})
	}
	r.eventually("expiry", func() bool { return r.status() == contract.PlexError })
	if msg := r.m.State().Message; msg != MsgCodeExpired {
		t.Fatalf("message = %q", msg)
	}
}

func TestChooseAmongSeveralServersProbesConnections(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	other, err := plexfake.New(plexfake.Options{MachineID: "other0machine", ServerName: "DEMO Second Server", Token: plexfake.DefaultToken})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	second := other.SelfResource()
	// A dead connection first: it must be skipped, not fatal.
	second.Connections = append([]plexfake.Connection{{Protocol: "https", URI: "https://127.0.0.1:1", Local: true}}, second.Connections...)
	// An impostor: its only connection answers as a different server.
	impostor := plexfake.Resource{Name: "DEMO Impostor", ClientIdentifier: "impostor", Provides: "server",
		Connections: []plexfake.Connection{{Protocol: "http", URI: r.fake.URL, Local: true}}}
	r.fake.SetResources([]plexfake.Resource{r.fake.SelfResource(), second, impostor})

	ctx := context.Background()
	if err := r.m.SignIn(ctx); err != nil {
		t.Fatal(err)
	}
	r.fake.Link()
	r.advanceTo(r.clk.Now().Add(DefaultPollInterval))
	r.eventually("choose_server", func() bool { return r.status() == contract.PlexChooseServer })
	if n := len(r.m.State().Servers); n != 3 {
		t.Fatalf("servers = %+v", r.m.State().Servers)
	}
	if err := r.m.ChooseServer(ctx, "impostor"); err == nil {
		t.Fatal("chose a server whose connection answers as another machine")
	}
	if st := r.m.State(); st.Status != contract.PlexChooseServer || !strings.Contains(st.Message, "can't reach") {
		t.Fatalf("after the impostor: %+v", st)
	}
	if err := r.m.ChooseServer(ctx, "no-such-server"); err == nil {
		t.Fatal("chose an unknown server")
	}
	if err := r.m.ChooseServer(ctx, "other0machine"); err != nil {
		t.Fatal(err)
	}
	if err := r.m.ChooseLibraries(ctx, []string{"1"}); err != nil {
		t.Fatal(err)
	}
	if u := r.store.Current().PlexContent.ServerURL; u == nil || *u != other.URL {
		t.Fatalf("server_url = %v, want the second server %s", u, other.URL)
	}
}

func TestChooseLibrariesNeedsOneKnownLibrary(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	ctx := context.Background()
	if err := r.m.ChooseLibraries(ctx, []string{"1"}); err == nil {
		t.Fatal("chose libraries before signing in")
	}
	if err := r.m.SignIn(ctx); err != nil {
		t.Fatal(err)
	}
	r.fake.Link()
	r.advanceTo(r.clk.Now().Add(DefaultPollInterval))
	r.eventually("choose_libraries", func() bool { return r.status() == contract.PlexChooseLibraries })
	if err := r.m.ChooseLibraries(ctx, nil); err == nil || r.m.State().Message != MsgPickLibraries {
		t.Fatalf("err = %v, state = %+v", err, r.m.State())
	}
	if err := r.m.ChooseLibraries(ctx, []string{"99"}); err == nil {
		t.Fatal("accepted a library that is not on the server")
	}
	if r.store.Current().PlexContent.Enabled {
		t.Fatal("plex_content enabled by a refused choice")
	}
}

func TestCancelForgetsAnUnfinishedSignIn(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	ctx := context.Background()
	if err := r.m.SignIn(ctx); err != nil {
		t.Fatal(err)
	}
	r.fake.Link()
	r.advanceTo(r.clk.Now().Add(DefaultPollInterval))
	r.eventually("choose_libraries", func() bool { return r.status() == contract.PlexChooseLibraries })
	if err := r.m.Cancel(ctx); err != nil {
		t.Fatal(err)
	}
	if r.status() != contract.PlexSignedOut {
		t.Fatalf("status = %s", r.status())
	}
	if _, err := r.mem.Get(ctx, DefaultConnectionRef); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("an unused token stayed in the keyring: %v", err)
	}
}

func TestSignOutForgetsEverything(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	r.run()
	r.m.Observe(true, true)
	r.signIn("1", "2")
	r.eventually("rows", func() bool { c := r.m.Content(); return c != nil && c.Status == providers.StatusReady })
	r.eventually("artwork", func() bool { return len(r.m.art.Scopes()) > 0 })
	ctx := context.Background()
	if err := r.m.SignOut(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.mem.Get(ctx, DefaultConnectionRef); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("token survived sign-out: %v", err)
	}
	cfg := r.store.Current()
	if cfg.PlexContent.Enabled || cfg.PlexContent.ServerURL != nil || len(cfg.PlexContent.LibraryIDs) != 0 {
		t.Fatalf("plex_content = %+v", cfg.PlexContent)
	}
	for _, s := range cfg.Sections {
		if s.Kind != "applications" && s.Enabled {
			t.Fatalf("section %s still on", s.ID)
		}
	}
	if r.m.Content() != nil || r.status() != contract.PlexSignedOut || len(r.m.art.Scopes()) != 0 {
		t.Fatalf("content=%v status=%s scopes=%v", r.m.Content(), r.status(), r.m.art.Scopes())
	}
	entries, _ := os.ReadDir(r.artDir)
	if len(entries) != 0 {
		t.Fatalf("artwork left on disk: %v", entries)
	}
}

func TestRowsShowItemsWithLocalArtwork(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	r.run()
	r.m.Observe(true, true)
	r.signIn("1", "2")
	r.eventually("rows", func() bool { c := r.m.Content(); return c != nil && c.Status == providers.StatusReady })
	c := r.m.Content()
	if c.Provider != "plex" || len(c.Sections) != 2 {
		t.Fatalf("content = %+v", c)
	}
	cw := c.Sections[0]
	if cw.SectionID != "plex-continue" || len(cw.Items) != 4 {
		t.Fatalf("continue watching = %+v", cw)
	}
	for _, it := range cw.Items {
		if it.Progress == nil || it.Artwork == nil || !strings.HasPrefix(*it.Artwork, r.artDir) || it.OpenAction != providers.OpenApp || it.Demo {
			t.Fatalf("item = %+v", it)
		}
	}
	if len(c.Sections[1].Items) != 7 {
		t.Fatalf("recently added = %d items", len(c.Sections[1].Items))
	}
	if st := r.m.State(); len(st.Libraries) != 3 {
		t.Fatalf("connected view libraries = %+v", st.Libraries)
	}
}

func TestRefreshCadenceFollowsHome(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	r.run()
	r.m.Observe(true, false) // Settings in front while signing in
	r.signIn("1", "2")
	hub := func() int { return r.fake.Count("/hubs/continueWatching") }
	// A sign-in refreshes once while the shell is in front.
	r.eventually("first refresh", func() bool { return hub() == 1 && !r.m.refreshingNow() })

	// Home shown right after: fresh enough, no second fetch.
	r.m.Observe(true, true)
	r.consistently("no refetch within the gap", func() bool { return hub() == 1 })

	// Every 10 minutes while Home stays in front.
	r.advanceTo(r.clk.Now().Add(DefaultRefreshInterval))
	r.eventually("periodic refresh", func() bool { return hub() == 2 && !r.m.refreshingNow() })

	// An app in front: nothing, however long.
	r.m.Observe(false, false)
	r.clk.Advance(3 * DefaultRefreshInterval)
	r.consistently("no refresh behind an app", func() bool { return hub() == 2 })

	// Back Home after a long time: refresh at once.
	r.m.Observe(true, true)
	r.eventually("refresh on Home", func() bool { return hub() == 3 })
}

func TestForcedRefreshWaitsForTheShell(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	r.run()
	r.m.Observe(true, false)
	r.signIn("1")
	r.eventually("first refresh", func() bool { return r.fake.Count("/hubs/continueWatching") == 1 && !r.m.refreshingNow() })
	r.m.Observe(false, false) // an app comes to the front
	// Changing the rows (a section edit) forces a refresh, but not now.
	if _, err := r.store.Update(func(c *config.Config) error {
		for i := range c.Sections {
			if c.Sections[i].Kind == providers.KindRecentlyAdded {
				c.Sections[i].Enabled = false
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r.consistently("no refresh behind an app", func() bool { return r.fake.Count("/hubs/continueWatching") == 1 })
	r.m.Observe(true, false)
	r.eventually("refresh once the shell is back", func() bool { return r.fake.Count("/hubs/continueWatching") == 2 })
}

func TestFailuresBackOffAndSayWhy(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	r.run()
	r.m.Observe(false, false) // an app in front: sign in, but no refresh yet
	r.signIn("1", "2")
	r.fake.SetDown(true)
	ident := func() int { return r.fake.Count("/identity") }
	r.m.Observe(true, true)
	r.eventually("first failed refresh", func() bool {
		c := r.m.Content()
		return c != nil && c.Status == providers.StatusError && !r.m.refreshingNow()
	})
	if msg := r.m.Content().Message; msg != plex.MsgUnreachable {
		t.Fatalf("rows say %q, want %q", msg, plex.MsgUnreachable)
	}
	// Each failed attempt costs one /identity call. The schedule after the
	// first failure is 30 s, then 1 min, then 2 min, then 5 min.
	base := ident()
	at := r.clk.Now()
	for n, gap := range DefaultBackoff[:3] {
		want := base + n + 1
		r.advanceTo(at.Add(gap))
		r.eventually("retry", func() bool { return ident() == want && !r.m.refreshingNow() })
		at = at.Add(gap)
		nextGap := DefaultBackoff[n+1]
		r.eventually("the next retry armed", func() bool {
			next, ok := r.clk.NextDeadline()
			return ok && next.Equal(at.Add(nextGap))
		})
	}
	// Home shown again during a backoff does not jump the queue.
	r.m.Observe(true, false)
	r.m.Observe(true, true)
	r.consistently("no retry before the backoff ends", func() bool { return ident() == base+3 })
	// Recovery resets to the normal cadence.
	r.fake.SetDown(false)
	r.advanceTo(at.Add(DefaultBackoff[3]))
	r.eventually("recovered", func() bool {
		c := r.m.Content()
		return c.Status == providers.StatusReady && !r.m.refreshingNow()
	})
	r.eventually("the normal cadence armed", func() bool {
		next, ok := r.clk.NextDeadline()
		return ok && next.Equal(r.clk.Now().Add(DefaultRefreshInterval))
	})
}

func (m *Manager) refreshingNow() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.refreshing
}

func TestKeyringMissingAtStartupShowsWhyOnTheRows(t *testing.T) {
	r := newRig(t, plexfake.Options{}, nil)
	r.run()
	r.m.Observe(true, true)
	r.signIn("1")
	r.eventually("rows", func() bool { c := r.m.Content(); return c != nil && c.Status == providers.StatusReady })
	// The keyring loses the token (another tool deleted it); a new connector
	// (as after a restart) must say so instead of showing empty rows.
	_ = r.mem.Delete(context.Background(), DefaultConnectionRef)
	r.m.mu.Lock()
	r.m.rebuild = true
	r.m.mu.Unlock()
	r.m.Reconfigure()
	r.eventually("an explained error", func() bool {
		c := r.m.Content()
		return c != nil && c.Status == providers.StatusError && c.Message == "Plex account is not linked"
	})
}
