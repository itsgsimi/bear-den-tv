// Server: request dispatch, transport refusals and revocation for the LAN
// remote (spec contracts/http.md).

package remote

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	bdtv "bear-den-tv"
)

// Wire limits fixed by contracts/http.md.
const (
	maxBodyBytes      = 64 << 10
	wsMaxMessageBytes = 16 << 10
	claimPerMinute    = 10
	actionsPerSecond  = 30
	wsMessagesPerSec  = 60

	sessionCookieName = "bdtv_session"
	csrfHeader        = "X-BDTV-CSRF"
	transportHeader   = "X-BDTV-Transport"
	cspValue          = "default-src 'self'; img-src 'self' data:; connect-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'"
	webDistDir        = "apps/remote-web/dist"
	sessionCookieAge  = 365 * 24 * time.Hour
)

// Server is the LAN remote HTTP/WebSocket server. It implements http.Handler
// and owns the WebSocket registry so revocations close live sockets.
type Server struct {
	opts      Options
	log       *slog.Logger
	clock     Clock
	https     bool
	hosts     map[string]struct{}
	validator RequestValidator
	static    fs.FS

	claimLimiter  *keyedLimiter
	actionLimiter *keyedLimiter
	conns         *connRegistry

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New validates opts, subscribes to device revocations, and returns a server
// ready to mount. Call Close (or let ListenAndServe return) to release the
// revocation subscription and close every WebSocket.
func New(opts Options) (*Server, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	static, err := fs.Sub(bdtv.WebDist, webDistDir)
	if err != nil {
		return nil, err
	}
	s := &Server{
		opts:      opts,
		log:       opts.Logger,
		clock:     opts.Clock,
		https:     opts.Transport == TransportHTTPS,
		hosts:     map[string]struct{}{},
		validator: opts.Validator,
		static:    static,
		conns:     newConnRegistry(),
	}
	for _, h := range opts.AllowedHosts {
		s.hosts[strings.ToLower(h)] = struct{}{}
	}
	s.claimLimiter = newKeyedLimiter(s.clock, rate.Every(time.Minute/claimPerMinute), claimPerMinute)
	s.actionLimiter = newKeyedLimiter(s.clock, rate.Limit(actionsPerSecond), actionsPerSecond)
	s.ctx, s.cancel = context.WithCancel(context.Background())

	revocations, err := opts.Devices.Revocations(s.ctx)
	if err != nil {
		s.cancel()
		return nil, err
	}
	s.wg.Add(1)
	go s.revocationLoop(revocations)
	return s, nil
}

// Handler returns the server as an http.Handler.
func (s *Server) Handler() http.Handler { return s }

// Close stops the revocation loop and closes every WebSocket with 1001.
func (s *Server) Close() error {
	s.conns.closeAll(websocketStatusGoingAway, "server shutting down")
	s.cancel()
	s.wg.Wait()
	return nil
}

func (s *Server) revocationLoop(ch <-chan string) {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case id, ok := <-ch:
			if !ok {
				return
			}
			s.log.Info("remote: device revoked", "device_id", id)
			s.conns.revoke(id)
		}
	}
}

// requestScope carries per-request facts through the pipeline.
type requestScope struct {
	id           string
	sourceIP     string
	viewer       *Viewer
	csrf         string
	sessionToken string
	params       map[string]string
}

