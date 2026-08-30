import path from 'path';
import { defineConfig } from 'vitest/config';

// Unit tests cover pure logic in src/utils and src/lib only — no jsdom/RTL
// needed, since none of the covered functions touch the DOM. UI behavior is
// covered by the Playwright e2e suite (e2e/playwright.config.ts) instead.
export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node',
  },
});
