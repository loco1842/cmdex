import { test, expect } from '../fixtures';
import type { Page } from '@playwright/test';

// A custom theme carries its colors in the settings payload; there is no
// `[data-theme="custom-..."]` CSS rule, so they only take effect when the app
// writes them as inline CSS variables on the root element.
const CUSTOM_THEME = {
  id: 'custom-test-theme',
  name: 'Test Custom',
  type: 'dark' as const,
  colors: {
    background: '#101014',
    foreground: '#e8e6e3',
    primary: '#d2691e',
    'status-bar-bg': '#1b1b21',
  },
};

function rootVar(page: Page, name: string): Promise<string> {
  return page.evaluate(
    (varName) => document.documentElement.style.getPropertyValue(varName),
    name,
  );
}

// The settings window emits `settings-changed` with a raw payload
// (main.tsx/SettingsPage.tsx call Events.Emit(name, settings) directly); the
// runtime mock now wraps every Events.Emit in a WailsEvent envelope itself
// (matching real Wails), so callers here pass the raw payload too. The app
// subscribes only once its async event-name lookup resolves, so wait for the
// listener — emitting earlier drops the event and the assertions below would
// then be waiting for something that can never arrive.
async function emitSettingsChanged(page: Page, data: Record<string, unknown>) {
  await expect
    .poll(() =>
      page.evaluate(() => window.__cmdexE2E?.hasListener('settings-changed') ?? false),
    )
    .toBe(true);
  await page.evaluate((payload) => {
    window.__cmdexE2E?.emit('settings-changed', payload);
  }, data);
}

test.describe('Custom themes', () => {
  test('applies the saved custom theme colors on load', async ({ page, seed, gotoApp }) => {
    await seed({
      settings: {
        theme: CUSTOM_THEME.id,
        customThemes: JSON.stringify([CUSTOM_THEME]),
      },
    });
    await gotoApp();

    await expect.poll(() => rootVar(page, '--background')).toBe('#101014');
    expect(await rootVar(page, '--primary')).toBe('#d2691e');
    expect(await rootVar(page, '--status-bar-bg')).toBe('#1b1b21');
    expect(await page.getAttribute('html', 'data-theme')).toBe(CUSTOM_THEME.id);
  });

  test('applies a custom theme delivered via settings-changed', async ({ page, gotoApp }) => {
    await gotoApp();
    expect(await rootVar(page, '--background')).toBe('');

    await emitSettingsChanged(page, {
      theme: CUSTOM_THEME.id,
      customThemes: JSON.stringify([CUSTOM_THEME]),
    });

    await expect.poll(() => rootVar(page, '--background')).toBe('#101014');
    expect(await rootVar(page, '--foreground')).toBe('#e8e6e3');
    expect(await page.getAttribute('html', 'data-theme')).toBe(CUSTOM_THEME.id);
  });

  test('clears custom colors when switching back to a built-in theme', async ({ page, seed, gotoApp }) => {
    await seed({
      settings: {
        theme: CUSTOM_THEME.id,
        customThemes: JSON.stringify([CUSTOM_THEME]),
      },
    });
    await gotoApp();
    await expect.poll(() => rootVar(page, '--background')).toBe('#101014');

    await emitSettingsChanged(page, { theme: 'vscode-light', customThemes: '[]' });

    await expect.poll(() => rootVar(page, '--background')).toBe('');
    expect(await rootVar(page, '--primary')).toBe('');
    expect(await page.getAttribute('html', 'data-theme')).toBe('vscode-light');
  });

  test('does not leak colors between two custom themes', async ({ page, seed, gotoApp }) => {
    await seed({
      settings: {
        theme: CUSTOM_THEME.id,
        customThemes: JSON.stringify([CUSTOM_THEME]),
      },
    });
    await gotoApp();
    await expect.poll(() => rootVar(page, '--primary')).toBe('#d2691e');

    // The second theme omits `primary` and `status-bar-bg`; those must fall back
    // to the stylesheet rather than keep the first theme's values.
    const partial = {
      id: 'custom-partial',
      name: 'Partial',
      type: 'dark' as const,
      colors: { background: '#202028', foreground: '#f0f0f0' },
    };
    await emitSettingsChanged(page, {
      theme: partial.id,
      customThemes: JSON.stringify([partial]),
    });

    await expect.poll(() => rootVar(page, '--background')).toBe('#202028');
    expect(await rootVar(page, '--primary')).toBe('');
    expect(await rootVar(page, '--status-bar-bg')).toBe('');
  });
});
