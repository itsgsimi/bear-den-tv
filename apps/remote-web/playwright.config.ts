// Playwright configuration for the phone remote's browser tests (tests/e2e):
// the real coordinator, `bear-den-tv dev --dev-fixtures --no-shell`, is
// started by the tests themselves (tests/e2e/coordinator.ts) on a free
// loopback port with its own dev root, and serves the embedded dist/. One
// portrait phone; headless Playwright Chromium (the same version as
// apps/web-nav, so CI installs it once).
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: 'tests/e2e',
  testMatch: '*.spec.ts',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: [['list']],
  timeout: 30_000,
  use: {
    trace: 'retain-on-failure',
    ...devices['Desktop Chrome'],
    isMobile: true,
    hasTouch: true,
  },
  projects: [{ name: 'phone-portrait', use: { viewport: { width: 390, height: 844 } } }],
});
