// Browser tests of the navigation script (src/nav.ts) against local fixture
// pages that mimic streaming layouts: poster rows, a details overlay, a
// player with the site's own keyboard shortcuts, a search box. The Driver
// injects dist/nav.js the way the coordinator does (CDP isolated world +
// binding) and performs effects with trusted input. Run `npm run build`
// first (make test-webnav does). No real streaming site is ever loaded.
import { expect, test } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import Ajv2020 from 'ajv/dist/2020.js';
import { Driver, loadHints } from './driver';

const here = dirname(fileURLToPath(import.meta.url));

test.describe('spatial navigation on a poster grid', () => {
  test('starts top-left, stays in a row, and scrolls a row sideways', async ({ page }) => {
    const d = await Driver.attach(page);
    await d.open('grid.html');
    const first = await d.act('nav.right');
    expect(first.outcome).toBe('moved');
    expect(await d.focused()).toBe('nav-home');
    await d.act('nav.down');
    expect(await d.focused()).toBe('r1c1');
    await d.act('nav.right');
    await d.act('nav.right');
    expect(await d.focused()).toBe('r1c3');
    await d.act('nav.down');
    expect(await d.focused()).toBe('r2c3'); // the card below, not the nearest centre
    for (let i = 0; i < 6; i++) await d.act('nav.right');
    expect(await d.focused()).toBe('r2c9');
    const inView = await page.evaluate(() => {
      const r = document.getElementById('r2c9')!.getBoundingClientRect();
      return r.left >= 0 && r.right <= innerWidth;
    });
    expect(inView).toBe(true);
    const edge = await d.act('nav.right');
    expect(edge.outcome).toBe('edge');
    await d.act('nav.down');
    expect(await d.focused()).toMatch(/^r3c/);
  });

  test('a cursor:pointer card is one target, not its inner span too', async ({ page }) => {
    const d = await Driver.attach(page);
    await d.open('grid.html');
    await d.act('nav.right');
    await d.act('nav.down');
    await d.act('nav.down');
    expect(await d.focused()).toBe('r2c1');
    await d.act('nav.right');
    expect(await d.focused()).toBe('r2c2');
  });

  test('OK clicks with trusted input; focus stays inside an open overlay', async ({ page }) => {
    const d = await Driver.attach(page);
    await d.open('grid.html');
    await d.act('nav.right');
    await d.act('nav.down');
    await d.act('nav.down');
    await d.act('nav.right');
    const click = await d.act('select');
    expect(click.effect?.kind).toBe('click');
    expect(await d.body('clicked')).toBe('r2c2');
    await expect(page.locator('#details')).toBeVisible();
    await d.act('nav.right');
    expect(await d.focused()).toBe('play');
    await d.act('nav.right');
    await d.act('nav.right');
    expect(await d.focused()).toBe('close');
    const edge = await d.act('nav.right');
    expect(edge.outcome).toBe('edge');
    for (const dir of ['nav.up', 'nav.down', 'nav.left']) {
      await d.act(dir);
      expect(['play', 'more', 'close']).toContain(await d.focused());
    }
  });
});

test.describe('Back sequence', () => {
  test('text field, then overlay, then history, then at root', async ({ page }) => {
    const d = await Driver.attach(page);
    await d.open('grid.html');
    await d.act('nav.right');
    await d.act('nav.right');
    await d.act('nav.right');
    expect(await d.focused()).toBe('nav-search');
    await d.act('select');
    await page.waitForURL(/search\.html$/);
    await d.ready();
    await d.act('nav.right');
    expect(await d.focused()).toBe('q');
    expect((await d.act('select')).outcome).toBe('text_field');
    expect((await d.act('back')).outcome).toBe('left_field');
    const back = await d.act('back');
    expect(back.outcome).toBe('history_back');
    await page.waitForURL(/grid\.html$/);
    await d.ready();
    await d.act('nav.right');
    await d.act('nav.down');
    await d.act('select');
    await expect(page.locator('#details')).toBeVisible();
    const close = await d.act('back');
    expect(close.outcome).toBe('closing_overlay');
    await expect(page.locator('#details')).toBeHidden();
    expect(await d.body('clicked')).toBe('close');
    expect((await d.act('back')).outcome).toBe('at_root');
  });

  test('fullscreen is left first', async ({ page }) => {
    const d = await Driver.attach(page);
    await d.open('player.html');
    await page.keyboard.press('f');
    await expect.poll(() => page.evaluate(() => !!document.fullscreenElement)).toBe(true);
    expect((await d.act('back')).outcome).toBe('exited_fullscreen');
    await expect.poll(() => page.evaluate(() => !!document.fullscreenElement)).toBe(false);
  });
});

