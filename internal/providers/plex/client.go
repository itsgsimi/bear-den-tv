// Package plex is the optional Plex content connector: a bounded HTTP client
// for one Plex Media Server, plex.tv PIN sign-in and server discovery, an
// artwork disk cache with a byte budget, and the providers.ContentProvider
// that maps Continue Watching, Recently Added, and collections into
// contract.Content. The account token comes from internal/secrets by
// connection_ref; it travels only in the X-Plex-Token header, never in a URL,
// log line, or config file (spec §7.2, §9.3, §9.4).
package plex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// Client identification sent with every request.
const (
	Product  = "Bear Den TV"
	Version  = "0.1"
	Platform = "Linux"
)

// Bounds every Client applies.
const (
	// DefaultConnectTimeout bounds TCP connect and TLS handshake.
	DefaultConnectTimeout = 5 * time.Second
	// DefaultRequestTimeout bounds one attempt end to end.
	DefaultRequestTimeout = 10 * time.Second
	// MaxJSONBytes bounds a decoded JSON response body.
	MaxJSONBytes = 8 << 20
	// DefaultRetries is how many times a failed idempotent request is retried.
	DefaultRetries = 2
	maxRedirects   = 3
)

var (
	// ErrInvalidServerURL reports a server_url that is not http(s), lacks a
	// host, or carries userinfo, query, or fragment.
	ErrInvalidServerURL = errors.New("plex: invalid server URL")
	// ErrRedirectRefused reports a redirect whose destination differs from the
	// configured server (scheme or host), the SSRF guard from spec §9.4.
	ErrRedirectRefused = errors.New("plex: redirect to another host refused")
	// ErrResponseTooLarge reports a JSON body above MaxJSONBytes.
	ErrResponseTooLarge = errors.New("plex: response exceeds size limit")
	// ErrInvalidArtworkPath reports a thumb path that is not server-relative.
	ErrInvalidArtworkPath = errors.New("plex: invalid artwork path")
	// ErrNotImage reports artwork whose Content-Type is not an image.
	ErrNotImage = errors.New("plex: artwork is not an image")
)

// StatusError is a non-2xx reply. Path never includes the query string.
type StatusError struct {
	Status int
	Path   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("plex: server returned %d for %s", e.Status, e.Path)
}

// ParseServerURL validates plex_content.server_url: http or https, a host,
// optional path prefix, and no userinfo, query, or fragment.
func ParseServerURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidServerURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: scheme must be http or https", ErrInvalidServerURL)
	}
	if u.Host == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("%w: missing host", ErrInvalidServerURL)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: userinfo is not allowed", ErrInvalidServerURL)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, fmt.Errorf("%w: query and fragment are not allowed", ErrInvalidServerURL)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u, nil
}

// ClientOptions configures a Client. Zero durations and counts take the
// package defaults; Transport nil builds one with the timeouts.
type ClientOptions struct {
	ServerURL        string
	ClientIdentifier string
	Transport        http.RoundTripper
	ConnectTimeout   time.Duration
	RequestTimeout   time.Duration
	Retries          int
	// Backoff returns the pause before retry attempt n (0-based); nil uses a
	// jittered exponential backoff starting at 250 ms.
	Backoff func(attempt int) time.Duration
}

// Client talks to one Plex Media Server with explicit timeouts, bounded
// retries, bounded JSON decoding, and a same-origin redirect policy. The
// token is held in memory only and sent as a header. Safe for concurrent use.
type Client struct {
	base     *url.URL
	http     *http.Client
	clientID string
	retries  int
	backoff  func(attempt int) time.Duration
	maxJSON  int64

	mu    sync.RWMutex
	token string
}

