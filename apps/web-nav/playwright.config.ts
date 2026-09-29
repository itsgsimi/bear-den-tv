// Playwright configuration for the navigation script: headless Chromium from
// Playwright, local fixture pages only (tests/fixtures, served through
// page.route, no web server, no real streaming site).
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: 'tests',
  testMatch: '*.spec.ts',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: [['list']],
  timeout: 20_000,
  use: {
    browserName: 'chromium',
    headless: true,
    viewport: { width: 1280, height: 720 },
  },
});
