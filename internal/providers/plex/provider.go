// Provider: the Plex ContentProvider for home-screen sections.

package plex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"sync"

	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/providers"
	"bear-den-tv/internal/secrets"
)

// Name is the contract content.provider value.
const Name = "plex"

// DefaultAppID is the applications[].id the connector opens (adapter plex-htpc).
const DefaultAppID = "plex-htpc"

// Defaults for item pages and artwork requests.
const (
	DefaultPageSize      = 20
	DefaultArtworkWidth  = 480
	DefaultArtworkHeight = 720
)

// ErrNotConnected reports a fetch before a successful Connect.
var ErrNotConnected = errors.New("plex: not connected")

// User-facing messages the Home rails show (state.content.message).
const (
	MsgUnreachable   = "Can't reach your Plex server"
	MsgTokenRejected = "Plex no longer accepts this TV's sign-in; sign in again in Settings → Plex"
)

// userError carries a user-facing message while keeping the cause for
// errors.Is/As; the cause's text (already redacted) goes only to the log.
type userError struct {
	msg   string
	cause error
}

func (e *userError) Error() string { return e.msg }
func (e *userError) Unwrap() error { return e.cause }

// fetchError logs the redacted cause and returns the message the rails show.
func (p *Provider) fetchError(what string, err error) error {
	red := p.redactor.Error(err)
	p.logger.Warn("plex fetch failed", "what", what, "error", red)
	var se *StatusError
	if errors.As(err, &se) && (se.Status == http.StatusUnauthorized || se.Status == http.StatusForbidden) {
		return &userError{msg: MsgTokenRejected, cause: red}
	}
	return &userError{msg: MsgUnreachable, cause: red}
}

// Options builds a Provider from validated configuration (plex_content) and
// the coordinator's services. Secrets and Artwork are required.
type Options struct {
	// ServerURL is plex_content.server_url.
	ServerURL string
	// ConnectionRef is plex_content.connection_ref, the secrets.Store key.
	ConnectionRef string
	// LibraryIDs is plex_content.library_ids; empty means every movie and
	// show library. A configured id missing on the server fails Connect.
	LibraryIDs []string
	// ClientIdentifier is the stable per-install X-Plex-Client-Identifier
	// (LoadOrCreateClientIdentifier).
	ClientIdentifier string
	Secrets          secrets.Store
	Artwork          *ArtworkCache
	// Logger receives redacted diagnostics; nil discards.
	Logger *slog.Logger
	// Transport overrides the HTTP transport (tests, custom TLS roots).
	Transport http.RoundTripper
	// AppID is the application the open action launches; default DefaultAppID.
	AppID string
	// ArtworkWidth/ArtworkHeight are the transcode bounds; defaults 480×720.
	ArtworkWidth  int
	ArtworkHeight int
	// PageSize caps items per section; default 20.
	PageSize int
	// ClientOptions overrides timeouts and retries (tests).
	Client ClientOptions
}

type artRef struct {
	thumb     string
	libraryID string
}

// Provider is the Plex providers.ContentProvider. Connect reads the token
// from the secret store and verifies the server identity; fetches map server
// items into contract items without inventing progress or watched state.
// Safe for concurrent use.
type Provider struct {
	opts     Options
	client   *Client
	redactor *Redactor
	logger   *slog.Logger

	mu        sync.Mutex
	status    string
	message   string
	connected bool
	machineID string
	owner     string
	libraries []Library
	art       map[string]artRef
}

