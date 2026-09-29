// Package plexfake is a local stand-in for plex.tv and one Plex Media Server,
// used by the plex connector's tests and by `bear-den-tv dev --dev-plex-fake`
// (docs/operations.md). It serves the handful of endpoints the connector
// calls, on loopback only, with fictional DEMO-titled items and generated
// DEMO posters. Nothing here talks to the real plex.tv or a real server.
package plexfake

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultToken is the account token the fake issues when a PIN is linked.
// It is a fixed test value, not a credential.
const DefaultToken = "fake-plex-token-DEMO-0123456789"

// Library is one library section the fake server offers.
type Library struct {
	Key   string
	Title string
	Type  string // movie | show | artist | photo
}

// Item is one media item. LibraryKey names its library; Kind is the Plex
// type (movie | episode). Titles are fictional and carry "DEMO".
type Item struct {
	RatingKey  string
	LibraryKey string
	Kind       string
	Title      string
	Show       string // episode: the show title
	Season     int
	Episode    int
	Year       int
	DurationMs int64
	OffsetMs   int64 // > 0 puts the item in Continue Watching
	AddedAt    int64
}

// Connection is one address the fake advertises for a resource.
type Connection struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	URI      string `json:"uri"`
	Local    bool   `json:"local"`
	Relay    bool   `json:"relay"`
}

// Resource is one /api/v2/resources entry.
type Resource struct {
	Name             string       `json:"name"`
	ClientIdentifier string       `json:"clientIdentifier"`
	Provides         string       `json:"provides"`
	Owned            bool         `json:"owned"`
	Connections      []Connection `json:"connections"`
}

// Options configures a Fake. Zero values take the documented defaults.
type Options struct {
	// Token is issued when a PIN links; default DefaultToken.
	Token string
	// LinkAfterPolls links a PIN automatically on that poll (dev); 0 means
	// only Link links it.
	LinkAfterPolls int
	// PINTTL is how long a code stays valid; default 15 minutes.
	PINTTL time.Duration
	// Now is the fake's clock for PIN expiry; default time.Now.
	Now func() time.Time
	// MachineID is the server's machineIdentifier; default a fixed demo id.
	MachineID string
	// ServerName is the resource name; default "DEMO Plex Server".
	ServerName string
	// NoContinueWatchingHub makes /hubs/continueWatching answer 404 so the
	// client falls back to /library/onDeck (older servers).
	NoContinueWatchingHub bool
	// Libraries and Items replace the default DEMO catalogue when non-nil.
	Libraries []Library
	Items     []Item
}

// Fake is a running fake plex.tv + Plex Media Server on one loopback HTTP
// server. Safe for concurrent use.
type Fake struct {
	URL string

	srv  *http.Server
	ln   net.Listener
	opts Options

	mu        sync.Mutex
	pins      map[int]*pin
	nextPIN   int
	resources []Resource
	down      bool
	photo     http.HandlerFunc
	requests  []Request
}

// Request is one request the fake received: method, path, and whether the
// token travelled in the header or (never acceptable) the query string.
type Request struct {
	Method       string
	Path         string
	HeaderToken  string
	TokenInQuery bool
}

type pin struct {
	id      int
	code    string
	expires time.Time
	polls   int
	linked  bool
	gone    bool
}

// New starts a fake on 127.0.0.1 (a free port). Close it when done.
func New(opts Options) (*Fake, error) {
	if opts.Token == "" {
		opts.Token = DefaultToken
	}
	if opts.PINTTL <= 0 {
		opts.PINTTL = 15 * time.Minute
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.MachineID == "" {
		opts.MachineID = "demo0machine0identifier0000000001"
	}
	if opts.ServerName == "" {
		opts.ServerName = "DEMO Plex Server"
	}
	if opts.Libraries == nil {
		opts.Libraries = DefaultLibraries()
	}
	if opts.Items == nil {
		opts.Items = DefaultItems()
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	f := &Fake{opts: opts, pins: map[int]*pin{}, nextPIN: 4100, ln: ln}
	f.srv = &http.Server{Handler: http.HandlerFunc(f.serve), ReadHeaderTimeout: 5 * time.Second}
	f.URL = "http://" + ln.Addr().String()
	f.resources = []Resource{f.SelfResource()}
	go func() { _ = f.srv.Serve(ln) }()
	return f, nil
}

// Close stops the server.
func (f *Fake) Close() { _ = f.srv.Close() }

// Token is the token the fake issues and accepts.
func (f *Fake) Token() string { return f.opts.Token }

// MachineID is the fake server's machineIdentifier.
func (f *Fake) MachineID() string { return f.opts.MachineID }

// SelfResource is the resource entry that points at this fake: one local
// http connection to itself, plus a relay the connector must ignore.
func (f *Fake) SelfResource() Resource {
	host, port := hostPort(f.URL)
	return Resource{
		Name: f.opts.ServerName, ClientIdentifier: f.opts.MachineID, Provides: "server", Owned: true,
		Connections: []Connection{
			{Protocol: "https", Address: "203.0.113.9", Port: 443, URI: "https://relay.invalid:443", Local: false, Relay: true},
			{Protocol: "http", Address: host, Port: port, URI: f.URL, Local: true},
		},
	}
}

// SetResources replaces the /api/v2/resources answer.
func (f *Fake) SetResources(rs []Resource) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resources = append([]Resource(nil), rs...)
}

// SetDown makes every Media Server endpoint answer 503 (plex.tv endpoints
// keep working), as when the server is off.
func (f *Fake) SetDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

// SetPhotoHandler overrides /photo/:/transcode (tests: redirects, bad types).
func (f *Fake) SetPhotoHandler(h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.photo = h
}

// Link marks the most recent PIN as linked by the account.
func (f *Fake) Link() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p := f.pins[f.nextPIN-1]; p != nil {
		p.linked = true
	}
}

