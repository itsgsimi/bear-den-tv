// The HTTP route table and handlers (spec contracts/http.md).

package remote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode"

	"bear-den-tv/internal/contract"
)

// routeKind selects the transport rule applied under trusted-LAN HTTP.
type routeKind int

const (
	kindPlain routeKind = iota
	kindLayoutWrite
	kindDeviceWrite
	kindDiagnostics
)

// route describes one API endpoint and the checks that precede its handler.
type route struct {
	method   string
	pattern  string
	auth     bool                // cookie session required
	mutating bool                // CSRF (when auth) and Origin required
	origin   bool                // Origin required without CSRF (WebSocket upgrade)
	perm     contract.Permission // minimum permission; "" for none
	kind     routeKind
	handler  func(s *Server, w http.ResponseWriter, r *http.Request, scope *requestScope)
}

var routes = []route{
	{method: http.MethodGet, pattern: "/api/v1/info", handler: (*Server).handleInfo},
	{method: http.MethodPost, pattern: "/api/v1/pair/claim", mutating: true, handler: (*Server).handleClaim},
	{method: http.MethodGet, pattern: "/api/v1/session", auth: true, handler: (*Server).handleSession},
	{method: http.MethodPost, pattern: "/api/v1/logout", auth: true, mutating: true, handler: (*Server).handleLogout},
	{method: http.MethodGet, pattern: "/api/v1/state", auth: true, handler: (*Server).handleState},
	{method: http.MethodGet, pattern: "/api/v1/capabilities", auth: true, handler: (*Server).handleCapabilities},
	{method: http.MethodPost, pattern: "/api/v1/actions", auth: true, mutating: true, handler: (*Server).handleActions},
	{method: http.MethodGet, pattern: "/api/v1/events", auth: true, origin: true, handler: (*Server).handleEvents},
	{method: http.MethodGet, pattern: "/api/v1/layout", auth: true, perm: contract.PermLayoutEditor, handler: (*Server).handleLayoutGet},
	{method: http.MethodPut, pattern: "/api/v1/layout", auth: true, mutating: true, perm: contract.PermLayoutEditor, kind: kindLayoutWrite, handler: (*Server).handleLayoutPut},
	{method: http.MethodPost, pattern: "/api/v1/layout/preview", auth: true, mutating: true, perm: contract.PermLayoutEditor, kind: kindLayoutWrite, handler: (*Server).handleLayoutPreview},
	{method: http.MethodPost, pattern: "/api/v1/layout/preview/end", auth: true, mutating: true, perm: contract.PermLayoutEditor, kind: kindLayoutWrite, handler: (*Server).handleLayoutPreviewEnd},
	{method: http.MethodPost, pattern: "/api/v1/layout/confirm", auth: true, mutating: true, perm: contract.PermLayoutEditor, kind: kindLayoutWrite, handler: (*Server).handleLayoutConfirm},
	{method: http.MethodPost, pattern: "/api/v1/layout/cancel", auth: true, mutating: true, perm: contract.PermLayoutEditor, kind: kindLayoutWrite, handler: (*Server).handleLayoutCancel},
	{method: http.MethodPost, pattern: "/api/v1/layout/undo", auth: true, mutating: true, perm: contract.PermLayoutEditor, kind: kindLayoutWrite, handler: (*Server).handleLayoutUndo},
	{method: http.MethodPost, pattern: "/api/v1/layout/reset", auth: true, mutating: true, perm: contract.PermLayoutEditor, kind: kindLayoutWrite, handler: (*Server).handleLayoutReset},
	{method: http.MethodGet, pattern: "/api/v1/devices", auth: true, perm: contract.PermOwner, handler: (*Server).handleDevicesList},
	{method: http.MethodDelete, pattern: "/api/v1/devices/{id}", auth: true, mutating: true, kind: kindDeviceWrite, handler: (*Server).handleDeviceDelete},
	{method: http.MethodGet, pattern: "/api/v1/diagnostics", auth: true, perm: contract.PermOwner, kind: kindDiagnostics, handler: (*Server).handleDiagnostics},
	// Guest passes too: they see the same tiles (contracts/http.md#app-icons).
	{method: http.MethodGet, pattern: "/api/v1/apps/{adapter}/icon", auth: true, handler: (*Server).handleAppIcon},
}