// NewClient validates the server URL and builds the client.
func NewClient(opts ClientOptions) (*Client, error) {
	base, err := ParseServerURL(opts.ServerURL)
	if err != nil {
		return nil, err
	}
	if opts.ClientIdentifier == "" {
		return nil, errors.New("plex: client identifier is required")
	}
	connect := opts.ConnectTimeout
	if connect <= 0 {
		connect = DefaultConnectTimeout
	}
	request := opts.RequestTimeout
	if request <= 0 {
		request = DefaultRequestTimeout
	}
	transport := opts.Transport
	if transport == nil {
		transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: connect}).DialContext,
			TLSHandshakeTimeout:   connect,
			ResponseHeaderTimeout: request,
			MaxIdleConns:          4,
			IdleConnTimeout:       90 * time.Second,
		}
	}
	c := &Client{base: base, clientID: opts.ClientIdentifier, retries: opts.Retries, backoff: opts.Backoff, maxJSON: MaxJSONBytes}
	if opts.Retries < 0 {
		c.retries = 0
	} else if opts.Retries == 0 {
		c.retries = DefaultRetries
	}
	if c.backoff == nil {
		c.backoff = defaultBackoff
	}
	c.http = &http.Client{
		Transport: transport,
		Timeout:   request,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("%w: too many redirects", ErrRedirectRefused)
			}
			if req.URL.Scheme != base.Scheme || req.URL.Host != base.Host {
				return ErrRedirectRefused
			}
			return nil
		},
	}
	return c, nil
}

func defaultBackoff(attempt int) time.Duration {
	base := 250 * time.Millisecond << uint(attempt)
	return base + rand.N(250*time.Millisecond)
}

// SetToken installs the account token used for every later request.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

// Host is the configured server host[:port].
func (c *Client) Host() string { return c.base.Host }

func (c *Client) newRequest(ctx context.Context, p string, query url.Values, accept string) (*http.Request, error) {
	u := *c.base
	u.Path = path.Join(c.base.Path, p)
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-Plex-Product", Product)
	req.Header.Set("X-Plex-Version", Version)
	req.Header.Set("X-Plex-Platform", Platform)
	req.Header.Set("X-Plex-Device-Name", Product)
	req.Header.Set("X-Plex-Client-Identifier", c.clientID)
	c.mu.RLock()
	if c.token != "" {
		req.Header.Set("X-Plex-Token", c.token)
	}
	c.mu.RUnlock()
	return req, nil
}

func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// do performs a GET with retries. A returned response has a live body the
// caller must close. Errors name the path, never the query or headers.
func (c *Client) do(ctx context.Context, p string, query url.Values, accept string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoff(attempt - 1)):
			}
		}
		req, err := c.newRequest(ctx, p, query, accept)
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = requestError(p, err)
			if ctx.Err() != nil || errors.Is(err, ErrRedirectRefused) {
				return nil, lastErr
			}
			continue
		}
		if retryable(resp.StatusCode) {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = &StatusError{Status: resp.StatusCode, Path: p}
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

// requestError strips the URL (which may carry a query) from transport
// errors while keeping the cause for errors.Is.
func requestError(p string, err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		err = uerr.Err
	}
	return fmt.Errorf("plex: request %s: %w", p, err)
}

// getJSON performs a GET expecting a 2xx JSON body of at most MaxJSONBytes.
func (c *Client) getJSON(ctx context.Context, p string, query url.Values, out any) error {
	resp, err := c.do(ctx, p, query, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return &StatusError{Status: resp.StatusCode, Path: p}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxJSON+1))
	if err != nil {
		return requestError(p, err)
	}
	if int64(len(data)) > c.maxJSON {
		return fmt.Errorf("%w: %s", ErrResponseTooLarge, p)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("plex: decode %s: %w", p, err)
	}
	return nil
}

// Identity is GET /identity: the server's machineIdentifier and version.
func (c *Client) Identity(ctx context.Context) (Identity, error) {
	var env identityEnvelope
	if err := c.getJSON(ctx, "/identity", nil, &env); err != nil {
		return Identity{}, err
	}
	if env.MediaContainer.MachineIdentifier == "" {
		return Identity{}, errors.New("plex: /identity has no machineIdentifier")
	}
	return env.MediaContainer, nil
}