test.describe('media through the site shortcuts', () => {
  test('pause, play and seek press keys and never set currentTime', async ({ page }) => {
    const d = await Driver.attach(page);
    await d.open('player.html');
    await expect.poll(() => d.evaluate<string>('__bdtv.status().video')).toBe('playing');
    // Focus a button first: a space would press it instead of pausing.
    await d.act('nav.right');
    expect(await d.focused()).toBe('back-to-browse');
    const pause = await d.act('media.pause');
    expect(pause.effect).toEqual({ kind: 'keys', keys: [' '] });
    await expect.poll(() => d.evaluate<string>('__bdtv.status().video')).toBe('paused');
    expect(await d.body('lastKeyTrusted')).toBe('true');
    expect((await d.act('media.pause')).outcome).toBe('already_paused');
    await d.act('media.play');
    await expect.poll(() => d.evaluate<string>('__bdtv.status().video')).toBe('playing');
    const seek = await d.act('media.seek_relative', 30);
    expect(seek.effect?.keys).toEqual(['ArrowRight', 'ArrowRight', 'ArrowRight']);
    expect(await d.body('seeks')).toBe('30');
    await d.act('media.seek_relative', -10);
    expect(await d.body('seeks')).toBe('20');
    expect(await d.body('seekError')).toBeUndefined();
    expect(d.reports.some((r) => r.video === 'paused')).toBe(true);
  });

  test('a page without a video says so', async ({ page }) => {
    const d = await Driver.attach(page);
    await d.open('grid.html');
    const r = await d.act('media.pause');
    expect(r.ok).toBe(false);
    expect(r.reason).toBe('No video on this page.');
  });
});

test.describe('text entry', () => {
  test('types the phone text into the focused search box and submits', async ({ page }) => {
    const d = await Driver.attach(page);
    await d.open('search.html');
    await d.act('nav.down');
    expect(await d.focused()).toBe('q');
    await d.act('select');
    await expect.poll(() => d.reports.some((r) => r.text_field === true)).toBe(true);
    await page.fill('#q', 'old'); // a value that must be replaced, not appended to
    const prep = await d.act('text.prepare', undefined, 'gam');
    expect(prep.effect).toEqual({ kind: 'text' });
    expect(await d.body('submitted')).toBe('gam');
    await expect(page.locator('.result')).toHaveCount(1);
    expect((await d.act('back')).outcome).toBe('left_field');
    await d.act('nav.down');
    expect(await d.focused()).toBe('res-gamma');
  });

  test('text.prepare without a focused field fails closed', async ({ page }) => {
    const d = await Driver.attach(page);
    await d.open('grid.html');
    const r = await d.act('text.prepare');
    expect(r.ok).toBe(false);
    expect(r.effect).toBeUndefined();
  });
});

