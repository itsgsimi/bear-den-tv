// Tests for the Den badge rows (achievements.go, migration 3): the v2 → v3
// upgrade keeps paired phones and guest passes, the tables hold only the
// columns docs/security.md#den-badges names, and their CHECKs refuse titles
// and times.

package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
)

// schemaV2 is the schema as release 2 (guest passes) shipped it, with rows,
// frozen so the upgrade is tested from the real v2 shape.
const schemaV2 = schemaV1 + `
ALTER TABLE devices ADD COLUMN expires_at_ms INTEGER;
ALTER TABLE invitations ADD COLUMN permissions TEXT;
ALTER TABLE invitations ADD COLUMN pass_expires_at_ms INTEGER;
INSERT INTO devices VALUES ('dev_guest', 'Visitor', 'guest', '2026-09-27T20:00:00Z', NULL, 9, 1790647200000);
UPDATE invitations SET permissions = 'guest', pass_expires_at_ms = 1790647200000 WHERE id = 'inv';
INSERT INTO focus_memory VALUES ('home', 'favorites', 'youtube', 0, '2026-09-27T20:00:00Z');
PRAGMA user_version = 2;`

func openV2(t *testing.T) *DB {
	t.Helper()
	p := filepath.Join(t.TempDir(), "state.db")
	raw, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(schemaV2); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// A schema 2 database opens as schema 3 with every family phone, guest pass
// (and its end), session, invitation and focus row intact, and empty badge
// tables that work.
func TestMigrateV2ToV3KeepsDevicesAndGuestPasses(t *testing.T) {
	db := openV2(t)
	ctx := context.Background()
	if v, _ := db.Version(); v != 3 || SchemaVersion != 3 {
		t.Fatalf("version %d (SchemaVersion %d), want 3", v, SchemaVersion)
	}
	list, err := db.ListDevices(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("devices after migration: %+v %v", list, err)
	}
	if list[0].ID != "dev_fam" || list[0].ExpiresAtMs != nil || len(list[0].Permissions) != 2 {
		t.Fatalf("family phone changed: %+v", list[0])
	}
	if list[1].ID != "dev_guest" || list[1].ExpiresAtMs == nil || *list[1].ExpiresAtMs != 1790647200000 || list[1].Permissions[0] != "guest" {
		t.Fatalf("guest pass changed: %+v", list[1])
	}
	if s, err := db.SessionByTokenHash(ctx, "tok"); err != nil || s.DeviceID != "dev_fam" {
		t.Fatalf("session lost: %+v %v", s, err)
	}
	if inv, err := db.LiveInvitation(ctx); err != nil || len(inv.Permissions) != 1 || inv.PassExpiresAtMs == nil {
		t.Fatalf("guest invitation lost: %+v %v", inv, err)
	}
	if f, err := db.Focus(ctx, "home"); err != nil || f.ItemID == nil || *f.ItemID != "youtube" {
		t.Fatalf("focus memory lost: %+v %v", f, err)
	}
	if c, err := db.Counters(ctx); err != nil || len(c) != 0 {
		t.Fatalf("badge counters after migration: %v %v", c, err)
	}
	day := "2026-09-28"
	if err := db.SaveCounter(ctx, Counter{Name: "launches", Count: 1, FirstDay: &day, LastDay: &day}); err != nil {
		t.Fatal(err)
	}
	if fresh, err := db.EarnBadge(ctx, "first-night-in", day); err != nil || !fresh {
		t.Fatalf("earn: %v %v", fresh, err)
	}
}

// The badge tables have exactly these columns: names, counts and days. A
// new column is a privacy decision (docs/security.md#den-badges), not a
// drive-by change.
func TestAchievementTablesHoldOnlyIdsCountsAndDays(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cols := func(table string) []string {
		rows, err := db.sql.Query(`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var n string
			_ = rows.Scan(&n)
			out = append(out, n)
		}
		return out
	}
	if got := cols("achievement_counters"); !reflect.DeepEqual(got, []string{"name", "count", "first_day", "last_day"}) {
		t.Fatalf("achievement_counters columns %v", got)
	}
	if got := cols("achievement_badges"); !reflect.DeepEqual(got, []string{"id", "earned_day", "celebrated"}) {
		t.Fatalf("achievement_badges columns %v", got)
	}
	ctx := context.Background()
	day, when := "2026-09-28", "2026-09-28T23:14:00Z"
	for _, c := range []Counter{
		{Name: "launch:DEMO Movie Night", Count: 1},     // a title as a name
		{Name: "app:plex htpc", Count: 1},               // a space
		{Name: "Launches", Count: 1},                    // capitals
		{Name: "launches", Count: 1, LastDay: &when},    // a time
		{Name: "launches", Count: -1},                   // a negative count
		{Name: "launches-launches-launches-launches-lau", Count: 1, FirstDay: &when},
	} {
		if err := db.SaveCounter(ctx, c); err == nil {
			t.Fatalf("stored %+v", c)
		}
	}
	if err := db.SaveCounter(ctx, Counter{Name: "launch:plex-htpc", Count: 3, FirstDay: &day, LastDay: &day}); err != nil {
		t.Fatalf("a real counter was refused: %v", err)
	}
	if _, err := db.EarnBadge(ctx, "first-night-in", when); err == nil {
		t.Fatal("an earned time was stored")
	}
	if _, err := db.EarnBadge(ctx, "DEMO Movie", day); err == nil {
		t.Fatal("a title was stored as a badge id")
	}
}

func TestEarnOnceCelebrateAndReset(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()
	ctx := context.Background()
	if fresh, _ := db.EarnBadge(ctx, "good-host", "2026-09-01"); !fresh {
		t.Fatal("first earn not fresh")
	}
	if fresh, _ := db.EarnBadge(ctx, "good-host", "2026-09-02"); fresh {
		t.Fatal("a badge was earned twice")
	}
	_, _ = db.EarnBadge(ctx, "first-night-in", "2026-08-30")
	list, err := db.EarnedBadges(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "first-night-in" || list[1].Day != "2026-09-01" || list[1].Celebrated {
		t.Fatalf("earned %+v %v", list, err)
	}
	_ = db.MarkCelebrated(ctx, []string{"good-host", "unknown"})
	list, _ = db.EarnedBadges(ctx)
	if !list[1].Celebrated || list[0].Celebrated {
		t.Fatalf("celebrated %+v", list)
	}
	day := "2026-09-01"
	_ = db.SaveCounter(ctx, Counter{Name: "days", Count: 4, FirstDay: &day, LastDay: &day})
	if err := db.ResetAchievements(ctx); err != nil {
		t.Fatal(err)
	}
	if c, _ := db.Counters(ctx); len(c) != 0 {
		t.Fatalf("counters after reset %v", c)
	}
	if l, _ := db.EarnedBadges(ctx); len(l) != 0 {
		t.Fatalf("badges after reset %v", l)
	}
	if c, err := db.Counter(ctx, "days"); err != nil || c.Count != 0 || c.LastDay != nil {
		t.Fatalf("missing counter %+v %v", c, err)
	}
}