// Libraries is GET /library/sections.
func (c *Client) Libraries(ctx context.Context) ([]Library, error) {
	var env librariesEnvelope
	if err := c.getJSON(ctx, "/library/sections", nil, &env); err != nil {
		return nil, err
	}
	return env.MediaContainer.Directory, nil
}

// ContinueWatching is GET /hubs/continueWatching with a fallback to
// GET /library/onDeck when the server has no such hub (404).
func (c *Client) ContinueWatching(ctx context.Context) ([]Metadata, error) {
	var hubs hubsEnvelope
	err := c.getJSON(ctx, "/hubs/continueWatching", nil, &hubs)
	if err == nil {
		var items []Metadata
		for _, hub := range hubs.MediaContainer.Hub {
			items = append(items, hub.Metadata...)
		}
		return items, nil
	}
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusNotFound {
		return nil, err
	}
	var env metadataEnvelope
	if err := c.getJSON(ctx, "/library/onDeck", nil, &env); err != nil {
		return nil, err
	}
	return env.MediaContainer.Metadata, nil
}

func pageQuery(start, size int) url.Values {
	q := url.Values{}
	q.Set("X-Plex-Container-Start", itoa(start))
	q.Set("X-Plex-Container-Size", itoa(size))
	return q
}

func (c *Client) page(ctx context.Context, p string, start, size int) (Page, error) {
	var env metadataEnvelope
	if err := c.getJSON(ctx, p, pageQuery(start, size), &env); err != nil {
		return Page{}, err
	}
	mc := env.MediaContainer
	return Page{Items: mc.Metadata, Offset: mc.Offset, Size: mc.Size, Total: mc.TotalSize}, nil
}

// RecentlyAdded is GET /library/sections/{id}/recentlyAdded, one page.
func (c *Client) RecentlyAdded(ctx context.Context, libraryID string, start, size int) (Page, error) {
	if !safeSegment(libraryID) {
		return Page{}, fmt.Errorf("plex: invalid library id")
	}
	return c.page(ctx, "/library/sections/"+libraryID+"/recentlyAdded", start, size)
}

// CollectionChildren is GET /library/collections/{id}/children, one page.
func (c *Client) CollectionChildren(ctx context.Context, collectionID string, start, size int) (Page, error) {
	if !safeSegment(collectionID) {
		return Page{}, fmt.Errorf("plex: invalid collection id")
	}
	return c.page(ctx, "/library/collections/"+collectionID+"/children", start, size)
}

func safeSegment(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// ValidThumb reports whether thumb is a server-relative path the photo
// transcoder may be asked for: absolute path, no scheme, no "//" prefix.
func ValidThumb(thumb string) bool {
	if !strings.HasPrefix(thumb, "/") || strings.HasPrefix(thumb, "//") {
		return false
	}
	if strings.Contains(thumb, "://") || strings.ContainsAny(thumb, "\n\r") {
		return false
	}
	return true
}

// Photo is GET /photo/:/transcode for a server-relative thumb resized to fit
// width×height. The body is the image bytes (caller closes; caller bounds
// size and dimensions). Non-image replies are refused.
func (c *Client) Photo(ctx context.Context, thumb string, width, height int) (contentType string, body io.ReadCloser, err error) {
	if !ValidThumb(thumb) {
		return "", nil, ErrInvalidArtworkPath
	}
	q := url.Values{}
	q.Set("width", itoa(width))
	q.Set("height", itoa(height))
	q.Set("minSize", "1")
	q.Set("upscale", "0")
	q.Set("url", thumb)
	resp, err := c.do(ctx, "/photo/:/transcode", q, "image/*")
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode/100 != 2 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return "", nil, &StatusError{Status: resp.StatusCode, Path: "/photo/:/transcode"}
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(ct), "image/") {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return "", nil, ErrNotImage
	}
	return ct, resp.Body, nil
}
