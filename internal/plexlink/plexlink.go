// Package plexlink owns Plex on the TV: the sign-in flow driven from
// Settings → Plex (state.plex, contracts/ipc.md plex.*) and the Home rows fed
// by internal/providers/plex once plex_content is enabled (state.content).
//
// Privacy (docs/security.md): nothing here touches the network until the
// owner chooses Sign in. From then on it talks to plex.tv (link code, server
// list) and, after the owner picks a server, only to that server. The token
// goes into the keyring (internal/secrets) under plex_content.connection_ref
// and nowhere else; without a usable keyring sign-in fails closed.
//
// Refresh cadence: rows refresh when Home comes to the front (unless they
// did less than MinRefreshGap ago) and every RefreshInterval while Home stays
// in front, never while an app or another screen is in front. Failures back
// off (Backoff) and the rows show an honest message.
package plexlink

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/config"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/providers"
	"bear-den-tv/internal/providers/plex"
	"bear-den-tv/internal/secrets"
)

// DefaultConnectionRef is the keyring reference used when plex_content has
// none.
const DefaultConnectionRef = "plex"

// Cadence defaults (Options fields override them).
const (
	DefaultPollInterval    = 2 * time.Second
	DefaultRefreshInterval = 10 * time.Minute
	DefaultMinRefreshGap   = 2 * time.Minute
	DefaultProbeTimeout    = 4 * time.Second
	// pollFailures is how many failed polls in a row end linking.
	pollFailures = 5
)

// DefaultBackoff is the pause after the 1st, 2nd, ... failed refresh; the
// last value repeats.
var DefaultBackoff = []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute}

// User-facing messages (state.plex.message).
const (
	MsgNoKeyring     = "Plex sign-in needs a keyring; install or enable gnome-keyring"
	MsgPlexTV        = "Can't reach plex.tv; check the TV's internet connection"
	MsgCodeExpired   = "The code expired. Choose Sign in to get a new one."
	MsgNoServers     = "This Plex account has no Plex Media Server"
	MsgPickLibraries = "Choose at least one library"
)

// ConfigStore is the slice of config.Store the manager uses.
type ConfigStore interface {
	Current() config.Config
	Update(mutate func(*config.Config) error) (int64, error)
}

// Options wires a Manager. Config and Secrets are required.
type Options struct {
	Config  ConfigStore
	Secrets secrets.Store
	Clock   clock.Clock
	Logger  *slog.Logger
	// AccountURL is the plex.tv origin; default plex.DefaultAccountURL. Dev
	// runs point it at plexfake.
	AccountURL string
	// Transport overrides HTTP for plex.tv and the server (tests).
	Transport http.RoundTripper
	// ClientIdentifier is the stable X-Plex-Client-Identifier.
	ClientIdentifier string
	// ArtworkDir holds cached posters; ArtworkBudget bounds it in bytes
	// (0: plex.DefaultArtworkBudgetMiB).
	ArtworkDir    string
	ArtworkBudget int64
	// OnChange is called (never under the manager's lock) whenever state.plex
	// or the rows may have changed.
	OnChange func()

	PollInterval    time.Duration
	RefreshInterval time.Duration
	MinRefreshGap   time.Duration
	ProbeTimeout    time.Duration
	Backoff         []time.Duration
	// FeedTimeout bounds one refresh (providers.FeedOptions.RefreshTimeout).
	FeedTimeout time.Duration
	// ClientOptions tune the server client (tests: Retries -1).
	ClientOptions plex.ClientOptions
	// OnPass, when set, is called (never under the manager's lock) after
	// each decision of Run's loop about the rows, so tests can wait for the
	// loop instead of sleeping. Production leaves it nil.
	OnPass func(Pass)
}

// Pass is one decision of Run's loop (Options.OnPass): whether the rows
// exist (Feed), a refresh was due (Due), the shell in front allowed one
// (Allowed), and whether a refresh started (Started).
type Pass struct {
	Feed, Due, Allowed, Started bool
}

