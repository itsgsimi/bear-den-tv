// Badges tab: the TV's shelf of Den badges, read-only (state.achievements;
// contracts/http.md "Den badges"). Shown only when the snapshot carries
// achievements, which the coordinator sends to controller phones and never to
// guest passes or while locked. Earned badges show their medal and the local
// day they were earned; the rest a silhouette, a hint and their progress.
// Names and hints are the phone's copy (i18n.ts), keyed by badge id; art is
// the same medals as the TV's (tools/pixelart and tools/classicart
// badges.py), in the TV's art style. Phones cannot change anything here.
import type { JSX } from 'preact';
import type { Achievements, ArtStyle, StateSnapshot } from '../contract.ts';
import { badgeDay, t } from '../i18n.ts';
import { artStyleOf } from '../icons.tsx';
import type { AppState } from '../state.ts';

/** One badge on the shelf, ready to draw. */
export interface ShelfBadge {
  id: string;
  name: string;
  hint: string;
  earned: boolean;
  /** "2 Sep 2026" when earned, else "". */
  day: string;
  count: number;
  goal: number;
  /** This phone has art for the id (a newer TV may know more badges). */
  known: boolean;
}

/**
 * @param snapshot Latest snapshot or null.
 * @returns state.achievements, or null when this phone may not see it.
 */
export function achievementsOf(snapshot: StateSnapshot | null): Achievements | null {
  return snapshot?.achievements ?? null;
}

/**
 * @param a state.achievements.
 * @returns Every badge in the TV's order, with the phone's copy.
 */
export function shelf(a: Achievements): ShelfBadge[] {
  const days = new Map(a.earned.map((e) => [e.id, e.day] as const));
  return a.progress.map((p) => {
    const copy = t.badges.names[p.id];
    const day = days.get(p.id);
    return {
      id: p.id,
      name: copy?.[0] ?? p.id,
      hint: copy?.[1] ?? '',
      earned: day !== undefined,
      day: day !== undefined ? badgeDay(day) : '',
      count: Math.min(p.count, p.goal),
      goal: p.goal,
      known: copy !== undefined,
    };
  });
}

/** A medal: pixel PNG at 2x (64 px) or classic SVG; the silhouette until earned. */
function Medal({ badge, art }: { badge: ShelfBadge; art: ArtStyle }): JSX.Element {
  const id = badge.known ? badge.id : 'first-night-in';
  const suffix = badge.earned && badge.known ? '' : '-locked';
  const src = art === 'classic' ? `art/badge-${id}${suffix}.svg` : `art/pixel/badge-${id}${suffix}.png`;
  return <img class={`art badge-art ${art === 'classic' ? 'art-classic' : ''}`} src={src} width={64} height={64} alt="" aria-hidden="true" draggable={false} />;
}

export function BadgesView({ state }: { state: AppState }): JSX.Element {
  const a = achievementsOf(state.snapshot);
  const art = artStyleOf(state.snapshot?.appearance);
  if (!a) {
    return (
      <section class="page badges">
        <h2>{t.badges.heading}</h2>
        <p class="muted">{t.badges.hidden}</p>
      </section>
    );
  }
  const items = shelf(a);
  const earned = items.filter((b) => b.earned).length;
  return (
    <section class="page badges" data-testid="badges">
      <h2>{t.badges.heading}</h2>
      <p class="muted" data-testid="badges-summary">
        {a.enabled ? t.badges.summary(earned, items.length) : t.badges.off}
      </p>
      <ul class="badge-grid">
        {items.map((b) => (
          <li key={b.id} class={`badge ${b.earned ? 'badge-earned' : 'badge-locked'}`} data-testid={`badge-${b.id}`}>
            <Medal badge={b} art={art} />
            <span class="badge-name">{b.name}</span>
            <span class="badge-detail small">{b.earned ? t.badges.earnedOn(b.day) : t.badges.progress(b.count, b.goal)}</span>
            {!b.earned && b.hint ? <span class="badge-hint small muted">{b.hint}</span> : null}
          </li>
        ))}
      </ul>
      <p class="muted small">{t.badges.privacy}</p>
    </section>
  );
}
