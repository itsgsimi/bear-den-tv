// Tests for guest passes (guest.go, IssuePass and the guest paths of Claim,
// Authenticate and Grant in pairing.go), on a fake clock.

package pairing

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
	_ "time/tzdata" // fixed zone rules, whatever the host has installed

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
	"bear-den-tv/internal/remote"
	"bear-den-tv/internal/storage"
)

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// "Tonight" is 04:00 the next morning on the local calendar, also across
// both daylight-saving changes (the pass is then 1 h shorter or longer).
func TestPassEndTonightAcrossDST(t *testing.T) {
	berlin, ny := mustZone(t, "Europe/Berlin"), mustZone(t, "America/New_York")
	cases := []struct {
		name     string
		now      time.Time
		want     time.Time
		duration time.Duration
	}{
		{"evening", time.Date(2026, 9, 28, 21, 0, 0, 0, berlin), time.Date(2026, 9, 29, 4, 0, 0, 0, berlin), 7 * time.Hour},
		{"spring forward (Berlin)", time.Date(2026, 3, 28, 22, 0, 0, 0, berlin), time.Date(2026, 3, 29, 4, 0, 0, 0, berlin), 5 * time.Hour},
		{"fall back (Berlin)", time.Date(2026, 10, 24, 22, 0, 0, 0, berlin), time.Date(2026, 10, 25, 4, 0, 0, 0, berlin), 7 * time.Hour},
		{"spring forward (New York)", time.Date(2026, 3, 7, 23, 0, 0, 0, ny), time.Date(2026, 3, 8, 4, 0, 0, 0, ny), 4 * time.Hour},
		{"fall back (New York)", time.Date(2026, 10, 31, 23, 0, 0, 0, ny), time.Date(2026, 11, 1, 4, 0, 0, 0, ny), 6 * time.Hour},
		{"after midnight ends this morning", time.Date(2026, 9, 29, 1, 30, 0, 0, berlin), time.Date(2026, 9, 29, 4, 0, 0, 0, berlin), 150 * time.Minute},
		{"at 04:00 ends tomorrow", time.Date(2026, 9, 29, 4, 0, 0, 0, berlin), time.Date(2026, 9, 30, 4, 0, 0, 0, berlin), 24 * time.Hour},
		{"month end", time.Date(2026, 9, 30, 20, 0, 0, 0, berlin), time.Date(2026, 10, 1, 4, 0, 0, 0, berlin), 8 * time.Hour},
	}
	for _, tc := range cases {
		// The coordinator's clock may be in any zone; loc decides.
		got, err := PassEnd(tc.now.UTC(), contract.PassTonight, tc.now.Location())
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(tc.want) || got.Sub(tc.now) != tc.duration {
			t.Errorf("%s: got %v (%v), want %v (%v)", tc.name, got, got.Sub(tc.now), tc.want, tc.duration)
		}
		if l := got.In(tc.now.Location()); l.Hour() != 4 || l.Minute() != 0 {
			t.Errorf("%s: ends at %v local, want 04:00", tc.name, l)
		}
	}
	now := time.Date(2026, 3, 28, 22, 0, 0, 0, berlin)
	if got, _ := PassEnd(now, contract.Pass24h, berlin); got.Sub(now) != 24*time.Hour {
		t.Errorf("24h pass lasts %v", got.Sub(now))
	}
	if got, _ := PassEnd(now, contract.Pass7d, berlin); got.Sub(now) != 7*24*time.Hour {
		t.Errorf("7d pass lasts %v", got.Sub(now))
	}
	if _, err := PassEnd(now, "forever", berlin); err == nil {
		t.Error("unknown pass accepted")
	}
}

type guestEnv struct {
	t    *testing.T
	clk  *clock.Fake
	db   *storage.DB
	svc  *Service
	revs <-chan string
}

