import { defineConfig, devices } from '@playwright/test';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  testDir: './tests',
  testMatch: '**/*.spec.ts',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [['html', { outputFolder: 'playwright-report' }], ['list']],
  timeout: 30000,
  expect: { timeout: 10000 },

  use: {
    baseURL: 'http://localhost:9246',
    testIdAttribute: 'data-testid',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    // Pinned rather than left to the device default: ResizablePanel
    // auto-collapses the sidebar at innerWidth <= 600 (ResizablePanel.tsx),
    // and the terminal panel's default height is derived once from
    // window.innerHeight at mount (App.tsx). A narrow or short viewport
    // would silently break sidebar/terminal assertions in unrelated specs.
    viewport: { width: 1440, height: 900 },
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
    },
  ],

  webServer: {
    command: 'pnpm vite --config e2e/vite.config.ts --port 9246 --strictPort',
    cwd: path.resolve(__dirname, '..'),
    url: 'http://localhost:9246',
    reuseExistingServer: !process.env.CI,
    timeout: 30000,
  },
});
