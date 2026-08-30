// Shared Playwright fixtures for the Cmdex e2e suite. Every spec should
// import `test`/`expect` from here instead of directly from
// `@playwright/test`, so seeding, navigation, and toast assertions share one
// implementation instead of being hand-rolled per spec (as the original 5
// specs did, including the 5 waitForTimeout sleeps this replaces).
import { test as base, expect, type Locator } from '@playwright/test';
import type { CmdexE2ESeed } from './utils/types';
import './utils/types';

interface CmdexFixtures {
  /** Seed the mock backend's initial state before the app's first render.
   * Must be called before `gotoApp()` — it uses addInitScript under the hood. */
  seed(data: CmdexE2ESeed): Promise<void>;
  /** Navigate to the main window and wait for the sidebar to mount. */
  gotoApp(): Promise<void>;
  /** Navigate to the settings window (`?window=settings`) and wait for it
   * to mount. */
  gotoSettings(): Promise<void>;
  /** Locator for a visible toast containing `text` (Sonner renders every
   * live toast with `[data-sonner-toast]`; `Toaster`'s duration is 3000ms,
   * per App.tsx, so callers should assert promptly). */
  toast(text: string | RegExp): Locator;
  /** The currently-visible command tab's root element — all opened tabs
   * stay mounted (CommandDetailTab.tsx toggles `display`), so any
   * `command-*` testid must be scoped through this to avoid matching a
   * hidden tab. */
  visibleTabShell(): Locator;
  /** Press the app's own cmd-or-ctrl modifier + `key` (e.g. 's', 'enter').
   * `lib/shortcuts.ts`'s `isMac` is computed from `navigator.userAgent` at
   * module load, and Playwright's bundled Chromium reports a fixed
   * (non-host-reflecting) UA — NOT necessarily matching what Playwright's
   * own `ControlOrMeta` modifier decides from the real OS. Using
   * `ControlOrMeta` here would silently press the wrong modifier and the
   * app's shortcut would just never fire. This reads the in-page UA the
   * same way the app does and presses whichever modifier it actually
   * registered ('meta' or 'ctrl'). */
  pressCmdOrCtrl(key: string): Promise<void>;
}

export const test = base.extend<CmdexFixtures>({
  seed: async ({ page }, use) => {
    await use(async (data: CmdexE2ESeed) => {
      await page.addInitScript((seedData) => {
        window.__cmdexE2E_SEED__ = seedData;
      }, data);
    });
  },

  gotoApp: async ({ page }, use) => {
    await use(async () => {
      await page.goto('/');
      await expect(page.locator('.sidebar')).toBeVisible();
    });
  },

  gotoSettings: async ({ page }, use) => {
    await use(async () => {
      await page.goto('/?window=settings');
      await expect(page.getByRole('tablist')).toBeVisible();
    });
  },

  toast: async ({ page }, use) => {
    await use((text: string | RegExp) => page.locator('[data-sonner-toast]').filter({ hasText: text }));
  },

  visibleTabShell: async ({ page }, use) => {
    await use(() => page.locator('.command-tab-shell:visible'));
  },

  pressCmdOrCtrl: async ({ page }, use) => {
    await use(async (key: string) => {
      const isMac = await page.evaluate(() => /Mac|iPhone|iPad/.test(navigator.userAgent));
      await page.keyboard.press(`${isMac ? 'Meta' : 'Control'}+${key}`);
    });
  },
});

export { expect };
export type { Page } from '@playwright/test';
