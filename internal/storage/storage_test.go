// Tests for migrations, devices, invitations and secret stores (storage.go,
// secrets.go).

package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateAndDevices(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if v, _ := db.Version(); v != SchemaVersion {
		t.Fatalf("version %d", v)
	}
	if err := db.CreateDevice(ctx, Device{ID: "dev_1", Name: "Phone", Permissions: []string{"controller"}, CreatedAt: "2026-09-22T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateDevice(ctx, Device{ID: "dev_2", Name: "Tablet", Permissions: []string{"controller", "owner"}, CreatedAt: "2026-09-22T00:00:01Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(ctx, Session{TokenSHA256: "t1", DeviceID: "dev_1", CSRFSHA256: "c1", CreatedAt: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(ctx, Session{TokenSHA256: "t2", DeviceID: "dev_2", CSRFSHA256: "c2", CreatedAt: "x"}); err != nil {
		t.Fatal(err)
	}
	list, _ := db.ListDevices(ctx)
	if len(list) != 2 || list[1].Permissions[1] != "owner" {
		t.Fatalf("list %+v", list)
	}
	if err := db.SetPermissions(ctx, "dev_1", []string{"controller", "layout_editor"}); err != nil {
		t.Fatal(err)
	}
	if err := db.TouchDevice(ctx, "dev_1", 42); err != nil {
		t.Fatal(err)
	}
	dev, _ := db.Device(ctx, "dev_1")
	if dev.LastSeenMs != 42 || len(dev.Permissions) != 2 {
		t.Fatalf("device %+v", dev)
	}
	ids, err := db.RevokeDevice(ctx, "dev_1", "now")
	if err != nil || len(ids) != 1 {
		t.Fatalf("revoke %v %v", ids, err)
	}
	if _, err := db.SessionByTokenHash(ctx, "t1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("session survived revocation")
	}
	if _, err := db.SessionByTokenHash(ctx, "t2"); err != nil {
		t.Fatal("other session lost")
	}
	if _, err := db.RevokeDevice(ctx, "dev_1", "now"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double revoke: %v", err)
	}
	if err := db.SetPermissions(ctx, "dev_1", []string{"owner"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("permissions granted to a revoked device")
	}
	ids, _ = db.RevokeDevice(ctx, "*", "now")
	if len(ids) != 1 || ids[0] != "dev_2" {
		t.Fatalf("revoke all %v", ids)
	}
	if l, _ := db.ListDevices(ctx); len(l) != 0 {
		t.Fatal("revoked devices listed")
	}
}

func TestInvitationsFocusAppState(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	ctx := context.Background()
	if _, err := db.LiveInvitation(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatal("phantom invitation")
	}
	_ = db.CreateInvitation(ctx, Invitation{ID: "a", TokenSHA256: "ta", CodeSHA256: "ca", ExpiresAtMs: 10, AttemptsLeft: 5})
	_ = db.CreateInvitation(ctx, Invitation{ID: "b", TokenSHA256: "tb", CodeSHA256: "cb", ExpiresAtMs: 20, AttemptsLeft: 5})
	inv, err := db.LiveInvitation(ctx)
	if err != nil || inv.ID != "b" {
		t.Fatalf("live invitation %+v %v", inv, err)
	}
	inv.AttemptsLeft = 4
	_ = db.UpdateInvitation(ctx, inv)
	inv, _ = db.LiveInvitation(ctx)
	if inv.AttemptsLeft != 4 {
		t.Fatal("attempts not stored")
	}
	_ = db.CancelInvitations(ctx)
	if _, err := db.LiveInvitation(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatal("cancel did not take")
	}
	sec := "favorites"
	if err := db.SaveFocus(ctx, FocusMemory{Screen: "home", SectionID: &sec, ItemID: nil, ScrollX: 1.5, UpdatedAt: "t"}); err != nil {
		t.Fatal(err)
	}
	item := "youtube"
	_ = db.SaveFocus(ctx, FocusMemory{Screen: "home", SectionID: &sec, ItemID: &item, ScrollX: 2, UpdatedAt: "t2"})
	f, err := db.Focus(ctx, "home")
	if err != nil || f.ItemID == nil || *f.ItemID != "youtube" || f.ScrollX != 2 {
		t.Fatalf("focus %+v %v", f, err)
	}
	e := "boom"
	_ = db.SaveAppState(ctx, AppState{AppID: "plex-htpc", LastError: &e})
	s, err := db.AppState(ctx, "plex-htpc")
	if err != nil || s.LastError == nil || *s.LastError != "boom" || s.LastLaunchAt != nil {
		t.Fatalf("app state %+v %v", s, err)
	}
}

func TestOpenFileCreatesDirAndPersists(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "state.db")
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.CreateDevice(context.Background(), Device{ID: "d", Name: "n", Permissions: []string{"controller"}, CreatedAt: "c"})
	_ = db.Close()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("db mode %v", info.Mode().Perm())
	}
	db2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if l, _ := db2.ListDevices(context.Background()); len(l) != 1 {
		t.Fatal("data not persisted")
	}
	if _, err := db2.sql.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	_ = db2.Close()
	if _, err := Open(p); err == nil {
		t.Fatal("newer schema accepted")
	}
}

// schemaV1 is the schema as release 1 shipped it, frozen here so the
// upgrade is tested from the real v1 shape even if migrations change later.
const schemaV1 = `CREATE TABLE devices (id TEXT PRIMARY KEY, name TEXT NOT NULL, permissions TEXT NOT NULL, created_at TEXT NOT NULL, revoked_at TEXT, last_seen_ms INTEGER NOT NULL DEFAULT 0);
CREATE TABLE sessions (token_sha256 TEXT PRIMARY KEY, device_id TEXT NOT NULL REFERENCES devices(id), csrf_sha256 TEXT NOT NULL, created_at TEXT NOT NULL, last_seen_ms INTEGER NOT NULL DEFAULT 0);
CREATE INDEX sessions_device ON sessions(device_id);
CREATE TABLE invitations (id TEXT PRIMARY KEY, token_sha256 TEXT NOT NULL, code_sha256 TEXT NOT NULL, expires_at_ms INTEGER NOT NULL, attempts_left INTEGER NOT NULL, redeemed_at TEXT, cancelled INTEGER NOT NULL DEFAULT 0);
CREATE TABLE focus_memory (screen TEXT PRIMARY KEY, section_id TEXT, item_id TEXT, scroll_x REAL NOT NULL DEFAULT 0, updated_at TEXT NOT NULL);
CREATE TABLE app_state (app_id TEXT PRIMARY KEY, last_launch_at TEXT, last_error TEXT);
INSERT INTO devices VALUES ('dev_fam', 'Family phone', 'controller,owner', '2026-01-01T00:00:00Z', NULL, 7);
INSERT INTO devices VALUES ('dev_old', 'Old phone', 'controller', '2026-01-02T00:00:00Z', '2026-02-01T00:00:00Z', 0);
INSERT INTO sessions VALUES ('tok', 'dev_fam', 'csrf', '2026-01-01T00:00:00Z', 7);
INSERT INTO invitations VALUES ('inv', 'th', 'ch', 99, 5, NULL, 0);
PRAGMA user_version = 1;`

// A database written by schema 1 opens as schema 2 with every paired device,
// its session and its permissions intact, and no expiry (family phones).
func TestMigrateV1ToV2KeepsDevices(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.db")
	raw, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(schemaV1); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if v, _ := db.Version(); v != 2 || SchemaVersion != 2 {
		t.Fatalf("version %d (SchemaVersion %d), want 2", v, SchemaVersion)
	}
	ctx := context.Background()
	list, err := db.ListDevices(ctx)
	if err != nil || len(list) != 1 || list[0].ID != "dev_fam" || list[0].ExpiresAtMs != nil || list[0].LastSeenMs != 7 {
		t.Fatalf("devices after migration: %+v %v", list, err)
	}
	if len(list[0].Permissions) != 2 || list[0].Permissions[1] != "owner" {
		t.Fatalf("permissions lost: %v", list[0].Permissions)
	}
	if old, err := db.Device(ctx, "dev_old"); err != nil || !old.Revoked() {
		t.Fatalf("revoked device changed: %+v %v", old, err)
	}
	if s, err := db.SessionByTokenHash(ctx, "tok"); err != nil || s.DeviceID != "dev_fam" {
		t.Fatalf("session lost: %+v %v", s, err)
	}
	if inv, err := db.LiveInvitation(ctx); err != nil || inv.Permissions != nil || inv.PassExpiresAtMs != nil {
		t.Fatalf("v1 invitation: %+v %v", inv, err)
	}
	// The new columns work on the migrated file.
	exp := int64(1790647200000)
	if err := db.CreateDevice(ctx, Device{ID: "dev_g", Name: "Guest", Permissions: []string{"guest"}, CreatedAt: "2026-09-28T00:00:00Z", ExpiresAtMs: &exp}); err != nil {
		t.Fatal(err)
	}
	if g, err := db.Device(ctx, "dev_g"); err != nil || g.ExpiresAtMs == nil || *g.ExpiresAtMs != exp {
		t.Fatalf("guest expiry not stored: %+v %v", g, err)
	}
	if err := db.CreateInvitation(ctx, Invitation{ID: "inv2", TokenSHA256: "t2", CodeSHA256: "c2", ExpiresAtMs: 200, AttemptsLeft: 5, Permissions: []string{"guest"}, PassExpiresAtMs: &exp}); err != nil {
		t.Fatal(err)
	}
	inv, err := db.LiveInvitation(ctx)
	if err != nil || len(inv.Permissions) != 1 || inv.Permissions[0] != "guest" || inv.PassExpiresAtMs == nil || *inv.PassExpiresAtMs != exp {
		t.Fatalf("guest invitation: %+v %v", inv, err)
	}
}

func TestSecretStores(t *testing.T) {
	ctx := context.Background()
	m := NewMemorySecrets()
	if _, err := m.Get(ctx, "x"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatal(err)
	}
	_ = m.Set(ctx, "x", "s")
	if v, _ := m.Get(ctx, "x"); v != "s" {
		t.Fatal("secret lost")
	}
	_ = m.Delete(ctx, "x")
	if _, err := m.Get(ctx, "x"); err == nil {
		t.Fatal("delete failed")
	}
	u := UnavailableSecrets{Reason: "keyring locked"}
	if ok, why := u.Available(); ok || why != "keyring locked" {
		t.Fatal("unavailable store claims availability")
	}
	if err := u.Set(ctx, "x", "y"); !errors.Is(err, ErrSecretsUnavailable) {
		t.Fatal(err)
	}
}
