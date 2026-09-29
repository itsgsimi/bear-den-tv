# ADR 0009 — Den badges from local counters, ids, counts and days only

Date: 2026-09-28. Status: accepted.

Bear Den replaces a smart TV that watched its owners. The owner's wish list
still had "achievements (needs stored data)": playful badges for using the
den. That means Bear Den has to count how it is used, for the first time.
Counting is exactly what a privacy-first TV must be careful with, so the
shape of what is stored and shown is fixed here.

## Options

| Option | For | Against |
|---|---|---|
| An event log (what happened, when) and badges computed from it | any new badge can be computed from history | a log of launches and times is a viewing diary; it grows forever; phones would need rules for what to hide |
| **Named counters with first and last local day, plus earned badges with the day** | a badge is a threshold on a number; nothing to trim; the stored rows cannot say what was watched or when during a day | a new badge starts from zero on existing TVs; "N different days" needs care with midnight, DST and clocks set back |
| Counting in the shell (QML, a local file) | no coordinator change | the shell cannot see launches from phones, pairing or guest passes; a second store beside `state.db` |

## Decision

1. **Counters in `state.db` (schema 3), owned by the coordinator.**
   `achievement_counters(name, count, first_day, last_day)` and
   `achievement_badges(id, earned_day, celebrated)`. Table CHECKs refuse
   anything but lowercase ids and `YYYY-MM-DD` days, so a title or a time
   cannot be stored even by a bug. `internal/achievements` moves counters at
   the event points that already exist (a launch that worked, Home shown, a
   phone paired, a guest pass issued, a sleep timer set, the parade the shell
   reports, a look chosen in Settings).
2. **Days are local calendar days on the injected clock.** A day counter
   moves at most once per day and never for a day earlier than the last one
   (a clock set back counts nothing). A night belongs to the evening it
   started (00:30 is still last night).
3. **The badge catalogue is data**: one table of `{id, counter, goal}` (or
   "every member of a set": installed apps, built-in themes, art styles,
   seasons). No code branches per badge; names, hints and art are keyed by
   id in the shell, the phone and the art tools.
4. **Only the shell and controller phones see badges** (`state.achievements`:
   ids, counts capped at the goal, earned days). Never guest passes, never
   while locked; the schema itself rejects both. `celebrate` (not yet shown
   on Home) is shell-only.
5. **One switch and one reset, on the TV.** Config `achievements.enabled`
   (default on) stops all counting; Reset deletes both tables' rows. Phones
   cannot change either.

## Consequences

- A badge added later counts from the day it ships; existing history cannot
  be replayed, by design.
- Movie Night counts Plex launches per adapter (`launch:plex-htpc`), and
  Couch Explorer which apps were opened at least once: the TV knows which
  apps exist and were used, as a count. It never knows what was played in
  them.
- Tests assert the table columns, the CHECKs and that a DEMO title in front
  never reaches the rows (`internal/storage/achievements_test.go`,
  `internal/session/achievements_test.go`).