// flow is one sign-in attempt, from Sign in to the library choice.
type flow struct {
	status    string
	message   string
	code      string
	cancel    context.CancelFunc
	token     string
	servers   []plex.Server
	server    *plex.Server
	serverURL string
	libraries []plex.Library
	selected  map[string]bool
}

// Manager is the Plex sign-in flow plus the Home rows. Safe for concurrent
// use. Run must be running for rows to refresh.
type Manager struct {
	opts     Options
	clk      clock.Clock
	log      *slog.Logger
	art      *plex.ArtworkCache
	wakeC    chan struct{}
	wg       sync.WaitGroup // in-flight refreshes; Run waits for them
	onChange atomic.Pointer[func()]

	mu          sync.Mutex
	base        context.Context
	fl          *flow
	notice      string // message shown while signed out or connected
	serverName  string
	libraries   []plex.Library // the connected server's libraries, when known
	provider    *plex.Provider
	feed        *providers.Feed
	feedKey     string
	sectionsKey string
	rebuild     bool
	shellFront  bool
	homeVisible bool
	force       bool
	refreshing  bool
	failures    int
	lastSuccess time.Time
	nextDue     time.Time
}

// New builds a Manager; no network traffic happens here.
func New(opts Options) (*Manager, error) {
	if opts.Config == nil || opts.Secrets == nil {
		return nil, errors.New("plexlink: config and secrets are required")
	}
	if opts.Clock == nil {
		opts.Clock = clock.Real{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if opts.AccountURL == "" {
		opts.AccountURL = plex.DefaultAccountURL
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = DefaultPollInterval
	}
	if opts.RefreshInterval <= 0 {
		opts.RefreshInterval = DefaultRefreshInterval
	}
	if opts.MinRefreshGap <= 0 {
		opts.MinRefreshGap = DefaultMinRefreshGap
	}
	if opts.ProbeTimeout <= 0 {
		opts.ProbeTimeout = DefaultProbeTimeout
	}
	if len(opts.Backoff) == 0 {
		opts.Backoff = DefaultBackoff
	}
	if opts.ClientIdentifier == "" {
		return nil, errors.New("plexlink: client identifier is required")
	}
	if opts.ArtworkDir == "" {
		return nil, errors.New("plexlink: artwork directory is required")
	}
	art, err := plex.NewArtworkCache(opts.ArtworkDir, opts.ArtworkBudget, opts.Clock)
	if err != nil {
		return nil, err
	}
	m := &Manager{opts: opts, clk: opts.Clock, log: opts.Logger, art: art, wakeC: make(chan struct{}, 1), base: context.Background()}
	if opts.OnChange != nil {
		m.SetOnChange(opts.OnChange)
	}
	m.Reconfigure()
	return m, nil
}

func (m *Manager) ref(cfg config.Config) string {
	if r := cfg.PlexContent.ConnectionRef; r != nil && *r != "" {
		return *r
	}
	return DefaultConnectionRef
}

// SetOnChange replaces Options.OnChange (the session sets it once the
// coordinator exists).
func (m *Manager) SetOnChange(fn func()) { m.onChange.Store(&fn) }

func (m *Manager) changed() {
	if fn := m.onChange.Load(); fn != nil && *fn != nil {
		(*fn)()
	}
}

func (m *Manager) wake() {
	select {
	case m.wakeC <- struct{}{}:
	default:
	}
}

// ---- state ----

// State is state.plex.
func (m *Manager) State() contract.Plex {
	cfg := m.opts.Config.Current()
	m.mu.Lock()
	defer m.mu.Unlock()
	st := contract.Plex{Status: contract.PlexSignedOut, Servers: []contract.PlexServer{}, Libraries: []contract.PlexLibrary{}}
	if f := m.fl; f != nil {
		st.Status, st.Message = f.status, f.message
		if f.status == contract.PlexLinking && f.code != "" {
			code, link := f.code, plex.LinkURL
			st.Code, st.LinkURL = &code, &link
		}
		if f.server != nil {
			name := f.server.Name
			st.Server = &name
		}
		if f.status == contract.PlexChooseServer {
			for _, s := range f.servers {
				local := false
				for _, c := range s.Connections {
					local = local || c.Local
				}
				st.Servers = append(st.Servers, contract.PlexServer{ID: s.MachineIdentifier, Name: s.Name, Owned: s.Owned, Local: local})
			}
		}
		if f.status == contract.PlexChooseLibraries {
			st.Libraries = libraryList(f.libraries, f.selected)
		}
		return st
	}
	st.Message = m.notice
	if cfg.PlexContent.Enabled {
		st.Status = contract.PlexConnected
		if m.serverName != "" {
			name := m.serverName
			st.Server = &name
		}
		sel := map[string]bool{}
		for _, id := range cfg.PlexContent.LibraryIDs {
			sel[id] = true
		}
		if len(cfg.PlexContent.LibraryIDs) == 0 {
			sel = defaultSelection(m.libraries)
		}
		st.Libraries = libraryList(m.libraries, sel)
	}
	return st
}

func libraryKind(t string) string {
	switch t {
	case "movie", "show", "artist", "photo":
		return t
	}
	return "other"
}

func libraryList(libs []plex.Library, selected map[string]bool) []contract.PlexLibrary {
	out := []contract.PlexLibrary{}
	for _, l := range libs {
		id := string(l.Key)
		out = append(out, contract.PlexLibrary{ID: id, Title: l.Title, Kind: libraryKind(l.Type), Selected: selected[id]})
	}
	return out
}

func defaultSelection(libs []plex.Library) map[string]bool {
	sel := map[string]bool{}
	for _, l := range libs {
		if l.Type == "movie" || l.Type == "show" {
			sel[string(l.Key)] = true
		}
	}
	return sel
}

// Content is state.content for the Plex rows, or nil while Plex is off.
func (m *Manager) Content() *contract.Content {
	m.mu.Lock()
	feed := m.feed
	m.mu.Unlock()
	if feed == nil {
		return nil
	}
	ct := feed.Snapshot()
	return &ct
}

// ---- the rows ----

// plexSections lists the enabled Plex-backed sections of the layout.
func plexSections(cfg config.Config) []providers.SectionConfig {
	var out []providers.SectionConfig
	for _, s := range cfg.Sections {
		if !s.Enabled {
			continue
		}
		switch s.Kind {
		case providers.KindContinueWatching, providers.KindRecentlyAdded:
			out = append(out, providers.SectionConfig{ID: s.ID, Kind: s.Kind, LibraryIDs: cfg.PlexContent.LibraryIDs, Limit: 20})
		}
	}
	return out
}

// Reconfigure brings the rows in line with config.json: builds, rebuilds or
// drops the provider when plex_content changes, and follows section edits.
// Call it after every configuration change.
func (m *Manager) Reconfigure() {
	cfg := m.opts.Config.Current()
	pc := cfg.PlexContent
	key := ""
	if pc.Enabled && pc.ServerURL != nil {
		key = fmt.Sprintf("%s|%s|%s", *pc.ServerURL, strings.Join(pc.LibraryIDs, ","), m.ref(cfg))
	}
	sections := plexSections(cfg)
	m.mu.Lock()
	if key == "" {
		if m.provider != nil {
			m.provider.Disconnect()
		}
		m.provider, m.feed, m.feedKey = nil, nil, ""
		m.mu.Unlock()
		return
	}
	secKey := fmt.Sprint(sections)
	if key == m.feedKey && !m.rebuild {
		if secKey == m.sectionsKey {
			m.mu.Unlock() // an unrelated change (theme, remote, ...)
			return
		}
		m.sectionsKey = secKey
		m.feed.SetSections(sections)
		m.force = true
		m.mu.Unlock()
		m.wake()
		return
	}
	m.sectionsKey = secKey
	m.rebuild = false
	copts := m.opts.ClientOptions
	p, err := plex.New(plex.Options{
		ServerURL: *pc.ServerURL, ConnectionRef: m.ref(cfg), LibraryIDs: pc.LibraryIDs,
		ClientIdentifier: m.opts.ClientIdentifier, Secrets: m.opts.Secrets, Artwork: m.art,
		Logger: m.log, Transport: m.opts.Transport, Client: copts,
	})
	if err != nil {
		// Validated config should never get here; say so on the rows.
		m.log.Warn("plex: cannot build the connector", "err", err)
		m.provider, m.feed, m.feedKey = nil, nil, ""
		m.notice = "Plex settings are not usable; sign in again"
		m.mu.Unlock()
		return
	}
	if m.provider != nil {
		m.provider.Disconnect()
	}
	m.provider = p
	m.feed = providers.NewFeed(p, sections, providers.FeedOptions{Logger: m.log, Clock: m.clk, RefreshTimeout: m.opts.FeedTimeout})
	m.feedKey = key
	m.failures, m.nextDue, m.lastSuccess = 0, time.Time{}, time.Time{}
	m.force = true
	m.mu.Unlock()
	m.wake()
}

// Observe tells the manager what is in front: shellFront when Bear Den's
// shell is (not an app, not locked), home when that screen is Home. Called
// on every state publish; cheap.
func (m *Manager) Observe(shellFront, home bool) {
	home = home && shellFront
	m.mu.Lock()
	cameHome := home && !m.homeVisible
	changed := shellFront != m.shellFront || home != m.homeVisible
	m.shellFront, m.homeVisible = shellFront, home
	if cameHome && m.failures == 0 && (m.lastSuccess.IsZero() || m.clk.Since(m.lastSuccess) >= m.opts.MinRefreshGap) {
		m.force = true
	}
	m.mu.Unlock()
	if changed {
		m.wake()
	}
}

// Run drives refreshes until ctx ends. Sign-in flows started later use ctx
// as their parent.
func (m *Manager) Run(ctx context.Context) {
	m.mu.Lock()
	m.base = ctx
	m.mu.Unlock()
	timer := m.clk.NewTimer(time.Hour)
	defer timer.Stop()
	defer m.wg.Wait()
	for {
		m.maybeRefresh(ctx)
		timer.Stop()
		if d, ok := m.nextWait(); ok {
			timer.Reset(d)
		}
		select {
		case <-ctx.Done():
			m.mu.Lock()
			if m.fl != nil && m.fl.cancel != nil {
				m.fl.cancel()
			}
			m.mu.Unlock()
			return
		case <-m.wakeC:
		case <-timer.C():
		}
	}
}

// nextWait is how long until the next scheduled refresh, if one is due
// while Home is in front.
func (m *Manager) nextWait() (time.Duration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.feed == nil || !m.homeVisible || m.refreshing || m.nextDue.IsZero() {
		return 0, false
	}
	d := m.nextDue.Sub(m.clk.Now())
	if d < 0 {
		d = 0
	}
	return d, true
}

func (m *Manager) backoff(failures int) time.Duration {
	b := m.opts.Backoff
	if failures > len(b) {
		return b[len(b)-1]
	}
	return b[failures-1]
}

// maybeRefresh starts one refresh when it is allowed and due: Home in front
// (or, for a refresh forced by a sign-in or settings change, the shell in
// front), none running, and either forced or past nextDue.
func (m *Manager) maybeRefresh(ctx context.Context) {
	m.mu.Lock()
	feed := m.feed
	allowed := m.homeVisible || (m.force && m.shellFront)
	due := m.force || (!m.nextDue.IsZero() && !m.clk.Now().Before(m.nextDue))
	if feed == nil || m.refreshing || !allowed || !due {
		m.mu.Unlock()
		m.pass(Pass{Feed: feed != nil, Due: due, Allowed: allowed})
		return
	}
	m.refreshing, m.force = true, false
	m.mu.Unlock()
	m.pass(Pass{Feed: true, Due: true, Allowed: true, Started: true})

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		feed.Refresh(ctx)
		snap := feed.Snapshot()
		ok := snap.Status == providers.StatusReady
		m.mu.Lock()
		now := m.clk.Now()
		m.refreshing = false
		if m.feed == feed {
			if ok {
				m.failures = 0
				m.lastSuccess = now
				m.nextDue = now.Add(m.opts.RefreshInterval)
			} else {
				m.failures++
				m.nextDue = now.Add(m.backoff(m.failures))
			}
		}
		prov := m.provider
		m.mu.Unlock()
		if !ok {
			m.log.Info("plex: rows refresh failed", "status", snap.Status, "message", snap.Message)
		}
		if prov != nil {
			m.learnLibraries(ctx, prov)
		}
		m.changed()
		m.wake()
	}()
}

func (m *Manager) pass(p Pass) {
	if m.opts.OnPass != nil {
		m.opts.OnPass(p)
	}
}

// learnLibraries fills the connected view's library list once, from the
// connected provider (no extra request when already known).
func (m *Manager) learnLibraries(ctx context.Context, p *plex.Provider) {
	m.mu.Lock()
	known := len(m.libraries) > 0
	m.mu.Unlock()
	if known {
		return
	}
	libs, err := p.Libraries(ctx)
	if err != nil || len(libs) == 0 {
		return
	}
	m.mu.Lock()
	m.libraries = libs
	m.mu.Unlock()
}

// ---- sign-in flow ----

func (m *Manager) account() (*plex.Account, error) {
	return plex.NewAccount(plex.AccountOptions{
		BaseURL: m.opts.AccountURL, ClientIdentifier: m.opts.ClientIdentifier,
		Transport: m.opts.Transport, Now: m.clk.Now,
	})
}

// setFlow replaces the flow's status and message (and nothing else).
func (m *Manager) setFlow(f *flow, status, message string) {
	m.mu.Lock()
	if m.fl == f {
		f.status, f.message = status, message
	}
	m.mu.Unlock()
	m.changed()
}

// SignIn starts linking: a keyring check, then a link code from plex.tv.
// It returns once the code is shown (or with the reason it cannot be).
func (m *Manager) SignIn(ctx context.Context) error {
	m.stopFlow()
	if ok, reason := m.opts.Secrets.Available(ctx); !ok {
		msg := MsgNoKeyring
		if reason != "" {
			msg += " (" + reason + ")"
		}
		m.mu.Lock()
		m.fl = &flow{status: contract.PlexError, message: msg}
		m.mu.Unlock()
		m.changed()
		return errors.New(msg)
	}
	acct, err := m.account()
	if err != nil {
		return err
	}
	pin, err := acct.StartPIN(ctx)
	if err != nil {
		m.log.Warn("plex: link code request failed", "err", err)
		m.mu.Lock()
		m.fl = &flow{status: contract.PlexError, message: MsgPlexTV}
		m.mu.Unlock()
		m.changed()
		return errors.New(MsgPlexTV)
	}
	m.mu.Lock()
	fctx, cancel := context.WithCancel(m.base)
	f := &flow{status: contract.PlexLinking, code: pin.Code, cancel: cancel}
	m.fl = f
	m.mu.Unlock()
	m.changed()
	go m.poll(fctx, f, acct, pin)
	return nil
}

// poll waits for the account to link the code.
func (m *Manager) poll(ctx context.Context, f *flow, acct *plex.Account, pin plex.PIN) {
	misses := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.clk.After(m.opts.PollInterval):
		}
		token, done, err := acct.PollPIN(ctx, pin.ID)
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, plex.ErrPINExpired):
			m.setFlow(f, contract.PlexError, MsgCodeExpired)
			return
		case err != nil:
			misses++
			m.log.Warn("plex: link code poll failed", "err", err)
			if misses >= pollFailures {
				m.setFlow(f, contract.PlexError, MsgPlexTV)
				return
			}
			continue
		case !done:
			misses = 0
			continue
		}
		m.linked(ctx, f, acct, token)
		return
	}
}