// matchRoute returns the route for method+path, or a 404/405 status.
func matchRoute(method, p string) (*route, map[string]string, int) {
	pathMatched := false
	for i := range routes {
		rt := &routes[i]
		params, ok := matchPattern(rt.pattern, p)
		if !ok {
			continue
		}
		pathMatched = true
		if rt.method == method {
			return rt, params, 0
		}
	}
	if pathMatched {
		return nil, nil, http.StatusMethodNotAllowed
	}
	return nil, nil, http.StatusNotFound
}

func allowedMethods(p string) []string {
	var out []string
	for i := range routes {
		if _, ok := matchPattern(routes[i].pattern, p); ok {
			out = append(out, routes[i].method)
		}
	}
	return out
}

// matchPattern matches a path against a pattern whose "{name}" segments
// capture exactly one non-empty segment.
func matchPattern(pattern, p string) (map[string]string, bool) {
	ps := strings.Split(pattern, "/")
	rs := strings.Split(p, "/")
	if len(ps) != len(rs) {
		return nil, false
	}
	var params map[string]string
	for i, seg := range ps {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			if rs[i] == "" {
				return nil, false
			}
			if params == nil {
				params = map[string]string{}
			}
			params[seg[1:len(seg)-1]] = rs[i]
			continue
		}
		if seg != rs[i] {
			return nil, false
		}
	}
	return params, true
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request, _ *requestScope) {
	snap := s.opts.Backend.Snapshot(r.Context(), nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"protocol":         contract.Protocol,
		"device_name":      snap.DeviceName,
		"transport":        s.opts.Transport,
		"https":            s.https,
		"pairing_required": true,
		"dev_mode":         s.opts.DevMode,
	})
}

type claimRequest struct {
	Invitation *string `json:"invitation"`
	Code       *string `json:"code"`
	DeviceName string  `json:"device_name"`
}

func validDeviceName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return "", false
	}
	for _, c := range name {
		if unicode.IsControl(c) {
			return "", false
		}
	}
	return name, true
}

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	if !s.claimLimiter.allow(scope.sourceIP) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many pairing attempts from this address; wait a minute")
		return
	}
	var req claimRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name, ok := validDeviceName(req.DeviceName)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid", "device_name must be 1-64 printable characters")
		return
	}
	var token, code string
	if req.Invitation != nil {
		token = *req.Invitation
	}
	if req.Code != nil {
		code = *req.Code
	}
	if token == "" && code == "" {
		writeError(w, http.StatusBadRequest, "invalid", "provide an invitation token or the code shown on the TV")
		return
	}
	res, err := s.opts.Devices.Claim(r.Context(), token, code, name, scope.sourceIP)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvitationExpired):
			writeError(w, http.StatusGone, "invitation_expired", "this invitation has expired; show a new one on the TV")
		case errors.Is(err, ErrTooManyAttempts):
			writeError(w, http.StatusTooManyRequests, "too_many_attempts", "too many failed attempts for this invitation; show a new one on the TV")
		case errors.Is(err, ErrInvalidInvitation):
			writeError(w, http.StatusUnauthorized, "invalid_invitation", "the invitation or code is not valid")
		default:
			s.log.Warn("remote: pair/claim failed", "request_id", scope.id, "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "pairing failed")
		}
		return
	}
	s.setSessionCookie(w, res.SessionToken)
	s.log.Info("remote: device paired", "request_id", scope.id, "device_id", res.DeviceID)
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":   res.DeviceID,
		"device_name": res.DeviceName,
		"permissions": permissionsOrEmpty(res.Permissions),
		"csrf_token":  res.CSRFToken,
	})
}

func permissionsOrEmpty(p []contract.Permission) []contract.Permission {
	if p == nil {
		return []contract.Permission{}
	}
	return p
}

