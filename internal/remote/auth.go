// Cookie authentication, CSRF and Origin checks for the LAN remote (spec
// contracts/http.md; docs/security.md).

package remote

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
)

// authenticate resolves the session cookie through Devices.Authenticate and
// fills scope.viewer; ok=false means no cookie, an unknown session, or a
// revoked device.
func (s *Server) authenticate(r *http.Request, scope *requestScope) bool {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	v, csrf, ok := s.opts.Devices.Authenticate(r.Context(), c.Value)
	if !ok {
		return false
	}
	v.Secure = s.https
	scope.viewer = &v
	scope.csrf = csrf
	scope.sessionToken = c.Value
	return true
}

// csrfValid compares X-BDTV-CSRF with the session's token in constant time.
func (s *Server) csrfValid(r *http.Request, scope *requestScope) bool {
	got := r.Header.Get(csrfHeader)
	if got == "" || scope.csrf == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(scope.csrf)) == 1
}

// originAllowed requires Origin to be exactly scheme://host for the transport
// scheme and an allowed host; a missing Origin is rejected.
func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	want := "http"
	if s.https {
		want = "https"
	}
	if !strings.EqualFold(u.Scheme, want) || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return s.hostAllowed(u.Host)
}

// setSessionCookie writes the bdtv_session cookie with the contract flags.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.https,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionCookieAge.Seconds()),
	})
}

// clearSessionCookie expires the cookie on the client.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.https,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}