// ForgetPINs makes every existing PIN answer 404, as plex.tv does once a
// code has been gone for a while.
func (f *Fake) ForgetPINs() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.pins {
		p.gone = true
	}
}

// Requests returns every request received so far.
func (f *Fake) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.requests...)
}

// Count reports how many requests hit path (exact match, no query).
func (f *Fake) Count(path string) int {
	n := 0
	for _, r := range f.Requests() {
		if r.Path == path {
			n++
		}
	}
	return n
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, Request{
		Method: r.Method, Path: r.URL.Path, HeaderToken: r.Header.Get("X-Plex-Token"),
		TokenInQuery: r.URL.Query().Get("X-Plex-Token") != "",
	})
	down := f.down
	photo := f.photo
	f.mu.Unlock()

	p := r.URL.Path
	switch {
	case p == "/api/v2/pins" && r.Method == http.MethodPost:
		f.createPIN(w)
		return
	case strings.HasPrefix(p, "/api/v2/pins/") && r.Method == http.MethodGet:
		f.pollPIN(w, strings.TrimPrefix(p, "/api/v2/pins/"))
		return
	case p == "/api/v2/resources":
		if !f.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		rs := append([]Resource(nil), f.resources...)
		f.mu.Unlock()
		writeJSON(w, rs)
		return
	}

	// Plex Media Server endpoints.
	if down {
		http.Error(w, "server unavailable", http.StatusServiceUnavailable)
		return
	}
	if p == "/identity" {
		writeJSON(w, map[string]any{"MediaContainer": map[string]any{"machineIdentifier": f.opts.MachineID, "version": "1.40.0-DEMO"}})
		return
	}
	if !f.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch {
	case p == "/library/sections":
		var dirs []map[string]any
		for _, l := range f.opts.Libraries {
			dirs = append(dirs, map[string]any{"key": l.Key, "title": l.Title, "type": l.Type})
		}
		writeJSON(w, map[string]any{"MediaContainer": map[string]any{"Directory": dirs}})
	case p == "/hubs/continueWatching":
		if f.opts.NoContinueWatchingHub {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]any{"MediaContainer": map[string]any{"Hub": []any{
			map[string]any{"hubIdentifier": "continueWatching", "Metadata": f.metadata(f.inProgress())},
		}}})
	case p == "/library/onDeck":
		writeJSON(w, map[string]any{"MediaContainer": map[string]any{"Metadata": f.metadata(f.inProgress())}})
	case strings.HasPrefix(p, "/library/sections/") && strings.HasSuffix(p, "/recentlyAdded"):
		key := strings.TrimSuffix(strings.TrimPrefix(p, "/library/sections/"), "/recentlyAdded")
		f.recentlyAdded(w, r, key)
	case p == "/photo/:/transcode":
		if photo != nil {
			photo(w, r)
			return
		}
		f.poster(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *Fake) authorized(r *http.Request) bool {
	return r.Header.Get("X-Plex-Token") == f.opts.Token
}

func (f *Fake) createPIN(w http.ResponseWriter) {
	f.mu.Lock()
	id := f.nextPIN
	f.nextPIN++
	code := fmt.Sprintf("D%03d", id%1000)
	p := &pin{id: id, code: code, expires: f.opts.Now().Add(f.opts.PINTTL)}
	f.pins[id] = p
	f.mu.Unlock()
	writeJSON(w, map[string]any{"id": id, "code": code, "authToken": nil, "expiresAt": p.expires.UTC().Format(time.RFC3339)})
}

func (f *Fake) pollPIN(w http.ResponseWriter, raw string) {
	id, err := strconv.Atoi(raw)
	f.mu.Lock()
	p := f.pins[id]
	if err != nil || p == nil || p.gone {
		f.mu.Unlock()
		http.NotFound(w, nil)
		return
	}
	p.polls++
	if f.opts.LinkAfterPolls > 0 && p.polls >= f.opts.LinkAfterPolls && f.opts.Now().Before(p.expires) {
		p.linked = true
	}
	reply := map[string]any{"id": p.id, "code": p.code, "authToken": nil, "expiresAt": p.expires.UTC().Format(time.RFC3339)}
	if p.linked {
		reply["authToken"] = f.opts.Token
	}
	f.mu.Unlock()
	writeJSON(w, reply)
}