func newGuestEnv(t *testing.T, db *storage.DB, clk *clock.Fake) *guestEnv {
	t.Helper()
	svc, err := New(Options{DB: db, Clock: clk, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	revs, _ := svc.Revocations(ctx)
	return &guestEnv{t: t, clk: clk, db: db, svc: svc, revs: revs}
}

// pairGuest issues a pass and redeems it with the code, like a phone would.
func (e *guestEnv) pairGuest(pass string) (remote.ClaimResult, Issued) {
	e.t.Helper()
	iss, err := e.svc.IssuePass(context.Background(), pass)
	if err != nil {
		e.t.Fatal(err)
	}
	st := e.svc.State()
	if !st.Guest || st.PassExpiresAtMs == nil || *st.PassExpiresAtMs != iss.PassExpiresAt.UnixMilli() {
		e.t.Fatalf("pairing state does not show the guest pass: %+v", st)
	}
	res, err := e.svc.Claim(context.Background(), "", iss.Code, "Visitor", "192.0.2.5:1")
	if err != nil {
		e.t.Fatal(err)
	}
	return res, iss
}

func (e *guestEnv) revoked() string {
	select {
	case id := <-e.revs:
		return id
	default:
		return ""
	}
}

func openFileDB(t *testing.T, path string) *storage.DB {
	t.Helper()
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestGuestPassRedeemsAsGuestOnlyAndEndsOnTimer(t *testing.T) {
	db := openFileDB(t, ":memory:")
	defer db.Close()
	clk := clock.NewFake(time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC))
	e := newGuestEnv(t, db, clk)
	ctx := context.Background()

	res, iss := e.pairGuest(contract.Pass24h)
	if len(res.Permissions) != 1 || res.Permissions[0] != contract.PermGuest {
		t.Fatalf("guest got %v, want exactly [guest]", res.Permissions)
	}
	v, _, ok := e.svc.Authenticate(ctx, res.SessionToken)
	if !ok || !v.Guest() || v.Has(contract.PermController) || v.ExpiresAtMs == nil || *v.ExpiresAtMs != iss.PassExpiresAt.UnixMilli() {
		t.Fatalf("viewer %+v ok=%v", v, ok)
	}
	list, _ := e.svc.List(ctx)
	if len(list) != 1 || !list[0].Guest || list[0].ExpiresAtMs == nil {
		t.Fatalf("device list %+v", list)
	}

	clk.Advance(24*time.Hour - time.Second)
	if id := e.revoked(); id != "" {
		t.Fatalf("revoked %s a second early", id)
	}
	clk.Advance(time.Second)
	if id := e.revoked(); id != res.DeviceID {
		t.Fatalf("revocation at the end = %q, want %s", id, res.DeviceID)
	}
	if _, _, ok := e.svc.Authenticate(ctx, res.SessionToken); ok {
		t.Fatal("ended pass still authenticates")
	}
	if _, err := db.SessionByTokenHash(ctx, hashHex(res.SessionToken)); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("session survived the end of the pass: %v", err)
	}
	if clk.Pending() != 0 {
		t.Fatalf("%d timers still armed with no guest paired", clk.Pending())
	}
}

// Authenticate checks the end itself, so a pass whose timer has not fired
// (a suspended box, a wall-clock jump) is refused and revoked on the next
// request.
func TestGuestPassEndCheckedOnEveryRequest(t *testing.T) {
	db := openFileDB(t, ":memory:")
	defer db.Close()
	clk := clock.NewFake(time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC))
	e := newGuestEnv(t, db, clk)
	ctx := context.Background()
	ended := clk.Now().Add(-time.Minute).UnixMilli()
	if err := db.CreateDevice(ctx, storage.Device{ID: "dev_g", Name: "Visitor", Permissions: []string{"guest"}, CreatedAt: "x", ExpiresAtMs: &ended}); err != nil {
		t.Fatal(err)
	}
	token, _, err := e.svc.createSession(ctx, "dev_g", "x", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := e.svc.Authenticate(ctx, token); ok {
		t.Fatal("ended pass authenticated")
	}
	if id := e.revoked(); id != "dev_g" {
		t.Fatalf("ended pass not revoked on request (got %q)", id)
	}
	if dev, _ := db.Device(ctx, "dev_g"); !dev.Revoked() {
		t.Fatal("device not marked revoked")
	}
}

// The end is stored: a coordinator restart keeps the pass and its timer, and
// a pass that ended while the coordinator was stopped is revoked at start.
func TestGuestPassSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	clk := clock.NewFake(time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC))
	db := openFileDB(t, path)
	e := newGuestEnv(t, db, clk)
	res, _ := e.pairGuest(contract.PassTonight) // 04:00 UTC: 9 h
	fam, err := e.svc.Issue(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	famRes, err := e.svc.Claim(context.Background(), fam.Token, "", "Family", "192.0.2.6:1")
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Restart one hour later on a fresh clock (the old timers are gone).
	clk2 := clock.NewFake(clk.Now().Add(time.Hour))
	db2 := openFileDB(t, path)
	defer db2.Close()
	e2 := newGuestEnv(t, db2, clk2)
	ctx := context.Background()
	if _, _, ok := e2.svc.Authenticate(ctx, res.SessionToken); !ok {
		t.Fatal("guest lost its pass across a restart")
	}
	if clk2.Pending() != 1 {
		t.Fatalf("pass timer not re-armed after restart (%d timers)", clk2.Pending())
	}
	clk2.Advance(8 * time.Hour)
	if id := e2.revoked(); id != res.DeviceID {
		t.Fatalf("restored pass did not end at 04:00 (revoked %q)", id)
	}
	if _, _, ok := e2.svc.Authenticate(ctx, famRes.SessionToken); !ok {
		t.Fatal("the family phone was revoked with the guest")
	}
	_ = db2.Close()

	// A pass that ends while the coordinator is down is revoked by New.
	db3 := openFileDB(t, path)
	defer db3.Close()
	clk3 := clock.NewFake(clk2.Now())
	e3 := newGuestEnv(t, db3, clk3)
	res3, _ := e3.pairGuest(contract.Pass24h)
	_ = db3.Close()
	db4 := openFileDB(t, path)
	defer db4.Close()
	svc4, err := New(Options{DB: db4, Clock: clock.NewFake(clk3.Now().Add(25 * time.Hour)), Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	if dev, _ := db4.Device(ctx, res3.DeviceID); !dev.Revoked() {
		t.Fatal("pass that ended during downtime was not revoked at start")
	}
	if _, _, ok := svc4.Authenticate(ctx, famRes.SessionToken); !ok {
		t.Fatal("family phone lost at start")
	}
}

// Guest passes come only from IssuePass; guest is never granted, never mixed
// with other permissions, and a guest cannot be upgraded.
func TestGuestCannotBeGrantedOrUpgraded(t *testing.T) {
	db := openFileDB(t, ":memory:")
	defer db.Close()
	clk := clock.NewFake(time.Date(2026, 9, 28, 19, 0, 0, 0, time.UTC))
	e := newGuestEnv(t, db, clk)
	ctx := context.Background()
	if _, err := e.svc.Issue(ctx, []contract.Permission{contract.PermGuest}); err == nil {
		t.Fatal("Issue made a guest invitation without an end")
	}
	if _, err := e.svc.IssuePass(ctx, "all-weekend"); err == nil {
		t.Fatal("unknown pass length accepted")
	}
	res, _ := e.pairGuest(contract.Pass7d)
	if err := e.svc.Grant(ctx, res.DeviceID, []contract.Permission{contract.PermController}); err == nil {
		t.Fatal("guest pass upgraded to controller")
	}
	fam, _ := e.svc.Issue(ctx, nil)
	famRes, err := e.svc.Claim(ctx, fam.Token, "", "Family", "192.0.2.6:1")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Grant(ctx, famRes.DeviceID, []contract.Permission{contract.PermGuest}); err == nil {
		t.Fatal("guest granted to a family phone")
	}
	if got := normalizePermissions([]contract.Permission{contract.PermGuest, contract.PermOwner}); len(got) != 2 || got[0] != contract.PermController || got[1] != contract.PermOwner {
		t.Fatalf("normalizePermissions kept guest: %v", got)
	}
	// A "tonight" code redeemed after 04:00 never creates a device.
	clk.Advance(8*time.Hour + 59*time.Minute) // 03:59 UTC: the pass ends at 04:00
	iss, err := e.svc.IssuePass(ctx, contract.PassTonight)
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(90 * time.Second) // 04:00:30: the code is still live, the pass is not
	if !e.svc.State().Active {
		t.Fatal("the invitation itself expired; the test would not reach the pass check")
	}
	if _, err := e.svc.Claim(ctx, iss.Token, "", "Late", "192.0.2.7:1"); !errors.Is(err, remote.ErrInvitationExpired) {
		t.Fatalf("a pass that had already ended was redeemed (err %v)", err)
	}
}