// ServeHTTP runs the middleware order from contracts/http.md: body limit,
// Host allowlist, security headers, cookie auth, CSRF, Origin, permission,
// transport rules, then the handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
	scope := &requestScope{id: newRequestID(), sourceIP: sourceIP(r.RemoteAddr)}
	defer func() {
		attrs := []any{
			"request_id", scope.id,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		}
		if scope.viewer != nil {
			attrs = append(attrs, "device_id", scope.viewer.DeviceID)
		}
		s.log.Info("remote: request", attrs...)
	}()

	if r.ContentLength > maxBodyBytes {
		s.securityHeaders(rec, r)
		writeError(rec, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds 64 KiB")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	if !s.hostAllowed(r.Host) {
		s.securityHeaders(rec, r)
		writeError(rec, http.StatusMisdirectedRequest, "misdirected", "Host header is not an address this TV serves")
		return
	}
	s.securityHeaders(rec, r)

	if !strings.HasPrefix(r.URL.Path, "/api/") {
		s.serveStatic(rec, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/admin/") && !s.https {
		writeError(rec, http.StatusForbidden, "transport_refused", "administration is not available over trusted-LAN HTTP; use HTTPS or the TV")
		return
	}
	rt, params, status := matchRoute(r.Method, r.URL.Path)
	switch status {
	case http.StatusNotFound:
		writeError(rec, http.StatusNotFound, "not_found", "no such endpoint")
		return
	case http.StatusMethodNotAllowed:
		rec.Header().Set("Allow", strings.Join(allowedMethods(r.URL.Path), ", "))
		writeError(rec, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed for this endpoint")
		return
	}
	scope.params = params

	if rt.auth {
		if !s.authenticate(r, scope) {
			s.clearSessionCookie(rec)
			writeError(rec, http.StatusUnauthorized, "unauthorized", "pair this phone with the TV first")
			return
		}
		if rt.mutating && !s.csrfValid(r, scope) {
			writeError(rec, http.StatusForbidden, "csrf_rejected", "missing or incorrect X-BDTV-CSRF header")
			return
		}
	}
	if rt.mutating || rt.origin {
		if !s.originAllowed(r) {
			writeError(rec, http.StatusForbidden, "origin_rejected", "Origin header is missing or not an allowed host")
			return
		}
	}
	if rt.perm != "" && !scope.viewer.Has(rt.perm) {
		writeError(rec, http.StatusForbidden, "forbidden", "this device lacks the "+string(rt.perm)+" permission")
		return
	}
	if msg := s.transportRefusal(rt, r); msg != "" {
		writeError(rec, http.StatusForbidden, "transport_refused", msg)
		return
	}
	rt.handler(s, rec, r, scope)
}

// transportRefusal returns a non-empty message when the trusted-LAN HTTP
// rules forbid the route.
func (s *Server) transportRefusal(rt *route, r *http.Request) string {
	if s.https {
		return ""
	}
	switch rt.kind {
	case kindLayoutWrite:
		if !s.opts.HTTPLayoutEditing {
			return "layout editing over trusted-LAN HTTP is disabled; enable remote.http_layout_editing on the TV or use HTTPS"
		}
	case kindDeviceWrite:
		return "device changes are not allowed over trusted-LAN HTTP; use HTTPS or the TV"
	case kindDiagnostics:
		if !isLoopback(sourceIP(r.RemoteAddr)) {
			return "diagnostics are only available over HTTPS or on the TV itself"
		}
	}
	return ""
}

func (s *Server) securityHeaders(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", cspValue)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set(transportHeader, s.opts.Transport)
	if strings.HasPrefix(r.URL.Path, "/api/") {
		h.Set("Cache-Control", "no-store")
	}
}

func (s *Server) hostAllowed(host string) bool {
	_, ok := s.hosts[strings.ToLower(host)]
	return ok
}

// originPatterns lists allowed origin hosts for websocket.AcceptOptions.
func (s *Server) originPatterns() []string {
	out := make([]string, 0, len(s.hosts))
	for h := range s.hosts {
		out = append(out, h)
	}
	return out
}

// sourceIP strips the port from a connection address; forwarded headers are
// deliberately never consulted.
func sourceIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func isLoopback(ip string) bool {
	parsed := net.ParseIP(strings.TrimSuffix(strings.SplitN(ip, "%", 2)[0], "]"))
	return parsed != nil && parsed.IsLoopback()
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}

// errorBody is the contract error shape {"error","message"}.
type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte(`{"error":"internal","message":"response encoding failed"}`)
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// decodeJSON strictly decodes a request body into v. It reports a 413 for
// oversized bodies and a 400 otherwise, writing the error itself.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds 64 KiB")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not the expected JSON object")
		return false
	}
	if err := dec.Decode(&struct{}{}); err == nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body has trailing data")
		return false
	}
	return true
}

// responseRecorder captures the status for logging and exposes the wrapped
// writer so the WebSocket library can hijack the connection.
type responseRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *responseRecorder) WriteHeader(status int) {
	if !r.wrote {
		r.status = status
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.wrote = true
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController and coder/websocket reach Hijack/Flush.
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Flush forwards to the underlying writer when it supports flushing.
func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