func (s *Server) handleSession(w http.ResponseWriter, _ *http.Request, scope *requestScope) {
	v := scope.viewer
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":        v.DeviceID,
		"device_name":      v.DeviceName,
		"permissions":      permissionsOrEmpty(v.Permissions),
		"csrf_token":       scope.csrf,
		"transport_secure": s.https,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	if err := s.opts.Devices.Logout(r.Context(), scope.sessionToken); err != nil {
		s.log.Warn("remote: logout failed", "request_id", scope.id, "device_id", scope.viewer.DeviceID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "logout failed")
		return
	}
	s.conns.closeSession(scope.sessionToken, websocketStatusRevoked, "session ended")
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	writeJSON(w, http.StatusOK, s.opts.Backend.Snapshot(r.Context(), scope.viewer))
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	snap := s.opts.Backend.Snapshot(r.Context(), scope.viewer)
	caps := snap.Capabilities
	if caps == nil {
		caps = map[string]contract.Capability{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"context_epoch": snap.ContextEpoch,
		"target":        snap.Target,
		"capabilities":  caps,
	})
}

func (s *Server) handleActions(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds 64 KiB")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "request body could not be read")
		return
	}
	if !json.Valid(raw) {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid JSON")
		return
	}
	v := *scope.viewer
	res := s.submitRaw(r.Context(), v, raw, func() bool { return s.actionLimiter.allow(v.DeviceID) })
	writeJSON(w, http.StatusOK, res)
}