// linked stores the token and lists the account's servers.
func (m *Manager) linked(ctx context.Context, f *flow, acct *plex.Account, token string) {
	cfg := m.opts.Config.Current()
	if err := m.opts.Secrets.Set(ctx, m.ref(cfg), token, "Bear Den TV: Plex sign-in"); err != nil {
		m.log.Warn("plex: could not store the token", "err", err)
		m.setFlow(f, contract.PlexError, MsgNoKeyring)
		return
	}
	servers, err := acct.DiscoverServers(ctx, token)
	if err != nil {
		m.log.Warn("plex: server discovery failed", "err", err)
		m.setFlow(f, contract.PlexError, MsgPlexTV)
		return
	}
	m.mu.Lock()
	if m.fl != f {
		m.mu.Unlock()
		return
	}
	f.token, f.servers, f.code = token, servers, ""
	m.mu.Unlock()
	switch len(servers) {
	case 0:
		m.setFlow(f, contract.PlexError, MsgNoServers)
	case 1:
		if err := m.chooseServer(ctx, f, servers[0].MachineIdentifier); err != nil {
			m.setFlow(f, contract.PlexError, err.Error())
		}
	default:
		m.setFlow(f, contract.PlexChooseServer, "")
	}
}

// ChooseServer picks one of the listed servers (plex.choose_server).
func (m *Manager) ChooseServer(ctx context.Context, id string) error {
	m.mu.Lock()
	f := m.fl
	m.mu.Unlock()
	if f == nil || f.status != contract.PlexChooseServer {
		return errors.New("there is no server to choose right now; choose Sign in first")
	}
	if err := m.chooseServer(ctx, f, id); err != nil {
		m.setFlow(f, contract.PlexChooseServer, err.Error())
		return err
	}
	return nil
}