// New validates options and builds the provider without touching the
// network or the secret store.
func New(opts Options) (*Provider, error) {
	if opts.Secrets == nil {
		return nil, errors.New("plex: secrets store is required")
	}
	if opts.Artwork == nil {
		return nil, errors.New("plex: artwork cache is required")
	}
	if opts.ConnectionRef == "" {
		return nil, errors.New("plex: connection_ref is required")
	}
	if opts.AppID == "" {
		opts.AppID = DefaultAppID
	}
	if opts.ArtworkWidth <= 0 {
		opts.ArtworkWidth = DefaultArtworkWidth
	}
	if opts.ArtworkHeight <= 0 {
		opts.ArtworkHeight = DefaultArtworkHeight
	}
	if opts.PageSize <= 0 {
		opts.PageSize = DefaultPageSize
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	copts := opts.Client
	copts.ServerURL = opts.ServerURL
	copts.ClientIdentifier = opts.ClientIdentifier
	if copts.Transport == nil {
		copts.Transport = opts.Transport
	}
	client, err := NewClient(copts)
	if err != nil {
		return nil, err
	}
	r := NewRedactor()
	return &Provider{
		opts:     opts,
		client:   client,
		redactor: r,
		logger:   slog.New(r.Handler(opts.Logger.Handler())),
		status:   providers.StatusConnecting,
		art:      map[string]artRef{},
	}, nil
}

// Name implements providers.ContentProvider.
func (p *Provider) Name() string { return Name }

// Redactor is the provider's redactor; wrap any logger that may see
// provider errors with its Handler.
func (p *Provider) Redactor() *Redactor { return p.redactor }

func (p *Provider) setStatus(status, message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status, p.message = status, message
}

// Status implements providers.ContentProvider.
func (p *Provider) Status() (string, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status, p.message
}

// Connect implements providers.ContentProvider: token lookup, /identity,
// and library resolution. Errors carry user-facing, redacted text and leave
// Status at "error" with the same message.
func (p *Provider) Connect(ctx context.Context) error {
	p.setStatus(providers.StatusConnecting, "")
	fail := func(msg string, err error) error {
		p.setStatus(providers.StatusError, msg)
		p.logger.Warn("plex connect failed", "reason", msg, "error", err)
		return p.redactor.Error(fmt.Errorf("%s: %w", msg, err))
	}
	token, err := p.opts.Secrets.Get(ctx, p.opts.ConnectionRef)
	switch {
	case errors.Is(err, secrets.ErrLocked):
		return fail("Keyring is locked; unlock it to load Plex rows", err)
	case errors.Is(err, secrets.ErrNotFound):
		return fail("Plex account is not linked", err)
	case errors.Is(err, secrets.ErrUnavailable):
		return fail("No keyring is available for the Plex token", err)
	case err != nil:
		return fail("Plex token could not be read", err)
	}
	p.redactor.AddSecret(token)
	p.client.SetToken(token)

	// /identity answers without a token on real servers, so a revoked token
	// first shows up as a 401 from /library/sections: classify both alike.
	classify := func(err error) error {
		var se *StatusError
		if errors.As(err, &se) && (se.Status == http.StatusUnauthorized || se.Status == http.StatusForbidden) {
			return fail(MsgTokenRejected, err)
		}
		return fail(MsgUnreachable, err)
	}
	identity, err := p.client.Identity(ctx)
	if err != nil {
		return classify(err)
	}
	libraries, err := p.client.Libraries(ctx)
	if err != nil {
		return classify(err)
	}
	selected, err := selectLibraries(libraries, p.opts.LibraryIDs)
	if err != nil {
		return fail(err.Error(), err)
	}
	p.mu.Lock()
	p.machineID = identity.MachineIdentifier
	p.owner = OwnerKey(identity.MachineIdentifier)
	p.libraries = selected
	p.connected = true
	p.status, p.message = providers.StatusReady, ""
	p.art = map[string]artRef{}
	p.mu.Unlock()
	p.logger.Info("plex connected", "server_version", identity.Version, "libraries", len(selected))
	return nil
}

// Sessions is what the connected server says is playing (sessions.go). The
// token comes from the keyring when the rows have not connected yet; a
// failure is returned redacted.
func (p *Provider) Sessions(ctx context.Context) ([]Session, error) {
	p.mu.Lock()
	connected := p.connected
	p.mu.Unlock()
	if !connected {
		token, err := p.opts.Secrets.Get(ctx, p.opts.ConnectionRef)
		if err != nil {
			return nil, p.redactor.Error(fmt.Errorf("plex token could not be read: %w", err))
		}
		p.redactor.AddSecret(token)
		p.client.SetToken(token)
	}
	ss, err := p.client.Sessions(ctx)
	if err != nil {
		return nil, p.redactor.Error(err)
	}
	return ss, nil
}

// selectLibraries keeps the configured ids (all of which must exist) or, when
// none are configured, every movie and show library.
func selectLibraries(all []Library, ids []string) ([]Library, error) {
	if len(ids) == 0 {
		var out []Library
		for _, l := range all {
			if l.Type == "movie" || l.Type == "show" {
				out = append(out, l)
			}
		}
		return out, nil
	}
	byKey := map[string]Library{}
	for _, l := range all {
		byKey[string(l.Key)] = l
	}
	out := make([]Library, 0, len(ids))
	for _, id := range ids {
		l, ok := byKey[id]
		if !ok {
			return nil, fmt.Errorf("Plex library %s is not on this server", id)
		}
		out = append(out, l)
	}
	return out, nil
}

// Libraries lists every library on the connected server (Settings → Plex
// shows them with the configured ones ticked).
func (p *Provider) Libraries(ctx context.Context) ([]Library, error) {
	p.mu.Lock()
	connected := p.connected
	p.mu.Unlock()
	if !connected {
		return nil, ErrNotConnected
	}
	libs, err := p.client.Libraries(ctx)
	if err != nil {
		return nil, p.redactor.Error(err)
	}
	return libs, nil
}

// Disconnect forgets the token and server state; the next Connect reloads.
func (p *Provider) Disconnect() {
	p.client.SetToken("")
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connected = false
	p.machineID, p.owner, p.libraries = "", "", nil
	p.art = map[string]artRef{}
	p.status, p.message = providers.StatusDisabled, "Plex account is not linked"
}

// SignOut deletes the token from the secret store, purges every artwork
// scope of the connected server, and disconnects. Other servers' caches are
// untouched. Requires a Connect first for the purge to know its owner.
func (p *Provider) SignOut(ctx context.Context) error {
	p.mu.Lock()
	owner := p.owner
	p.mu.Unlock()
	var errs []error
	if err := p.opts.Secrets.Delete(ctx, p.opts.ConnectionRef); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		errs = append(errs, p.redactor.Error(err))
	}
	if owner != "" {
		if err := p.opts.Artwork.PurgeOwner(owner); err != nil {
			errs = append(errs, err)
		}
	}
	p.Disconnect()
	return errors.Join(errs...)
}

