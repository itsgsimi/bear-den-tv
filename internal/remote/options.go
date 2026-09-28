// Options and seams (clock, validator) for the remote server.

package remote

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"time"

	"bear-den-tv/internal/contract"
)

// Transport names accepted by Options.Transport (contracts/http.md).
const (
	// TransportTrustedLANHTTP is plain HTTP on a trusted home LAN: no Secure
	// cookies, layout writes only with HTTPLayoutEditing, no device writes,
	// no diagnostics off loopback, no admin routes.
	TransportTrustedLANHTTP = "trusted-lan-http"
	// TransportHTTPS is TLS with an owner-supplied certificate; every endpoint
	// is reachable subject to permissions.
	TransportHTTPS = "https"
)

// Clock supplies the time used for rate limiting so tests stay deterministic.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// RequestValidator turns a raw action request body into a typed request,
// rejecting anything outside contracts/action.schema.json. When
// Options.Validator is nil the embedded JSON Schema validator
// (contract.ValidateActionRequest) is used; StrictValidator is a schema-free
// alternative a caller may plug in explicitly.
type RequestValidator interface {
	// ValidateActionRequest returns the decoded request or an error describing
	// the first violation. The error text may be echoed to the requesting
	// device but is never logged.
	ValidateActionRequest(raw []byte) (contract.ActionRequest, error)
}

// ValidatorFunc adapts a plain function to RequestValidator.
type ValidatorFunc func(raw []byte) (contract.ActionRequest, error)

// ValidateActionRequest calls f.
func (f ValidatorFunc) ValidateActionRequest(raw []byte) (contract.ActionRequest, error) {
	return f(raw)
}

// Options configures New.
type Options struct {
	// ThemeAssets serves /themes/<id>/<file> and /themes/_ornaments/<name>.svg
	// (internal/themes.Registry.Assets); nil serves none.
	ThemeAssets fs.FS
	// Backend is the coordinator seam; required.
	Backend Backend
	// Devices is the pairing and session store; required.
	Devices Devices
	// Validator checks raw action requests; nil selects the embedded JSON
	// Schema validator contract.ValidateActionRequest.
	Validator RequestValidator
	// Transport is TransportTrustedLANHTTP or TransportHTTPS and decides the
	// Secure cookie flag, the Origin scheme, and the transport rules.
	Transport string
	// HTTPLayoutEditing permits layout writes over trusted-LAN HTTP
	// (remote.http_layout_editing).
	HTTPLayoutEditing bool
	// AllowedHosts lists every acceptable Host header value: the bound literal
	// host:port values plus remote.allowed_hosts. Required, non-empty.
	AllowedHosts []string
	// DevMode is reported in /api/v1/info so the phone can label the build.
	DevMode bool
	// Logger receives request logs; nil selects slog.Default().
	Logger *slog.Logger
	// Clock drives rate limiting; nil selects the wall clock.
	Clock Clock
	// IdleTimeout closes a WebSocket whose peer stops answering server pings;
	// zero selects 60 seconds.
	IdleTimeout time.Duration
	// ShutdownTimeout bounds graceful HTTP shutdown in ListenAndServe; zero
	// selects 5 seconds.
	ShutdownTimeout time.Duration
}

// validate fills defaults and reports misconfiguration.
func (o *Options) validate() error {
	if o.Backend == nil {
		return errors.New("remote: Options.Backend is required")
	}
	if o.Devices == nil {
		return errors.New("remote: Options.Devices is required")
	}
	switch o.Transport {
	case TransportTrustedLANHTTP, TransportHTTPS:
	default:
		return fmt.Errorf("remote: Options.Transport must be %q or %q, got %q", TransportTrustedLANHTTP, TransportHTTPS, o.Transport)
	}
	if len(o.AllowedHosts) == 0 {
		return errors.New("remote: Options.AllowedHosts must list at least one host:port")
	}
	for _, h := range o.AllowedHosts {
		if strings.TrimSpace(h) == "" || strings.Contains(h, "/") {
			return fmt.Errorf("remote: Options.AllowedHosts entry %q must be a bare host:port", h)
		}
	}
	if o.Validator == nil {
		o.Validator = ValidatorFunc(contract.ValidateActionRequest)
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Clock == nil {
		o.Clock = realClock{}
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = 60 * time.Second
	}
	if o.ShutdownTimeout <= 0 {
		o.ShutdownTimeout = 5 * time.Second
	}
	return nil
}
