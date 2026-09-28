// Package pairing issues single-use invitations on the TV, redeems them from
// phones, and owns the device/session records behind remote.Devices. Secrets
// (fragment token, six-digit code, session token) exist in memory only while
// they are handed to the TV or the phone; the database keeps SHA-256 hashes.
package pairing

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/time/rate"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/storage"
)

// Limits are the invitation parameters taken from the configuration.
type Limits struct {
	Expiry      time.Duration
	MaxAttempts int
}

// DefaultSourceClaimsPerMinute is the per-source-address claim budget
// (contracts/http.md: 10 per source address per minute).
const DefaultSourceClaimsPerMinute = 10

// MaxDeviceName bounds the phone-supplied device name.
const MaxDeviceName = 64

// Options configures a Service.
type Options struct {
	DB     *storage.DB
	Clock  clock.Clock
	Logger *slog.Logger
	// Limits returns the current expiry and attempt budget.
	Limits func() Limits
	// BaseURL returns the URL phones open (scheme://host:port/), or "" when
	// the remote is not listening; the fragment token is appended to it.
	BaseURL func() string
	// SourceClaimsPerMinute overrides DefaultSourceClaimsPerMinute.
	SourceClaimsPerMinute int
	// OnChange is called after the invitation state changes (issue, cancel,
	// expiry, redemption) or a device list change so the coordinator republishes.
	OnChange func()
}

// Issued is a freshly created invitation; Token and Code are shown once on the TV.
type Issued struct {
	ID           string
	Token        string
	Code         string
	URL          string
	ExpiresAt    time.Time
	AttemptsLeft int
}

type live struct {
	id           string
	tokenHash    string
	codeHash     string
	code         string
	url          string
	qr           []string
	expiresAt    time.Time
	attemptsLeft int
	permissions  []contract.Permission
	timer        clock.Timer
}

// Service implements remote.Devices and the TV-side invitation operations.
type Service struct {
	opts      Options
	mu        sync.Mutex
	live      *live
	limiters  map[string]*limiterEntry
	connected map[string]bool
	subs      map[chan string]struct{}
}

type limiterEntry struct {
	lim  *rate.Limiter
	seen time.Time
}