// ListSections implements providers.ContentProvider.
func (p *Provider) ListSections(ctx context.Context) ([]providers.SectionDescriptor, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.connected {
		return nil, ErrNotConnected
	}
	out := []providers.SectionDescriptor{{Kind: providers.KindContinueWatching, Title: "Continue Watching"}}
	for _, l := range p.libraries {
		out = append(out, providers.SectionDescriptor{Kind: providers.KindRecentlyAdded, Key: string(l.Key), Title: "Recently Added in " + l.Title})
	}
	return out, nil
}

func (p *Provider) snapshotLibraries() (map[string]bool, []Library, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	allowed := map[string]bool{}
	for _, l := range p.libraries {
		allowed[string(l.Key)] = true
	}
	return allowed, append([]Library(nil), p.libraries...), p.connected
}

// FetchItems implements providers.ContentProvider. Continue Watching and
// Recently Added are filtered to the selected libraries; cursor is the
// numeric page offset and is honored for collections and single-library
// Recently Added.
func (p *Provider) FetchItems(ctx context.Context, sectionKind string, cfg providers.SectionConfig, cursor string) ([]contract.ContentItem, string, error) {
	allowed, libraries, connected := p.snapshotLibraries()
	if !connected {
		return nil, "", ErrNotConnected
	}
	size := p.opts.PageSize
	if cfg.Limit > 0 && cfg.Limit < size {
		size = cfg.Limit
	}
	if len(cfg.LibraryIDs) > 0 {
		filtered := libraries[:0:0]
		allowed = map[string]bool{}
		for _, l := range libraries {
			for _, id := range cfg.LibraryIDs {
				if string(l.Key) == id {
					filtered = append(filtered, l)
					allowed[id] = true
				}
			}
		}
		libraries = filtered
	}
	start := 0
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 0 {
			return nil, "", fmt.Errorf("plex: invalid cursor")
		}
		start = n
	}
	var items []Metadata
	next := ""
	switch sectionKind {
	case providers.KindContinueWatching:
		all, err := p.client.ContinueWatching(ctx)
		if err != nil {
			return nil, "", p.fetchError(sectionKind, err)
		}
		for _, m := range all {
			if m.LibrarySectionID == "" || allowed[string(m.LibrarySectionID)] {
				items = append(items, m)
			}
		}
		if len(items) > size {
			items = items[:size]
		}
	case providers.KindRecentlyAdded:
		if len(libraries) == 1 {
			page, err := p.client.RecentlyAdded(ctx, string(libraries[0].Key), start, size)
			if err != nil {
				return nil, "", p.fetchError(sectionKind, err)
			}
			items = page.Items
			if page.Next() {
				next = strconv.Itoa(page.Offset + len(page.Items))
			}
			break
		}
		for _, l := range libraries {
			page, err := p.client.RecentlyAdded(ctx, string(l.Key), 0, size)
			if err != nil {
				return nil, "", p.fetchError(sectionKind, err)
			}
			items = append(items, page.Items...)
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].AddedAt > items[j].AddedAt })
		if len(items) > size {
			items = items[:size]
		}
	case providers.KindCollection:
		if cfg.Key == "" {
			return nil, "", fmt.Errorf("plex: section %s has no collection key", cfg.ID)
		}
		page, err := p.client.CollectionChildren(ctx, cfg.Key, start, size)
		if err != nil {
			return nil, "", p.fetchError(sectionKind, err)
		}
		items = page.Items
		if page.Next() {
			next = strconv.Itoa(page.Offset + len(page.Items))
		}
	default:
		return nil, "", fmt.Errorf("plex: unsupported section kind %q", sectionKind)
	}
	return p.mapItems(items), next, nil
}