// chooseServer probes the server's connections in order and lists the
// libraries of the first that answers as that server.
func (m *Manager) chooseServer(ctx context.Context, f *flow, id string) error {
	m.mu.Lock()
	var server *plex.Server
	for i := range f.servers {
		if f.servers[i].MachineIdentifier == id {
			s := f.servers[i]
			server = &s
		}
	}
	token := f.token
	m.mu.Unlock()
	if server == nil {
		return errors.New("that server is not on this Plex account")
	}
	for _, conn := range server.Connections {
		copts := m.opts.ClientOptions
		copts.ServerURL, copts.ClientIdentifier, copts.Transport = conn.URI, m.opts.ClientIdentifier, m.opts.Transport
		copts.RequestTimeout, copts.ConnectTimeout, copts.Retries = m.opts.ProbeTimeout, m.opts.ProbeTimeout, -1
		client, err := plex.NewClient(copts)
		if err != nil {
			continue
		}
		client.SetToken(token)
		ident, err := client.Identity(ctx)
		if err != nil || ident.MachineIdentifier != server.MachineIdentifier {
			continue
		}
		libs, err := client.Libraries(ctx)
		if err != nil {
			continue
		}
		base, _ := plex.ParseServerURL(conn.URI)
		m.mu.Lock()
		if m.fl != f {
			m.mu.Unlock()
			return errors.New("sign-in was cancelled")
		}
		sort.SliceStable(libs, func(i, j int) bool { return libs[i].Title < libs[j].Title })
		f.server, f.serverURL, f.libraries = server, base.String(), libs
		f.selected = defaultSelection(libs)
		f.status, f.message = contract.PlexChooseLibraries, ""
		m.mu.Unlock()
		m.changed()
		return nil
	}
	return fmt.Errorf("can't reach %s from this TV", server.Name)
}

