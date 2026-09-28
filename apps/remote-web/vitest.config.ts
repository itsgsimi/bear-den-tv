// Vitest configuration: unit and contract tests run under Node with fake timers
// and injected fakes; browser behaviour is covered by Playwright (tests/e2e).
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'node',
    include: ['tests/unit/**/*.spec.ts', 'tests/contract.spec.ts'],
  },
});