func (p *Provider) mapItems(items []Metadata) []contract.ContentItem {
	p.mu.Lock()
	defer p.mu.Unlock()
	prefix := p.machineID
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	out := make([]contract.ContentItem, 0, len(items))
	for _, m := range items {
		if m.RatingKey == "" {
			continue
		}
		it := contract.ContentItem{
			ID:         "plex:" + prefix + ":" + string(m.RatingKey),
			Title:      displayTitle(m),
			Subtitle:   displaySubtitle(m),
			Artwork:    nil,
			Progress:   progressOf(m),
			OpenAction: providers.OpenApp,
			Demo:       false,
		}
		p.redactor.AddTitle(m.Title)
		if m.GrandparentTitle != "" {
			p.redactor.AddTitle(m.GrandparentTitle)
		}
		if thumb := pickThumb(m); thumb != "" {
			p.art[it.ID] = artRef{thumb: thumb, libraryID: string(m.LibrarySectionID)}
		}
		out = append(out, it)
	}
	return out
}

func displayTitle(m Metadata) string {
	switch m.Type {
	case "episode":
		if m.GrandparentTitle != "" {
			return m.GrandparentTitle
		}
	case "track":
		if m.GrandparentTitle != "" {
			return m.GrandparentTitle + " · " + m.Title
		}
	}
	return m.Title
}

func displaySubtitle(m Metadata) string {
	switch m.Type {
	case "episode":
		s := ""
		if m.ParentIndex > 0 {
			s = "S" + strconv.Itoa(m.ParentIndex)
		}
		if m.Index > 0 {
			if s != "" {
				s += " · "
			}
			s += "E" + strconv.Itoa(m.Index)
		}
		if m.Title != "" {
			if s != "" {
				s += " · "
			}
			s += m.Title
		}
		return s
	case "season":
		return m.ParentTitle
	case "show":
		if m.Year > 0 {
			return "Series · " + strconv.Itoa(m.Year)
		}
		return "Series"
	case "track":
		return m.ParentTitle
	}
	if m.Year > 0 {
		return strconv.Itoa(m.Year)
	}
	return ""
}

// progressOf derives progress only from the server's viewOffset/duration
// pair; anything else is nil so no progress is ever invented.
func progressOf(m Metadata) *float64 {
	if m.ViewOffset <= 0 || m.Duration <= 0 || m.ViewOffset > m.Duration {
		return nil
	}
	v := float64(m.ViewOffset) / float64(m.Duration)
	return &v
}

func pickThumb(m Metadata) string {
	for _, t := range []string{m.Thumb, m.ParentThumb, m.GrandparentThumb, m.Art} {
		if ValidThumb(t) {
			return t
		}
	}
	return ""
}

// ResolveArtwork implements providers.ContentProvider: the transcoded thumb
// is fetched server-side with the token header and stored in the artwork
// cache under the item's library scope.
func (p *Provider) ResolveArtwork(ctx context.Context, item contract.ContentItem) (string, error) {
	p.mu.Lock()
	ref, ok := p.art[item.ID]
	machineID, owner, connected := p.machineID, p.owner, p.connected
	p.mu.Unlock()
	if !ok || !connected {
		return "", nil
	}
	w, h := p.opts.ArtworkWidth, p.opts.ArtworkHeight
	sum := sha256.Sum256([]byte(ref.thumb + "@" + strconv.Itoa(w) + "x" + strconv.Itoa(h)))
	key := hex.EncodeToString(sum[:])
	scope := ScopeKey(machineID, ref.libraryID)
	path, err := p.opts.Artwork.Get(ctx, scope, owner, key, func(ctx context.Context) (string, io.ReadCloser, error) {
		return p.client.Photo(ctx, ref.thumb, w, h)
	})
	if err != nil {
		return "", p.redactor.Error(err)
	}
	return path, nil
}

// ResolveOpenAction implements providers.ContentProvider and always returns
// open_app: selecting an item launches or activates Plex HTPC; the item is
// not opened. play_exact stays withheld until acceptance PLEX-02 records,
// on the target installation, that (1) the installed Plex HTPC build
// accepts a play request for a specific ratingKey through a verified
// channel (its advertised client protocol, a URI, or a timeline command);
// (2) the request names this server's machineIdentifier and this
// ratingKey and opens that exact item, not a search or a hub; (3) a
// viewOffset in the request resumes at that position; (4) the handoff
// works both for a cold launch and for an already-running player; and (5)
// the signed-in Plex HTPC account has access to the item. Until every point
// holds, "Open Plex" is the only honest affordance (spec §7.3).
func (p *Provider) ResolveOpenAction(ctx context.Context, item contract.ContentItem) providers.OpenAction {
	return providers.OpenAction{
		Kind:   providers.OpenApp,
		AppID:  p.opts.AppID,
		Reason: "exact-item playback handoff to Plex HTPC has not been verified (PLEX-02 not run)",
	}
}
