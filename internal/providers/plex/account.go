// Plex account: PIN sign-in and server discovery over plex.tv
// (docs/security.md).

package plex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// DefaultAccountURL is the plex.tv API origin.
const DefaultAccountURL = "https://plex.tv"

// LinkURL is where the user types the code shown on the TV.
const LinkURL = "https://plex.tv/link"

// ErrPINExpired reports that the link code is no longer valid; start a new one.
var ErrPINExpired = errors.New("plex: link code expired")

// AccountOptions configures plex.tv access. Strong selects the long code
// used with app.plex.tv/auth; the default (false) yields the 4-character
// code the user types at plex.tv/link.
type AccountOptions struct {
	BaseURL          string
	ClientIdentifier string
	Transport        http.RoundTripper
	RequestTimeout   time.Duration
	Strong           bool
	// Now is the clock PIN expiry is judged by; nil uses time.Now.
	Now func() time.Time
}

// Account is the plex.tv side of sign-in: PIN creation and polling, then
// server discovery for the linked account. No password ever passes through
// this process; the token is returned to the caller for storage in
// internal/secrets.
type Account struct {
	base     *url.URL
	http     *http.Client
	clientID string
	strong   bool
	maxJSON  int64
	now      func() time.Time
}

// PIN is a pending link code.
type PIN struct {
	ID        int
	Code      string
	ExpiresAt time.Time
}

// Server is one Plex Media Server the account can reach.
type Server struct {
	Name              string
	MachineIdentifier string
	Owned             bool
	// Connections are ordered local before remote, then https before http;
	// relay connections are excluded.
	Connections []Connection
}

// Connection is one address of a Server.
type Connection struct {
	URI      string
	Protocol string
	Address  string
	Port     int
	Local    bool
	Relay    bool
}

// NewAccount validates the base URL and builds the plex.tv client.
func NewAccount(opts AccountOptions) (*Account, error) {
	raw := opts.BaseURL
	if raw == "" {
		raw = DefaultAccountURL
	}
	base, err := ParseServerURL(raw)
	if err != nil {
		return nil, err
	}
	if opts.ClientIdentifier == "" {
		return nil, errors.New("plex: client identifier is required")
	}
	timeout := opts.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	transport := opts.Transport
	if transport == nil {
		transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: DefaultConnectTimeout}).DialContext,
			TLSHandshakeTimeout:   DefaultConnectTimeout,
			ResponseHeaderTimeout: timeout,
		}
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Account{
		now:      now,
		base:     base,
		clientID: opts.ClientIdentifier,
		strong:   opts.Strong,
		maxJSON:  MaxJSONBytes,
		http: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects || req.URL.Scheme != base.Scheme || req.URL.Host != base.Host {
				return ErrRedirectRefused
			}
			return nil
		}},
	}, nil
}

func (a *Account) request(ctx context.Context, method, p string, query url.Values, token string) (*http.Request, error) {
	u := *a.base
	u.Path = a.base.Path + p
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", Product)
	req.Header.Set("X-Plex-Version", Version)
	req.Header.Set("X-Plex-Platform", Platform)
	req.Header.Set("X-Plex-Device-Name", Product)
	req.Header.Set("X-Plex-Client-Identifier", a.clientID)
	if token != "" {
		req.Header.Set("X-Plex-Token", token)
	}
	return req, nil
}

func (a *Account) doJSON(ctx context.Context, method, p string, query url.Values, token string, out any) error {
	req, err := a.request(ctx, method, p, query, token)
	if err != nil {
		return err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return requestError(p, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return &StatusError{Status: resp.StatusCode, Path: p}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, a.maxJSON+1))
	if err != nil {
		return requestError(p, err)
	}
	if int64(len(data)) > a.maxJSON {
		return fmt.Errorf("%w: %s", ErrResponseTooLarge, p)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("plex: decode %s: %w", p, err)
	}
	return nil
}