test.describe('per-site hints', () => {
  test('without hints the quiet tile is unreachable and the promo is a target', async ({ page }) => {
    const d = await Driver.attach(page);
    await d.open('search.html');
    await d.act('nav.down');
    await d.act('nav.down');
    await d.act('nav.down');
    expect(await d.focused()).toBe('promo');
    const seen: (string | null)[] = [];
    for (let i = 0; i < 5; i++) {
      await d.act('nav.right');
      seen.push(await d.focused());
    }
    expect(seen).not.toContain('quiet-tile');
  });

  test('prefer finds the quiet tile, skip hides the promo', async ({ page }) => {
    const d = await Driver.attach(page, { id: 'fixture', verified: false, prefer: ['.hidden-tile'], skip: ['.promo'] });
    await d.open('search.html');
    await d.act('nav.down');
    await d.act('nav.down');
    await d.act('nav.down');
    expect(await d.focused()).toBe('quiet-tile');
  });

  test('a shipped hint file whose selectors match nothing leaves the generic behaviour', async ({ page }) => {
    const d = await Driver.attach(page, loadHints('netflix'));
    await d.open('grid.html');
    await d.act('nav.right');
    await d.act('nav.down');
    expect(await d.focused()).toBe('r1c1');
  });

  test('a hint selector that does not parse is ignored', async ({ page }) => {
    const d = await Driver.attach(page, { id: 'fixture', verified: false, prefer: ['[[['], skip: ['::nope('] });
    await d.open('grid.html');
    await d.act('nav.right');
    expect(await d.focused()).toBe('nav-home');
  });

  test('every shipped hint file matches contracts/web-hints.schema.json and is UNVERIFIED', () => {
    const schema = JSON.parse(readFileSync(join(here, '../../../contracts/web-hints.schema.json'), 'utf8'));
    const validate = new Ajv2020({ strict: false }).compile(schema);
    for (const id of ['netflix', 'disney-plus', 'hulu']) {
      const h = loadHints(id) as { id: string; verified: boolean };
      expect(validate(h), `${id}: ${JSON.stringify(validate.errors)}`).toBe(true);
      expect(h.id).toBe(id);
      expect(h.verified).toBe(false);
    }
    expect(validate({ id: 'x', verified: false, media: { toggle: 'q' } })).toBe(false);
  });
});

test('the focus ring is drawn over the focused card', async ({ page }) => {
  const d = await Driver.attach(page);
  await d.open('grid.html');
  await d.act('nav.right');
  await d.act('nav.down');
  await d.act('nav.right');
  await page.waitForTimeout(50);
  if (process.env.BDTV_SHOT_DIR) await page.screenshot({ path: join(process.env.BDTV_SHOT_DIR, 'web-nav-focus-ring.png') });
  const hostCount = await page.locator('[data-bdtv-overlay]').count();
  expect(hostCount).toBe(1);
});

// The focus ring over a playing video (TV test 2026-09-29 item 1): it fades
// 3 s after the last key and comes back on the next one; with the video
// paused it stays. The fixture's video plays a canvas stream, so it truly
// plays headless; time moves by CDP virtual time, not by sleeping.
test('the focus ring fades over a playing video and returns on the next key', async ({ page }) => {
  const d = await Driver.attach(page);
  await d.open('player.html');
  await expect.poll(() => page.evaluate(() => !(document.getElementById('video') as HTMLVideoElement).paused)).toBe(true);
  expect((await d.act('nav.down')).ok).toBe(true);
  expect(await d.ringOpacity()).toBe('1');
  await d.advance(2900);
  expect(await d.ringOpacity()).toBe('1');
  await d.advance(200);
  expect(await d.ringOpacity()).toBe('0');
  // The next key brings it back, and it fades again 3 s later.
  await d.act('nav.right');
  expect(await d.ringOpacity()).toBe('1');
  await d.advance(3100);
  expect(await d.ringOpacity()).toBe('0');
  // Paused: the ring stays.
  await page.evaluate(() => (document.getElementById('video') as HTMLVideoElement).pause());
  await d.act('nav.left');
  expect(await d.ringOpacity()).toBe('1');
  await d.advance(3100);
  expect(await d.ringOpacity()).toBe('1');
});
