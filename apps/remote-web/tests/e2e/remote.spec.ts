// The phone remote against the real coordinator (coordinator.ts: `bear-den-tv
// dev --dev-fixtures --no-shell`): pair with the code the TV side issues, see
// the DEMO apps, open Plex from its tile and find it in front in the
// coordinator's state, see the DEMO Now playing card, and press Right and
// find the key delivered to Plex's window in the coordinator's log. Every
// wait is on an event (a response, a log line, a DOM state), never a sleep.

import { test as base, expect } from '@playwright/test';
import { Coordinator } from './coordinator';

const test = base.extend<object, { coordinator: Coordinator }>({
  coordinator: [
    // eslint-disable-next-line no-empty-pattern
    async ({}, use) => {
      const c = await Coordinator.start();
      await use(c);
      await c.stop();
    },
    { scope: 'worker' },
  ],
});

test('pairs by code, opens an app and drives it', async ({ page, coordinator }) => {
  await page.goto(coordinator.baseURL + '/');

  // Pair by code, as from the TV's Pair a phone screen.
  const code = await coordinator.pairingCode();
  await page.getByLabel('Pairing code').fill(code);
  await page.getByLabel(/name/i).fill('Playwright phone');
  const claimed = page.waitForResponse((r) => r.url().endsWith('/api/v1/pair/claim'));
  await page.locator('form.pair-form button[type=submit]').click();
  expect((await claimed).status()).toBe(200);

  // The DEMO session and its apps.
  await expect(page.getByTestId('demo-badge')).toBeVisible();
  const plex = page.getByRole('button', { name: 'Open Plex' });
  await expect(plex).toBeVisible();
  await expect(page.getByRole('button', { name: 'Open YouTube' })).toBeVisible();

  // An app tile: the coordinator launches Plex and puts it in front.
  const launched = page.waitForResponse((r) => r.url().endsWith('/api/v1/actions') && (r.request().postData() ?? '').includes('"app.launch"'));
  await plex.click();
  const launch = await (await launched).json();
  expect(launch.outcome).not.toBe('failed');
  await expect
    .poll(async () => (await (await page.request.get(coordinator.baseURL + '/api/v1/state')).json()).target?.app_id)
    .toBe('plex-htpc');

  // The DEMO player in front shows on the phone.
  const np = page.getByTestId('now-playing');
  await expect(np).toBeVisible();
  await expect(np).toContainText('DEMO');

  // A nav button: the key reaches Plex's window on the coordinator's desktop.
  const right = page.getByRole('button', { name: 'Right', exact: true });
  await expect(right).toBeEnabled();
  const from = coordinator.mark();
  const pressed = page.waitForResponse((r) => r.url().endsWith('/api/v1/actions') && (r.request().postData() ?? '').includes('"nav.right"'));
  await right.click();
  const nav = await (await pressed).json();
  expect(nav.outcome).toBe('delivered');
  await coordinator.waitForLine(/msg="dev: key delivered" key=right/, from);
});