// ChooseLibraries finishes sign-in (plex.choose_libraries): plex_content is
// written, the Plex sections are turned on and the rows refresh.
func (m *Manager) ChooseLibraries(ctx context.Context, ids []string) error {
	m.mu.Lock()
	f := m.fl
	if f == nil || f.status != contract.PlexChooseLibraries {
		m.mu.Unlock()
		return errors.New("there are no libraries to choose right now; choose Sign in first")
	}
	known := map[string]bool{}
	for _, l := range f.libraries {
		known[string(l.Key)] = true
	}
	libs := append([]plex.Library(nil), f.libraries...)
	serverURL, serverName := f.serverURL, f.server.Name
	m.mu.Unlock()
	var chosen []string
	seen := map[string]bool{}
	for _, id := range ids {
		if !known[id] {
			return fmt.Errorf("library %q is not on this server", id)
		}
		if !seen[id] {
			seen[id] = true
			chosen = append(chosen, id)
		}
	}
	if len(chosen) == 0 {
		m.setFlow(f, contract.PlexChooseLibraries, MsgPickLibraries)
		return errors.New(MsgPickLibraries)
	}
	m.mu.Lock()
	m.rebuild = true // a new token may sit behind an unchanged config
	m.mu.Unlock()
	_, err := m.opts.Config.Update(func(c *config.Config) error {
		ref := m.ref(*c)
		u := serverURL
		c.PlexContent = config.PlexContent{Enabled: true, ConnectionRef: &ref, ServerURL: &u, LibraryIDs: chosen}
		for i := range c.Sections {
			switch c.Sections[i].Kind {
			case providers.KindContinueWatching, providers.KindRecentlyAdded:
				c.Sections[i].Enabled = true
			}
		}
		return nil
	})
	if err != nil {
		m.setFlow(f, contract.PlexChooseLibraries, "Could not save the Plex settings: "+err.Error())
		return err
	}
	m.mu.Lock()
	if m.fl == f {
		m.fl = nil
	}
	m.serverName, m.libraries, m.notice = serverName, libs, ""
	m.mu.Unlock()
	m.Reconfigure()
	m.changed()
	return nil
}