// New creates the service and cancels any invitation left in the database by
// a previous run, since the TV is no longer displaying it.
func New(opts Options) (*Service, error) {
	if opts.DB == nil {
		return nil, errors.New("pairing: database required")
	}
	if opts.Clock == nil {
		opts.Clock = clock.Real{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Limits == nil {
		opts.Limits = func() Limits { return Limits{Expiry: 120 * time.Second, MaxAttempts: 5} }
	}
	if opts.BaseURL == nil {
		opts.BaseURL = func() string { return "" }
	}
	if opts.SourceClaimsPerMinute <= 0 {
		opts.SourceClaimsPerMinute = DefaultSourceClaimsPerMinute
	}
	if err := opts.DB.CancelInvitations(context.Background()); err != nil {
		return nil, err
	}
	return &Service{opts: opts, limiters: map[string]*limiterEntry{}, connected: map[string]bool{}, subs: map[chan string]struct{}{}}, nil
}

// Issue creates a new invitation, replacing any live one. permissions are
// granted to the device that redeems it; nil means controller only.
func (s *Service) Issue(ctx context.Context, permissions []contract.Permission) (Issued, error) {
	lim := s.opts.Limits()
	if lim.Expiry <= 0 || lim.MaxAttempts <= 0 {
		return Issued{}, errors.New("pairing: invalid limits")
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return Issued{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	code, err := sixDigits()
	if err != nil {
		return Issued{}, err
	}
	id, err := randomID("inv_")
	if err != nil {
		return Issued{}, err
	}
	now := s.opts.Clock.Now()
	url := ""
	var qr []string
	if base := s.opts.BaseURL(); base != "" {
		url = base + "#pair=" + token
		qr, err = QRModules(url)
		if err != nil {
			return Issued{}, err
		}
	}
	inv := storage.Invitation{ID: id, TokenSHA256: hashHex(token), CodeSHA256: hashHex(code), ExpiresAtMs: now.Add(lim.Expiry).UnixMilli(), AttemptsLeft: lim.MaxAttempts}
	if err := s.opts.DB.CreateInvitation(ctx, inv); err != nil {
		return Issued{}, err
	}
	perms := normalizePermissions(permissions)
	s.mu.Lock()
	if s.live != nil && s.live.timer != nil {
		s.live.timer.Stop()
	}
	l := &live{id: id, tokenHash: inv.TokenSHA256, codeHash: inv.CodeSHA256, code: code, url: url, qr: qr, expiresAt: now.Add(lim.Expiry), attemptsLeft: lim.MaxAttempts, permissions: perms}
	l.timer = s.opts.Clock.AfterFunc(lim.Expiry, func() { s.expire(id) })
	s.live = l
	s.mu.Unlock()
	s.opts.Logger.Info("pairing: invitation issued", "invitation_id", id, "expires_in_s", int(lim.Expiry/time.Second))
	s.notify()
	return Issued{ID: id, Token: token, Code: code, URL: url, ExpiresAt: l.expiresAt, AttemptsLeft: lim.MaxAttempts}, nil
}

func (s *Service) expire(id string) {
	s.mu.Lock()
	if s.live == nil || s.live.id != id {
		s.mu.Unlock()
		return
	}
	s.live = nil
	s.mu.Unlock()
	_ = s.opts.DB.CancelInvitations(context.Background())
	s.opts.Logger.Info("pairing: invitation expired", "invitation_id", id)
	s.notify()
}

// Cancel withdraws the live invitation, if any.
func (s *Service) Cancel(ctx context.Context) error {
	s.mu.Lock()
	had := s.live != nil
	if had && s.live.timer != nil {
		s.live.timer.Stop()
	}
	s.live = nil
	s.mu.Unlock()
	if err := s.opts.DB.CancelInvitations(ctx); err != nil {
		return err
	}
	if had {
		s.notify()
	}
	return nil
}

// State is the shell's view of the live invitation.
func (s *Service) State() contract.Pairing {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.live == nil {
		return contract.Pairing{Active: false, ExpiresInS: 0, AttemptsLeft: 0}
	}
	left := s.live.expiresAt.Sub(s.opts.Clock.Now())
	if left < 0 {
		left = 0
	}
	code := s.live.code
	var url *string
	if s.live.url != "" {
		u := s.live.url
		url = &u
	}
	return contract.Pairing{Active: true, Code: &code, URL: url, QRModules: s.live.qr, ExpiresInS: int(left / time.Second), AttemptsLeft: s.live.attemptsLeft}
}

// Claim implements remote.Devices.
func (s *Service) Claim(ctx context.Context, token, code, deviceName, sourceAddr string) (remote.ClaimResult, error) {
	if !s.allowSource(sourceAddr) {
		s.opts.Logger.Warn("pairing: claim rate limited", "source", redactSource(sourceAddr))
		return remote.ClaimResult{}, remote.ErrTooManyAttempts
	}
	s.mu.Lock()
	l := s.live
	if l == nil {
		s.mu.Unlock()
		return remote.ClaimResult{}, remote.ErrInvalidInvitation
	}
	if !s.opts.Clock.Now().Before(l.expiresAt) {
		s.live = nil
		s.mu.Unlock()
		_ = s.opts.DB.CancelInvitations(ctx)
		s.notify()
		return remote.ClaimResult{}, remote.ErrInvitationExpired
	}
	matched := false
	switch {
	case token != "":
		matched = constantEqual(hashHex(token), l.tokenHash)
	case code != "":
		matched = constantEqual(hashHex(strings.TrimSpace(code)), l.codeHash)
	}
	if !matched {
		l.attemptsLeft--
		exhausted := l.attemptsLeft <= 0
		if exhausted {
			s.live = nil
		}
		s.mu.Unlock()
		if exhausted {
			_ = s.opts.DB.CancelInvitations(ctx)
			s.opts.Logger.Warn("pairing: invitation attempts exhausted", "invitation_id", l.id)
			s.notify()
			return remote.ClaimResult{}, remote.ErrTooManyAttempts
		}
		_ = s.opts.DB.UpdateInvitation(ctx, storage.Invitation{ID: l.id, AttemptsLeft: l.attemptsLeft})
		s.notify()
		return remote.ClaimResult{}, remote.ErrInvalidInvitation
	}
	// Single use: the invitation is consumed before any record is written.
	s.live = nil
	l.timer.Stop()
	perms := l.permissions
	s.mu.Unlock()

	now := s.opts.Clock.Now()
	nowText := now.UTC().Format(time.RFC3339)
	redeemed := nowText
	if err := s.opts.DB.UpdateInvitation(ctx, storage.Invitation{ID: l.id, AttemptsLeft: l.attemptsLeft, RedeemedAt: &redeemed}); err != nil {
		return remote.ClaimResult{}, err
	}
	deviceID, err := randomID("dev_")
	if err != nil {
		return remote.ClaimResult{}, err
	}
	name := sanitizeName(deviceName)
	permText := make([]string, len(perms))
	for i, p := range perms {
		permText[i] = string(p)
	}
	if err := s.opts.DB.CreateDevice(ctx, storage.Device{ID: deviceID, Name: name, Permissions: permText, CreatedAt: nowText, LastSeenMs: now.UnixMilli()}); err != nil {
		return remote.ClaimResult{}, err
	}
	sessionToken, csrf, err := s.createSession(ctx, deviceID, nowText, now.UnixMilli())
	if err != nil {
		return remote.ClaimResult{}, err
	}
	s.opts.Logger.Info("pairing: device paired", "device_id", deviceID, "permissions", permText)
	s.notify()
	return remote.ClaimResult{DeviceID: deviceID, DeviceName: name, Permissions: perms, SessionToken: sessionToken, CSRFToken: csrf}, nil
}

func (s *Service) createSession(ctx context.Context, deviceID, createdAt string, nowMs int64) (token, csrf string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	csrf = csrfFor(token)
	err = s.opts.DB.CreateSession(ctx, storage.Session{TokenSHA256: hashHex(token), DeviceID: deviceID, CSRFSHA256: hashHex(csrf), CreatedAt: createdAt, LastSeenMs: nowMs})
	return token, csrf, err
}

// csrfFor derives the CSRF token from the session token, so the server can
// hand it back at /api/v1/session without storing a recoverable copy.
func csrfFor(sessionToken string) string {
	mac := hmac.New(sha256.New, []byte(sessionToken))
	mac.Write([]byte("bear-den-tv csrf v1"))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Authenticate implements remote.Devices.
func (s *Service) Authenticate(ctx context.Context, sessionToken string) (remote.Viewer, string, bool) {
	if sessionToken == "" {
		return remote.Viewer{}, "", false
	}
	sess, err := s.opts.DB.SessionByTokenHash(ctx, hashHex(sessionToken))
	if err != nil {
		return remote.Viewer{}, "", false
	}
	dev, err := s.opts.DB.Device(ctx, sess.DeviceID)
	if err != nil || dev.Revoked() {
		return remote.Viewer{}, "", false
	}
	csrf := csrfFor(sessionToken)
	if !constantEqual(hashHex(csrf), sess.CSRFSHA256) {
		return remote.Viewer{}, "", false
	}
	_ = s.opts.DB.TouchSession(ctx, sess.TokenSHA256, s.opts.Clock.Now().UnixMilli())
	return remote.Viewer{DeviceID: dev.ID, DeviceName: dev.Name, Permissions: toPermissions(dev.Permissions)}, csrf, true
}

// Logout implements remote.Devices.
func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	err := s.opts.DB.DeleteSession(ctx, hashHex(sessionToken))
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	return err
}

// List implements remote.Devices.
func (s *Service) List(ctx context.Context) ([]contract.Device, error) {
	devs, err := s.opts.DB.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.Device, 0, len(devs))
	for _, d := range devs {
		out = append(out, contract.Device{ID: d.ID, Name: d.Name, Permissions: toPermissions(d.Permissions), Connected: s.connected[d.ID], LastSeenMs: d.LastSeenMs, CreatedAt: d.CreatedAt})
	}
	return out, nil
}

// Count returns the number of paired (not revoked) devices.
func (s *Service) Count(ctx context.Context) int {
	devs, err := s.opts.DB.ListDevices(ctx)
	if err != nil {
		return 0
	}
	return len(devs)
}

// Revoke implements remote.Devices; "*" revokes every device.
func (s *Service) Revoke(ctx context.Context, deviceID string) error {
	ids, err := s.opts.DB.RevokeDevice(ctx, deviceID, s.opts.Clock.Now().UTC().Format(time.RFC3339))
	if errors.Is(err, storage.ErrNotFound) {
		return storage.ErrNotFound
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	for _, id := range ids {
		delete(s.connected, id)
		for ch := range s.subs {
			select {
			case ch <- id:
			default:
				s.opts.Logger.Warn("pairing: revocation subscriber is not draining", "device_id", id)
			}
		}
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.opts.Logger.Info("pairing: device revoked", "device_id", id)
	}
	s.notify()
	return nil
}

// Revocations implements remote.Devices. The channel is buffered; a subscriber
// that stops draining loses events after 64 revocations.
func (s *Service) Revocations(ctx context.Context) (<-chan string, error) {
	ch := make(chan string, 64)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()
	return ch, nil
}

// Touch implements remote.Devices.
func (s *Service) Touch(ctx context.Context, deviceID string, connected bool) {
	s.mu.Lock()
	changed := s.connected[deviceID] != connected
	if connected {
		s.connected[deviceID] = true
	} else {
		delete(s.connected, deviceID)
	}
	s.mu.Unlock()
	_ = s.opts.DB.TouchDevice(ctx, deviceID, s.opts.Clock.Now().UnixMilli())
	if changed {
		s.notify()
	}
}

// Grant replaces a device's permissions (TV-only operation). Unknown
// permissions are rejected; controller is always included.
func (s *Service) Grant(ctx context.Context, deviceID string, permissions []contract.Permission) error {
	for _, p := range permissions {
		if p.Rank() == 0 {
			return fmt.Errorf("pairing: unknown permission %q", p)
		}
	}
	perms := normalizePermissions(permissions)
	text := make([]string, len(perms))
	for i, p := range perms {
		text[i] = string(p)
	}
	if err := s.opts.DB.SetPermissions(ctx, deviceID, text); err != nil {
		return err
	}
	s.opts.Logger.Info("pairing: permissions changed", "device_id", deviceID, "permissions", text)
	s.notify()
	return nil
}

func (s *Service) allowSource(sourceAddr string) bool {
	host := sourceAddr
	if h, _, err := net.SplitHostPort(sourceAddr); err == nil {
		host = h
	}
	now := s.opts.Clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.limiters {
		if now.Sub(e.seen) > 10*time.Minute {
			delete(s.limiters, k)
		}
	}
	e, ok := s.limiters[host]
	if !ok {
		e = &limiterEntry{lim: rate.NewLimiter(rate.Every(time.Minute/time.Duration(s.opts.SourceClaimsPerMinute)), s.opts.SourceClaimsPerMinute)}
		s.limiters[host] = e
	}
	e.seen = now
	return e.lim.AllowN(now, 1)
}

func (s *Service) notify() {
	if s.opts.OnChange != nil {
		s.opts.OnChange()
	}
}

func normalizePermissions(in []contract.Permission) []contract.Permission {
	set := map[contract.Permission]bool{contract.PermController: true}
	for _, p := range in {
		if p.Rank() > 0 {
			set[p] = true
		}
	}
	out := []contract.Permission{contract.PermController}
	if set[contract.PermLayoutEditor] {
		out = append(out, contract.PermLayoutEditor)
	}
	if set[contract.PermOwner] {
		out = append(out, contract.PermOwner)
	}
	return out
}

func toPermissions(in []string) []contract.Permission {
	out := make([]contract.Permission, 0, len(in))
	for _, p := range in {
		out = append(out, contract.Permission(p))
	}
	return out
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= MaxDeviceName {
			break
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "Phone"
	}
	return out
}

func hashHex(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func constantEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func sixDigits() (string, error) {
	var b [4]byte
	for {
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		n := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		// Reject values that would bias the modulo.
		if n >= 4294000000 {
			continue
		}
		return fmt.Sprintf("%06d", n%1000000), nil
	}
}

func randomID(prefix string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

func redactSource(addr string) string {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return "private-ip"
	}
	return host
}
