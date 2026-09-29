// Package storage is the coordinator's durable state: paired devices, cookie
// sessions (hashes only), pairing invitations (hashes only), shell focus
// memory, and per-application launch state, in one SQLite file. Schema changes
// are forward-only migrations keyed by SchemaVersion. It also defines the
// SecretStore seam for connector tokens; a keyring-backed implementation is
// not part of this package.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// SchemaVersion is the current user_version; migrations run up to it.
const SchemaVersion = 2

// migrations[i] upgrades user_version i to i+1.
var migrations = []string{
	`CREATE TABLE devices (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		permissions TEXT NOT NULL,
		created_at TEXT NOT NULL,
		revoked_at TEXT,
		last_seen_ms INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE sessions (
		token_sha256 TEXT PRIMARY KEY,
		device_id TEXT NOT NULL REFERENCES devices(id),
		csrf_sha256 TEXT NOT NULL,
		created_at TEXT NOT NULL,
		last_seen_ms INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX sessions_device ON sessions(device_id);
	CREATE TABLE invitations (
		id TEXT PRIMARY KEY,
		token_sha256 TEXT NOT NULL,
		code_sha256 TEXT NOT NULL,
		expires_at_ms INTEGER NOT NULL,
		attempts_left INTEGER NOT NULL,
		redeemed_at TEXT,
		cancelled INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE focus_memory (
		screen TEXT PRIMARY KEY,
		section_id TEXT,
		item_id TEXT,
		scroll_x REAL NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE app_state (
		app_id TEXT PRIMARY KEY,
		last_launch_at TEXT,
		last_error TEXT
	);`,
	// 2: guest passes. A device may carry an expiry (NULL for family phones;
	// the device is revoked when it passes); an invitation stores the
	// permissions it grants and, for a guest pass, when that pass will end.
	// Existing rows keep NULL: every device paired before stays a family phone.
	`ALTER TABLE devices ADD COLUMN expires_at_ms INTEGER;
	ALTER TABLE invitations ADD COLUMN permissions TEXT;
	ALTER TABLE invitations ADD COLUMN pass_expires_at_ms INTEGER;`,
}

// DefaultPath is $XDG_DATA_HOME/bear-den-tv/state.db (or ~/.local/share).
func DefaultPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "bear-den-tv", "state.db"), nil
}

// DB is an open state database. Methods are safe for concurrent use; the
// connection pool is limited to one connection so writes serialize and
// ":memory:" databases stay one database.
type DB struct {
	sql *sql.DB
}

// Open opens or creates the database at path (":memory:" for tests) and runs
// pending migrations. The parent directory is created with mode 0700.
func Open(path string) (*DB, error) {
	dsn := path
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	} else {
		dsn = "file::memory:?_pragma=foreign_keys(1)"
	}
	h, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	h.SetMaxOpenConns(1)
	db := &DB{sql: h}
	if err := db.migrate(); err != nil {
		_ = h.Close()
		return nil, err
	}
	if path != ":memory:" {
		_ = os.Chmod(path, 0o600)
	}
	return db, nil
}

// Close releases the database.
func (d *DB) Close() error { return d.sql.Close() }

// Version returns the schema user_version.
func (d *DB) Version() (int, error) {
	var v int
	err := d.sql.QueryRow("PRAGMA user_version").Scan(&v)
	return v, err
}

func (d *DB) migrate() error {
	v, err := d.Version()
	if err != nil {
		return err
	}
	if v > SchemaVersion {
		return fmt.Errorf("storage: database schema %d is newer than this build (%d)", v, SchemaVersion)
	}
	for v < SchemaVersion {
		tx, err := d.sql.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("storage: migration %d: %w", v+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		v++
	}
	return nil
}

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("storage: not found")

// Device is one paired device. Permissions are stored as a comma-separated
// list in rank-independent order. ExpiresAtMs (Unix epoch ms, wall clock) is
// set for guest passes only.
type Device struct {
	ID          string
	Name        string
	Permissions []string
	CreatedAt   string
	RevokedAt   *string
	LastSeenMs  int64
	ExpiresAtMs *int64
}

// Revoked reports whether the device has been revoked.
func (d Device) Revoked() bool { return d.RevokedAt != nil }

// CreateDevice inserts a device record.
func (d *DB) CreateDevice(ctx context.Context, dev Device) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO devices (id, name, permissions, created_at, revoked_at, last_seen_ms, expires_at_ms) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		dev.ID, dev.Name, strings.Join(dev.Permissions, ","), dev.CreatedAt, dev.RevokedAt, dev.LastSeenMs, dev.ExpiresAtMs)
	return err
}