// stopFlow ends a running flow without touching the keyring.
func (m *Manager) stopFlow() *flow {
	m.mu.Lock()
	f := m.fl
	m.fl = nil
	m.mu.Unlock()
	if f != nil && f.cancel != nil {
		f.cancel()
	}
	return f
}

// Cancel abandons sign-in (plex.cancel). A token already stored for an
// unfinished sign-in is removed, so no unused credential stays behind.
func (m *Manager) Cancel(ctx context.Context) error {
	f := m.stopFlow()
	cfg := m.opts.Config.Current()
	if f != nil && f.token != "" && !cfg.PlexContent.Enabled {
		if err := m.opts.Secrets.Delete(ctx, m.ref(cfg)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			m.log.Warn("plex: could not remove an unused token", "err", err)
		}
	}
	m.changed()
	return nil
}

// SignOut forgets the account (plex.sign_out): the token leaves the
// keyring, plex_content is turned off, the Plex sections are hidden and the
// cached artwork is deleted. Everything but the keyring step happens even
// when that step fails; the error then says why.
func (m *Manager) SignOut(ctx context.Context) error {
	m.stopFlow()
	cfg := m.opts.Config.Current()
	var errs []error
	if err := m.opts.Secrets.Delete(ctx, m.ref(cfg)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		errs = append(errs, fmt.Errorf("the token could not be removed from the keyring: %w", err))
	}
	if _, err := m.opts.Config.Update(func(c *config.Config) error {
		c.PlexContent.Enabled = false
		c.PlexContent.ServerURL = nil
		c.PlexContent.LibraryIDs = nil
		for i := range c.Sections {
			switch c.Sections[i].Kind {
			case providers.KindContinueWatching, providers.KindRecentlyAdded:
				c.Sections[i].Enabled = false
			}
		}
		return nil
	}); err != nil {
		errs = append(errs, err)
	}
	m.Reconfigure()
	for _, scope := range m.art.Scopes() {
		if err := m.art.Purge(scope); err != nil {
			errs = append(errs, err)
		}
	}
	err := errors.Join(errs...)
	m.mu.Lock()
	m.serverName, m.libraries = "", nil
	m.notice = ""
	if err != nil {
		m.notice = "Signed out, but " + err.Error()
	}
	m.mu.Unlock()
	m.changed()
	return err
}