type pinReply struct {
	ID        int    `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
	ExpiresAt string `json:"expiresAt"`
}

func (r pinReply) expires() time.Time {
	t, err := time.Parse(time.RFC3339, r.ExpiresAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

// StartPIN is POST /api/v2/pins. The TV shows PIN.Code and LinkURL; the user
// links from a browser, then the caller polls with PollPIN.
func (a *Account) StartPIN(ctx context.Context) (PIN, error) {
	q := url.Values{}
	q.Set("strong", fmt.Sprint(a.strong))
	var reply pinReply
	if err := a.doJSON(ctx, http.MethodPost, "/api/v2/pins", q, "", &reply); err != nil {
		return PIN{}, err
	}
	if reply.ID == 0 || reply.Code == "" {
		return PIN{}, errors.New("plex: pin reply has no id or code")
	}
	return PIN{ID: reply.ID, Code: reply.Code, ExpiresAt: reply.expires()}, nil
}

// PollPIN is GET /api/v2/pins/{id}. done is true when the account linked
// the code and token holds the account token. ErrPINExpired ends polling.
func (a *Account) PollPIN(ctx context.Context, id int) (token string, done bool, err error) {
	var reply pinReply
	if err := a.doJSON(ctx, http.MethodGet, "/api/v2/pins/"+itoa(id), nil, "", &reply); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Status == http.StatusNotFound {
			return "", false, ErrPINExpired
		}
		return "", false, err
	}
	if reply.AuthToken != "" {
		return reply.AuthToken, true, nil
	}
	if exp := reply.expires(); !exp.IsZero() && !a.now().Before(exp) {
		return "", false, ErrPINExpired
	}
	return "", false, nil
}

type resourceReply struct {
	Name             string `json:"name"`
	ClientIdentifier string `json:"clientIdentifier"`
	Provides         string `json:"provides"`
	Owned            bool   `json:"owned"`
	Connections      []struct {
		Protocol string `json:"protocol"`
		Address  string `json:"address"`
		Port     int    `json:"port"`
		URI      string `json:"uri"`
		Local    bool   `json:"local"`
		Relay    bool   `json:"relay"`
	} `json:"connections"`
}

// DiscoverServers is GET /api/v2/resources?includeHttps=1&includeRelay=0
// for the account token: every resource that provides "server", with its
// non-relay connections ordered local first, then https before http (the
// order a caller should try them in).
func (a *Account) DiscoverServers(ctx context.Context, token string) ([]Server, error) {
	if token == "" {
		return nil, errors.New("plex: token is required for discovery")
	}
	q := url.Values{}
	q.Set("includeHttps", "1")
	q.Set("includeRelay", "0")
	var reply []resourceReply
	if err := a.doJSON(ctx, http.MethodGet, "/api/v2/resources", q, token, &reply); err != nil {
		return nil, err
	}
	var servers []Server
	for _, r := range reply {
		if !providesServer(r.Provides) {
			continue
		}
		s := Server{Name: r.Name, MachineIdentifier: r.ClientIdentifier, Owned: r.Owned}
		for _, c := range r.Connections {
			if c.Relay {
				continue
			}
			conn := Connection{URI: c.URI, Protocol: c.Protocol, Address: c.Address, Port: c.Port, Local: c.Local, Relay: c.Relay}
			if _, err := ParseServerURL(conn.URI); err != nil {
				continue
			}
			s.Connections = append(s.Connections, conn)
		}
		sort.SliceStable(s.Connections, func(i, j int) bool {
			return connectionRank(s.Connections[i]) < connectionRank(s.Connections[j])
		})
		servers = append(servers, s)
	}
	return servers, nil
}

// connectionRank orders connections: local https, local http, remote https,
// remote http.
func connectionRank(c Connection) int {
	rank := 0
	if !c.Local {
		rank += 2
	}
	if !strings.HasPrefix(strings.ToLower(c.URI), "https://") {
		rank++
	}
	return rank
}

func providesServer(provides string) bool {
	for _, p := range strings.Split(provides, ",") {
		if strings.TrimSpace(p) == "server" {
			return true
		}
	}
	return false
}