// Device returns one device or ErrNotFound.
func (d *DB) Device(ctx context.Context, id string) (Device, error) {
	row := d.sql.QueryRowContext(ctx, `SELECT id, name, permissions, created_at, revoked_at, last_seen_ms, expires_at_ms FROM devices WHERE id = ?`, id)
	return scanDevice(row)
}

// ListDevices returns every device that has not been revoked, oldest first.
func (d *DB) ListDevices(ctx context.Context) ([]Device, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT id, name, permissions, created_at, revoked_at, last_seen_ms, expires_at_ms FROM devices WHERE revoked_at IS NULL ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		dev, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dev)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanDevice(s scanner) (Device, error) {
	var dev Device
	var perms string
	err := s.Scan(&dev.ID, &dev.Name, &perms, &dev.CreatedAt, &dev.RevokedAt, &dev.LastSeenMs, &dev.ExpiresAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, err
	}
	if perms != "" {
		dev.Permissions = strings.Split(perms, ",")
	}
	return dev, nil
}

// RevokeDevice marks a device revoked and deletes its sessions. Revoking "*"
// revokes every device. It returns the ids revoked.
func (d *DB) RevokeDevice(ctx context.Context, id, at string) ([]string, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var rows *sql.Rows
	if id == "*" {
		rows, err = tx.QueryContext(ctx, `SELECT id FROM devices WHERE revoked_at IS NULL`)
	} else {
		rows, err = tx.QueryContext(ctx, `SELECT id FROM devices WHERE revoked_at IS NULL AND id = ?`, id)
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var x string
		if err := rows.Scan(&x); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, x := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET revoked_at = ? WHERE id = ?`, at, x); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE device_id = ?`, x); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if id != "*" && len(ids) == 0 {
		return nil, ErrNotFound
	}
	return ids, nil
}

// TouchDevice records the last activity time.
func (d *DB) TouchDevice(ctx context.Context, id string, lastSeenMs int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE devices SET last_seen_ms = ? WHERE id = ?`, lastSeenMs, id)
	return err
}

// SetPermissions replaces a device's permissions.
func (d *DB) SetPermissions(ctx context.Context, id string, permissions []string) error {
	res, err := d.sql.ExecContext(ctx, `UPDATE devices SET permissions = ? WHERE id = ? AND revoked_at IS NULL`, strings.Join(permissions, ","), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Session is one cookie session; only hashes are stored.
type Session struct {
	TokenSHA256 string
	DeviceID    string
	CSRFSHA256  string
	CreatedAt   string
	LastSeenMs  int64
}

// CreateSession inserts a session.
func (d *DB) CreateSession(ctx context.Context, s Session) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO sessions (token_sha256, device_id, csrf_sha256, created_at, last_seen_ms) VALUES (?, ?, ?, ?, ?)`,
		s.TokenSHA256, s.DeviceID, s.CSRFSHA256, s.CreatedAt, s.LastSeenMs)
	return err
}

// SessionByTokenHash resolves a session or ErrNotFound.
func (d *DB) SessionByTokenHash(ctx context.Context, tokenSHA256 string) (Session, error) {
	var s Session
	err := d.sql.QueryRowContext(ctx, `SELECT token_sha256, device_id, csrf_sha256, created_at, last_seen_ms FROM sessions WHERE token_sha256 = ?`, tokenSHA256).
		Scan(&s.TokenSHA256, &s.DeviceID, &s.CSRFSHA256, &s.CreatedAt, &s.LastSeenMs)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return s, err
}

