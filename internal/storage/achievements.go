// Den badge rows (schema 3): counters and earned badges for
// internal/achievements. Only lowercase ids, counts and local calendar days
// are stored; the table CHECKs refuse anything else (docs/security.md#den-badges).

package storage

import (
	"context"
	"database/sql"
	"errors"
)

// Counter is one Den badge counter: how many times it moved and the first
// and last local day (YYYY-MM-DD) it did; days are nil until it first moves.
type Counter struct {
	Name     string
	Count    int
	FirstDay *string
	LastDay  *string
}

// EarnedBadge is one earned badge, the local day it was earned and whether
// the shell has celebrated it on Home.
type EarnedBadge struct {
	ID         string
	Day        string
	Celebrated bool
}

// Counters returns every counter by name.
func (d *DB) Counters(ctx context.Context) (map[string]Counter, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT name, count, first_day, last_day FROM achievement_counters`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Counter{}
	for rows.Next() {
		var c Counter
		if err := rows.Scan(&c.Name, &c.Count, &c.FirstDay, &c.LastDay); err != nil {
			return nil, err
		}
		out[c.Name] = c
	}
	return out, rows.Err()
}

// Counter returns one counter, or a zero counter with its name when it has
// never moved.
func (d *DB) Counter(ctx context.Context, name string) (Counter, error) {
	c := Counter{Name: name}
	err := d.sql.QueryRowContext(ctx, `SELECT count, first_day, last_day FROM achievement_counters WHERE name = ?`, name).
		Scan(&c.Count, &c.FirstDay, &c.LastDay)
	if errors.Is(err, sql.ErrNoRows) {
		return Counter{Name: name}, nil
	}
	return c, err
}

// SaveCounter upserts a counter.
func (d *DB) SaveCounter(ctx context.Context, c Counter) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO achievement_counters (name, count, first_day, last_day) VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET count = excluded.count, first_day = excluded.first_day, last_day = excluded.last_day`,
		c.Name, c.Count, c.FirstDay, c.LastDay)
	return err
}

// EarnBadge records a badge as earned on day, once: it reports false when
// the badge was already earned (the first day is kept).
func (d *DB) EarnBadge(ctx context.Context, id, day string) (bool, error) {
	res, err := d.sql.ExecContext(ctx, `INSERT INTO achievement_badges (id, earned_day, celebrated) VALUES (?, ?, 0) ON CONFLICT(id) DO NOTHING`, id, day)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// EarnedBadges returns every earned badge, oldest day first (then by id).
func (d *DB) EarnedBadges(ctx context.Context) ([]EarnedBadge, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT id, earned_day, celebrated FROM achievement_badges ORDER BY earned_day, rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EarnedBadge
	for rows.Next() {
		var b EarnedBadge
		var cel int
		if err := rows.Scan(&b.ID, &b.Day, &cel); err != nil {
			return nil, err
		}
		b.Celebrated = cel != 0
		out = append(out, b)
	}
	return out, rows.Err()
}

// MarkCelebrated marks earned badges as celebrated; unknown ids are ignored.
func (d *DB) MarkCelebrated(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if _, err := d.sql.ExecContext(ctx, `UPDATE achievement_badges SET celebrated = 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// ResetAchievements deletes every counter and every earned badge.
func (d *DB) ResetAchievements(ctx context.Context) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM achievement_counters`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM achievement_badges`); err != nil {
		return err
	}
	return tx.Commit()
}