func (f *Fake) inProgress() []Item {
	var out []Item
	for _, it := range f.opts.Items {
		if it.OffsetMs > 0 {
			out = append(out, it)
		}
	}
	return out
}

func (f *Fake) recentlyAdded(w http.ResponseWriter, r *http.Request, key string) {
	found := false
	for _, l := range f.opts.Libraries {
		found = found || l.Key == key
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	var items []Item
	for _, it := range f.opts.Items {
		if it.LibraryKey == key {
			items = append(items, it)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].AddedAt > items[j].AddedAt })
	start, _ := strconv.Atoi(r.URL.Query().Get("X-Plex-Container-Start"))
	size, _ := strconv.Atoi(r.URL.Query().Get("X-Plex-Container-Size"))
	total := len(items)
	if start > total {
		start = total
	}
	end := total
	if size > 0 && start+size < total {
		end = start + size
	}
	page := items[start:end]
	writeJSON(w, map[string]any{"MediaContainer": map[string]any{
		"size": len(page), "totalSize": total, "offset": start, "Metadata": f.metadata(page),
	}})
}

func (f *Fake) metadata(items []Item) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		m := map[string]any{
			"ratingKey":        it.RatingKey,
			"key":              "/library/metadata/" + it.RatingKey,
			"type":             it.Kind,
			"title":            it.Title,
			"year":             it.Year,
			"thumb":            "/library/metadata/" + it.RatingKey + "/thumb/1700000000",
			"duration":         it.DurationMs,
			"addedAt":          it.AddedAt,
			"librarySectionID": it.LibraryKey,
		}
		if it.OffsetMs > 0 {
			m["viewOffset"] = it.OffsetMs
		}
		if it.Kind == "episode" {
			m["grandparentTitle"] = it.Show
			m["parentIndex"] = it.Season
			m["index"] = it.Episode
			m["grandparentThumb"] = "/library/metadata/" + it.RatingKey + "/thumb/1700000000"
		}
		out = append(out, m)
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func hostPort(u string) (string, int) {
	hp := strings.TrimPrefix(strings.TrimPrefix(u, "http://"), "https://")
	i := strings.LastIndexByte(hp, ':')
	if i < 0 {
		return hp, 80
	}
	port, _ := strconv.Atoi(hp[i+1:])
	return hp[:i], port
}

// DefaultLibraries is the DEMO catalogue's libraries: two video libraries and
// a music library the connector must skip by default.
func DefaultLibraries() []Library {
	return []Library{
		{Key: "1", Title: "DEMO Movies", Type: "movie"},
		{Key: "2", Title: "DEMO Shows", Type: "show"},
		{Key: "3", Title: "DEMO Music", Type: "artist"},
	}
}

// DefaultItems is the DEMO catalogue: fictional titles, each marked DEMO.
func DefaultItems() []Item {
	return []Item{
		{RatingKey: "101", LibraryKey: "1", Kind: "movie", Title: "DEMO The Quiet Harbor", Year: 2024, DurationMs: 7_080_000, OffsetMs: 1_274_000, AddedAt: 1_700_000_100},
		{RatingKey: "102", LibraryKey: "1", Kind: "movie", Title: "DEMO Paper Planes", Year: 2023, DurationMs: 5_640_000, OffsetMs: 3_102_000, AddedAt: 1_700_000_300},
		{RatingKey: "103", LibraryKey: "1", Kind: "movie", Title: "DEMO Cedar Lake", Year: 2022, DurationMs: 6_000_000, AddedAt: 1_700_000_500},
		{RatingKey: "104", LibraryKey: "1", Kind: "movie", Title: "DEMO Salt & Stone", Year: 2021, DurationMs: 5_400_000, AddedAt: 1_700_000_700},
		{RatingKey: "201", LibraryKey: "2", Kind: "episode", Title: "The Long Night", Show: "DEMO Northern Lights", Season: 1, Episode: 3, DurationMs: 3_120_000, OffsetMs: 1_310_000, AddedAt: 1_700_000_200},
		{RatingKey: "202", LibraryKey: "2", Kind: "episode", Title: "Wild Garlic", Show: "DEMO Mountain Kitchen", Season: 2, Episode: 7, DurationMs: 1_680_000, OffsetMs: 1_276_000, AddedAt: 1_700_000_400},
		{RatingKey: "203", LibraryKey: "2", Kind: "episode", Title: "Thaw", Show: "DEMO Pine Hollow", Season: 3, Episode: 1, DurationMs: 2_820_000, AddedAt: 1_700_000_600},
		{RatingKey: "301", LibraryKey: "3", Kind: "track", Title: "DEMO Song", DurationMs: 200_000, AddedAt: 1_700_000_800},
	}
}