// DeleteSession removes one session.
func (d *DB) DeleteSession(ctx context.Context, tokenSHA256 string) error {
	res, err := d.sql.ExecContext(ctx, `DELETE FROM sessions WHERE token_sha256 = ?`, tokenSHA256)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchSession records session activity.
func (d *DB) TouchSession(ctx context.Context, tokenSHA256 string, lastSeenMs int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE sessions SET last_seen_ms = ? WHERE token_sha256 = ?`, lastSeenMs, tokenSHA256)
	return err
}

// Invitation is one pairing invitation; only hashes are stored. Permissions
// are what the redeeming device gets (nil in rows from schema 1: controller);
// PassExpiresAtMs is set for a guest pass.
type Invitation struct {
	ID              string
	TokenSHA256     string
	CodeSHA256      string
	ExpiresAtMs     int64
	AttemptsLeft    int
	RedeemedAt      *string
	Cancelled       bool
	Permissions     []string
	PassExpiresAtMs *int64
}

// CreateInvitation inserts an invitation after cancelling every live one so
// at most one invitation is live.
func (d *DB) CreateInvitation(ctx context.Context, inv Invitation) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE invitations SET cancelled = 1 WHERE redeemed_at IS NULL AND cancelled = 0`); err != nil {
		return err
	}
	var perms *string
	if inv.Permissions != nil {
		p := strings.Join(inv.Permissions, ",")
		perms = &p
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO invitations (id, token_sha256, code_sha256, expires_at_ms, attempts_left, redeemed_at, cancelled, permissions, pass_expires_at_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.TokenSHA256, inv.CodeSHA256, inv.ExpiresAtMs, inv.AttemptsLeft, inv.RedeemedAt, boolInt(inv.Cancelled), perms, inv.PassExpiresAtMs); err != nil {
		return err
	}
	return tx.Commit()
}

// LiveInvitation returns the invitation that is neither redeemed nor
// cancelled (expiry is judged by the caller), or ErrNotFound.
func (d *DB) LiveInvitation(ctx context.Context) (Invitation, error) {
	var inv Invitation
	var cancelled int
	var perms *string
	err := d.sql.QueryRowContext(ctx, `SELECT id, token_sha256, code_sha256, expires_at_ms, attempts_left, redeemed_at, cancelled, permissions, pass_expires_at_ms FROM invitations WHERE redeemed_at IS NULL AND cancelled = 0 ORDER BY expires_at_ms DESC LIMIT 1`).
		Scan(&inv.ID, &inv.TokenSHA256, &inv.CodeSHA256, &inv.ExpiresAtMs, &inv.AttemptsLeft, &inv.RedeemedAt, &cancelled, &perms, &inv.PassExpiresAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return Invitation{}, ErrNotFound
	}
	inv.Cancelled = cancelled != 0
	if perms != nil && *perms != "" {
		inv.Permissions = strings.Split(*perms, ",")
	}
	return inv, err
}

// UpdateInvitation stores attempts, redemption, and cancellation.
func (d *DB) UpdateInvitation(ctx context.Context, inv Invitation) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE invitations SET attempts_left = ?, redeemed_at = ?, cancelled = ? WHERE id = ?`,
		inv.AttemptsLeft, inv.RedeemedAt, boolInt(inv.Cancelled), inv.ID)
	return err
}

// CancelInvitations cancels every live invitation.
func (d *DB) CancelInvitations(ctx context.Context) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE invitations SET cancelled = 1 WHERE redeemed_at IS NULL AND cancelled = 0`)
	return err
}

// FocusMemory is the shell's last focus on one screen, by stable ids.
type FocusMemory struct {
	Screen    string
	SectionID *string
	ItemID    *string
	ScrollX   float64
	UpdatedAt string
}

// SaveFocus upserts focus memory for a screen.
func (d *DB) SaveFocus(ctx context.Context, f FocusMemory) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO focus_memory (screen, section_id, item_id, scroll_x, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(screen) DO UPDATE SET section_id = excluded.section_id, item_id = excluded.item_id, scroll_x = excluded.scroll_x, updated_at = excluded.updated_at`,
		f.Screen, f.SectionID, f.ItemID, f.ScrollX, f.UpdatedAt)
	return err
}

// Focus returns the remembered focus for a screen or ErrNotFound.
func (d *DB) Focus(ctx context.Context, screen string) (FocusMemory, error) {
	var f FocusMemory
	err := d.sql.QueryRowContext(ctx, `SELECT screen, section_id, item_id, scroll_x, updated_at FROM focus_memory WHERE screen = ?`, screen).
		Scan(&f.Screen, &f.SectionID, &f.ItemID, &f.ScrollX, &f.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FocusMemory{}, ErrNotFound
	}
	return f, err
}

// AppState is durable per-application launch bookkeeping.
type AppState struct {
	AppID        string
	LastLaunchAt *string
	LastError    *string
}

// SaveAppState upserts an application's state.
func (d *DB) SaveAppState(ctx context.Context, s AppState) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO app_state (app_id, last_launch_at, last_error) VALUES (?, ?, ?)
		ON CONFLICT(app_id) DO UPDATE SET last_launch_at = excluded.last_launch_at, last_error = excluded.last_error`,
		s.AppID, s.LastLaunchAt, s.LastError)
	return err
}

// AppState returns an application's state or ErrNotFound.
func (d *DB) AppState(ctx context.Context, appID string) (AppState, error) {
	var s AppState
	err := d.sql.QueryRowContext(ctx, `SELECT app_id, last_launch_at, last_error FROM app_state WHERE app_id = ?`, appID).
		Scan(&s.AppID, &s.LastLaunchAt, &s.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return AppState{}, ErrNotFound
	}
	return s, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
