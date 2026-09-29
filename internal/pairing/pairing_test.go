// Tests for invitations, claims, expiry and revocation (pairing.go).

package pairing

import (
	"context"
	"errors"
	"testing"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/storage"
)

func newService(t *testing.T) (*Service, *clock.Fake) {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	s, err := New(Options{
		DB:      db,
		Clock:   fc,
		Limits:  func() Limits { return Limits{Expiry: 120 * time.Second, MaxAttempts: 3} },
		BaseURL: func() string { return "http://192.0.2.10:8090/" },
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, fc
}

func TestIssueStateAndQR(t *testing.T) {
	s, fc := newService(t)
	ctx := context.Background()
	inv, err := s.Issue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Code) != 6 || len(inv.Token) < 40 || inv.URL != "http://192.0.2.10:8090/#pair="+inv.Token {
		t.Fatalf("issued %+v", inv)
	}
	st := s.State()
	if !st.Active || *st.Code != inv.Code || st.ExpiresInS != 120 || st.AttemptsLeft != 3 || len(st.QRModules) < 21 {
		t.Fatalf("state %+v", st)
	}
	for _, row := range st.QRModules {
		if len(row) != len(st.QRModules) {
			t.Fatal("QR rows are not square")
		}
	}
	fc.Advance(30 * time.Second)
	if s.State().ExpiresInS != 90 {
		t.Fatalf("expires_in_s %d", s.State().ExpiresInS)
	}
	second, _ := s.Issue(ctx, nil)
	if _, err := s.Claim(ctx, inv.Token, "", "old", "10.0.0.2:1"); !errors.Is(err, remote.ErrInvalidInvitation) {
		t.Fatalf("rotated invitation still redeemable: %v", err)
	}
	if err := s.Cancel(ctx); err != nil {
		t.Fatal(err)
	}
	if s.State().Active {
		t.Fatal("cancel did not clear the invitation")
	}
	if _, err := s.Claim(ctx, second.Token, "", "p", "10.0.0.2:1"); !errors.Is(err, remote.ErrInvalidInvitation) {
		t.Fatalf("cancelled invitation redeemable: %v", err)
	}
}

func TestClaimExpiry(t *testing.T) {
	s, fc := newService(t)
	ctx := context.Background()
	inv, _ := s.Issue(ctx, nil)
	fc.Advance(119 * time.Second)
	if !s.State().Active {
		t.Fatal("expired early")
	}
	fc.Advance(2 * time.Second)
	if s.State().Active {
		t.Fatal("invitation did not expire")
	}
	if _, err := s.Claim(ctx, inv.Token, "", "p", "10.0.0.2:1"); !errors.Is(err, remote.ErrInvalidInvitation) && !errors.Is(err, remote.ErrInvitationExpired) {
		t.Fatalf("expired claim: %v", err)
	}
}

func TestClaimExpiredButTimerNotYetFired(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	inv, _ := s.Issue(ctx, nil)
	s.mu.Lock()
	s.live.expiresAt = s.opts.Clock.Now().Add(-time.Second)
	s.mu.Unlock()
	if _, err := s.Claim(ctx, inv.Token, "", "p", "10.0.0.2:1"); !errors.Is(err, remote.ErrInvitationExpired) {
		t.Fatalf("want expired, got %v", err)
	}
}

func TestWrongCodeAndAttemptsExhaustion(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	_, _ = s.Issue(ctx, nil)
	for i := 0; i < 2; i++ {
		if _, err := s.Claim(ctx, "", "000000", "p", "10.0.0.2:1"); !errors.Is(err, remote.ErrInvalidInvitation) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if s.State().AttemptsLeft != 1 {
		t.Fatalf("attempts left %d", s.State().AttemptsLeft)
	}
	if _, err := s.Claim(ctx, "", "000000", "p", "10.0.0.2:1"); !errors.Is(err, remote.ErrTooManyAttempts) {
		t.Fatalf("exhaustion: %v", err)
	}
	if s.State().Active {
		t.Fatal("exhausted invitation still live")
	}
}

func TestClaimSucceedsOnceAndAuthenticates(t *testing.T) {
	s, fc := newService(t)
	ctx := context.Background()
	inv, _ := s.Issue(ctx, []contract.Permission{contract.PermOwner})
	res, err := s.Claim(ctx, "", inv.Code, "  Alice's\tphone ", "10.0.0.2:1")
	if err != nil {
		t.Fatal(err)
	}
	if res.DeviceName != "Alice'sphone" || len(res.Permissions) != 2 || res.Permissions[1] != contract.PermOwner {
		t.Fatalf("claim result %+v", res)
	}
	if res.SessionToken == "" || res.CSRFToken == "" || res.SessionToken == res.CSRFToken {
		t.Fatal("tokens missing or identical")
	}
	if s.State().Active {
		t.Fatal("redeemed invitation still displayed")
	}
	if _, err := s.Claim(ctx, inv.Token, "", "again", "10.0.0.3:1"); !errors.Is(err, remote.ErrInvalidInvitation) {
		t.Fatalf("token reused: %v", err)
	}
	v, csrf, ok := s.Authenticate(ctx, res.SessionToken)
	if !ok || v.DeviceID != res.DeviceID || csrf != res.CSRFToken || !v.Has(contract.PermOwner) {
		t.Fatalf("authenticate %+v %q %v", v, csrf, ok)
	}
	if _, _, ok := s.Authenticate(ctx, res.SessionToken+"x"); ok {
		t.Fatal("wrong token authenticated")
	}
	if s.Count(ctx) != 1 {
		t.Fatal("count")
	}
	s.Touch(ctx, res.DeviceID, true)
	fc.Advance(5 * time.Second)
	list, _ := s.List(ctx)
	if len(list) != 1 || !list[0].Connected || list[0].LastSeenMs == 0 {
		t.Fatalf("list %+v", list)
	}
	if err := s.Grant(ctx, res.DeviceID, []contract.Permission{contract.PermLayoutEditor}); err != nil {
		t.Fatal(err)
	}
	v, _, _ = s.Authenticate(ctx, res.SessionToken)
	if v.Has(contract.PermOwner) || !v.Has(contract.PermLayoutEditor) {
		t.Fatalf("grant not applied: %v", v.Permissions)
	}
	if err := s.Grant(ctx, res.DeviceID, []contract.Permission{"root"}); err == nil {
		t.Fatal("unknown permission granted")
	}
	if err := s.Logout(ctx, res.SessionToken); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := s.Authenticate(ctx, res.SessionToken); ok {
		t.Fatal("logged-out session authenticated")
	}
}

// OnPaired (Den badges count paired phones) runs once per successful claim,
// never for a wrong code or a reused token.
func TestOnPairedOnlyOnSuccess(t *testing.T) {
	s, _ := newService(t)
	paired := 0
	s.opts.OnPaired = func() { paired++ }
	ctx := context.Background()
	inv, _ := s.Issue(ctx, nil)
	if _, err := s.Claim(ctx, "", "000000", "p", "10.0.0.2:1"); err == nil {
		t.Fatal("wrong code claimed")
	}
	if paired != 0 {
		t.Fatal("OnPaired after a wrong code")
	}
	if _, err := s.Claim(ctx, inv.Token, "", "p", "10.0.0.2:1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx, inv.Token, "", "p", "10.0.0.2:1"); err == nil {
		t.Fatal("token reused")
	}
	if paired != 1 {
		t.Fatalf("OnPaired ran %d times, want 1", paired)
	}
}

func TestRevocationKillsAuthenticate(t *testing.T) {
	s, _ := newService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	revs, _ := s.Revocations(ctx)
	inv, _ := s.Issue(ctx, nil)
	res, _ := s.Claim(ctx, inv.Token, "", "p", "10.0.0.2:1")
	if _, _, ok := s.Authenticate(ctx, res.SessionToken); !ok {
		t.Fatal("fresh session rejected")
	}
	if err := s.Revoke(ctx, res.DeviceID); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-revs:
		if id != res.DeviceID {
			t.Fatalf("revoked %s", id)
		}
	default:
		t.Fatal("no revocation event")
	}
	if _, _, ok := s.Authenticate(ctx, res.SessionToken); ok {
		t.Fatal("revoked session authenticated")
	}
	if err := s.Revoke(ctx, res.DeviceID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("double revoke: %v", err)
	}
	inv, _ = s.Issue(ctx, nil)
	a, _ := s.Claim(ctx, inv.Token, "", "a", "10.0.0.2:1")
	inv, _ = s.Issue(ctx, nil)
	b, _ := s.Claim(ctx, inv.Token, "", "b", "10.0.0.2:1")
	if err := s.Revoke(ctx, "*"); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{<-revs: true, <-revs: true}
	if !got[a.DeviceID] || !got[b.DeviceID] {
		t.Fatalf("revoke all events %v", got)
	}
	if s.Count(ctx) != 0 {
		t.Fatal("devices remain after revoke all")
	}
}

func TestSourceRateLimit(t *testing.T) {
	s, fc := newService(t)
	ctx := context.Background()
	_, _ = s.Issue(ctx, nil)
	for i := 0; i < DefaultSourceClaimsPerMinute; i++ {
		_, err := s.Claim(ctx, "", "999999", "p", "10.0.0.9:5000")
		if errors.Is(err, remote.ErrTooManyAttempts) && i < 2 {
			t.Fatalf("rate limited too early at %d", i)
		}
		if i >= 2 {
			// Invitation exhausted after 3 wrong codes; keep hitting to spend the source budget.
			continue
		}
	}
	if _, err := s.Claim(ctx, "", "999999", "p", "10.0.0.9:6000"); !errors.Is(err, remote.ErrTooManyAttempts) {
		t.Fatalf("11th claim from the same host: %v", err)
	}
	// A different source is unaffected.
	if _, err := s.Claim(ctx, "", "999999", "p", "10.0.0.10:1"); errors.Is(err, remote.ErrTooManyAttempts) {
		t.Fatal("other source rate limited")
	}
	fc.Advance(time.Minute)
	_, _ = s.Issue(ctx, nil)
	if _, err := s.Claim(ctx, "", "999999", "p", "10.0.0.9:7000"); !errors.Is(err, remote.ErrInvalidInvitation) {
		t.Fatalf("budget did not refill: %v", err)
	}
}

func TestNewCancelsStaleInvitations(t *testing.T) {
	db, _ := storage.Open(":memory:")
	defer db.Close()
	_ = db.CreateInvitation(context.Background(), storage.Invitation{ID: "stale", TokenSHA256: "a", CodeSHA256: "b", ExpiresAtMs: 1 << 60, AttemptsLeft: 5})
	if _, err := New(Options{DB: db}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.LiveInvitation(context.Background()); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("stale invitation survived restart")
	}
}