// submitRaw is the shared HTTP/WebSocket action path: rate limit, protocol
// check, validation, then Backend.Submit. Every failure is an action result.
func (s *Server) submitRaw(ctx context.Context, v Viewer, raw []byte, allow func() bool) contract.ActionResult {
	var head struct {
		RequestID string           `json:"request_id"`
		Protocol  *json.RawMessage `json:"protocol"`
	}
	_ = json.Unmarshal(raw, &head)
	stub := contract.ActionRequest{RequestID: head.RequestID}
	if !isUUID(stub.RequestID) {
		stub.RequestID = "00000000-0000-0000-0000-000000000000"
	}
	fail := func(code contract.Code, msg string) contract.ActionResult {
		snap := s.opts.Backend.Snapshot(ctx, &v)
		return contract.Failed(stub, snap.ContextEpoch, snap.Target.Info(), code, msg)
	}
	if !allow() {
		return fail(contract.CodeRateLimited, "too many actions; slow down")
	}
	if head.Protocol != nil && string(*head.Protocol) != "1" {
		return fail(contract.CodeUnsupportedProtocol, "only protocol 1 is supported")
	}
	req, err := s.validator.ValidateActionRequest(raw)
	if err != nil {
		if errors.Is(err, ErrUnsupportedProtocol) {
			return fail(contract.CodeUnsupportedProtocol, "only protocol 1 is supported")
		}
		return fail(contract.CodeInvalid, truncate(err.Error(), 512))
	}
	return s.opts.Backend.Submit(ctx, v, req)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (s *Server) handleLayoutGet(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	view, err := s.opts.Backend.Layout(r.Context(), *scope.viewer)
	if err != nil {
		s.writeBackendError(w, scope, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleLayoutPut(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	var body struct {
		BaseRevision int64            `json:"base_revision"`
		Layout       *contract.Layout `json:"layout"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Layout == nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid", "layout is required")
		return
	}
	res, err := s.opts.Backend.PutLayout(r.Context(), *scope.viewer, body.BaseRevision, *body.Layout)
	if err != nil {
		if errors.Is(err, ErrRevisionConflict) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":            "revision_conflict",
				"message":          "the layout changed since you loaded it; reload and retry",
				"current_revision": res.Revision,
			})
			return
		}
		s.writeBackendError(w, scope, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleLayoutPreview(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	var body struct {
		Layout *contract.Layout `json:"layout"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Layout == nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid", "layout is required")
		return
	}
	if err := s.opts.Backend.PreviewLayout(r.Context(), *scope.viewer, *body.Layout); err != nil {
		s.writeBackendError(w, scope, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleLayoutPreviewEnd(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	if err := s.opts.Backend.EndPreview(r.Context(), *scope.viewer); err != nil {
		s.writeBackendError(w, scope, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type revisionBody struct {
	Revision int64 `json:"revision"`
}

func (s *Server) handleLayoutConfirm(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	var body revisionBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.opts.Backend.ConfirmLayout(r.Context(), *scope.viewer, body.Revision); err != nil {
		s.writeBackendError(w, scope, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleLayoutCancel(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	var body revisionBody
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.opts.Backend.CancelLayout(r.Context(), *scope.viewer, body.Revision); err != nil {
		s.writeBackendError(w, scope, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleLayoutUndo(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	rev, err := s.opts.Backend.UndoLayout(r.Context(), *scope.viewer)
	if err != nil {
		s.writeBackendError(w, scope, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revision": rev})
}

func (s *Server) handleLayoutReset(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	var body struct {
		SectionID string `json:"section_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	rev, err := s.opts.Backend.ResetLayout(r.Context(), *scope.viewer, body.SectionID)
	if err != nil {
		s.writeBackendError(w, scope, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revision": rev})
}

func (s *Server) handleDevicesList(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	list, err := s.opts.Devices.List(r.Context())
	if err != nil {
		s.log.Warn("remote: device list failed", "request_id", scope.id, "device_id", scope.viewer.DeviceID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "device list unavailable")
		return
	}
	if list == nil {
		list = []contract.Device{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": list})
}

func (s *Server) handleDeviceDelete(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	id := scope.params["id"]
	self := id == scope.viewer.DeviceID
	if !self && !scope.viewer.Has(contract.PermOwner) {
		writeError(w, http.StatusForbidden, "forbidden", "only an owner can revoke another device")
		return
	}
	if err := s.opts.Devices.Revoke(r.Context(), id); err != nil {
		if errors.Is(err, ErrForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", "this device may not be revoked from here")
			return
		}
		s.log.Warn("remote: revoke failed", "request_id", scope.id, "device_id", scope.viewer.DeviceID, "target", id, "err", err)
		writeError(w, http.StatusNotFound, "not_found", "device not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	report, err := s.opts.Backend.Diagnostics(r.Context(), *scope.viewer)
	if err != nil {
		s.writeBackendError(w, scope, err)
		return
	}
	if report == nil {
		report = map[string]any{}
	}
	writeJSON(w, http.StatusOK, report)
}

// maxIconBytes bounds what handleAppIcon sends, whatever the source returns.
const maxIconBytes = 1 << 20

// handleAppIcon serves one app's own icon as PNG (contracts/http.md#app-icons).
// The adapter must look like an adapter name before the source (which checks
// the adapter table) sees it; the query string is ignored.
func (s *Server) handleAppIcon(w http.ResponseWriter, r *http.Request, scope *requestScope) {
	adapter := scope.params["adapter"]
	if !adapterName(adapter) {
		writeError(w, http.StatusNotFound, "unknown_app", "not an app this TV knows")
		return
	}
	if s.opts.AppIcons == nil {
		writeError(w, http.StatusNotFound, "no_icon", "use Bear Den's icon")
		return
	}
	png, err := s.opts.AppIcons.AppIcon(r.Context(), adapter)
	switch {
	case errors.Is(err, ErrUnknownApp):
		writeError(w, http.StatusNotFound, "unknown_app", "not an app this TV knows")
		return
	case err != nil || len(png) == 0 || len(png) > maxIconBytes || !strings.HasPrefix(string(png[:min(len(png), 8)]), "\x89PNG\r\n\x1a\n"):
		writeError(w, http.StatusNotFound, "no_icon", "use Bear Den's icon")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/png")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'")
	h.Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

// adapterName reports whether s has the shape of an adapter name
// (config.schema.json: lower-case letters, digits and dashes).
func adapterName(s string) bool {
	if s == "" || len(s) > 32 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// writeBackendError maps Backend errors to contract responses.
func (s *Server) writeBackendError(w http.ResponseWriter, scope *requestScope, err error) {
	var invalid *LayoutValidationError
	switch {
	case errors.As(err, &invalid):
		errs := invalid.Errors
		if errs == nil {
			errs = []string{}
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "invalid", "message": "the layout is not valid", "errors": errs})
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "this device may not perform that change")
	case errors.Is(err, ErrTransport):
		writeError(w, http.StatusForbidden, "transport_refused", "not allowed over this transport")
	case errors.Is(err, ErrRevisionConflict):
		writeError(w, http.StatusConflict, "revision_conflict", "the revision does not match the current layout")
	default:
		s.log.Warn("remote: backend error", "request_id", scope.id, "device_id", scope.viewer.DeviceID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "the TV could not complete the request")
	}
}
