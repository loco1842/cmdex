import { test, expect, type Page } from '../fixtures';
import { sel } from '../utils/selectors';

const VALID_THEME = {
  name: 'My Custom Theme',
  type: 'dark',
  colors: { background: '#111111', foreground: '#eeeeee', primary: '#ff8800' },
};

function fileFrom(obj: unknown) {
  return { name: 'theme.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(obj)) };
}

async function goToTypographyTab(page: Page) {
  await page.getByRole('tab', { name: 'Typography' }).click();
}

async function goToGeneralTab(page: Page) {
  await page.getByRole('tab', { name: 'General' }).click();
}

test.describe('Settings — Appearance', () => {
  test('selecting a built-in theme swatch applies it and marks it pressed', async ({ page, gotoSettings }) => {
    await gotoSettings();
    const swatch = page.getByRole('button', { name: 'Monokai theme, dark' });
    await swatch.click();
    await expect(swatch).toHaveAttribute('aria-pressed', 'true');
  });

  test('importing a valid theme file applies it as a new, selected custom theme', async ({ page, gotoSettings }) => {
    await gotoSettings();
    await page.locator('input[type="file"]').setInputFiles(fileFrom(VALID_THEME));

    await expect(page.getByRole('button', { name: 'My Custom Theme theme, dark' })).toHaveAttribute('aria-pressed', 'true');
  });

  test('changing density updates the persisted setting', async ({ page, gotoSettings }) => {
    await gotoSettings();
    await page.getByRole('radio', { name: 'Compact' }).click();

    await expect
      .poll(() =>
        page.evaluate(() =>
          window.__cmdexE2E!.callLog.some(
            (c) => c.method === 'SetSettings' && (c.args[0] as string).includes('"density":"compact"'),
          ),
        ),
      )
      .toBe(true);
  });

  // Neither `main.tsx` (the settings window) nor its SettingsPage tree ever
  // mounts a `<Toaster>` — only App.tsx (the main window) does. Every
  // `toast.success`/`toast.error` call inside SettingsPage.tsx (theme
  // applied, invalid JSON, missing fields, pick-directory failure, "failed
  // to save settings", ...) is therefore a no-op the user never sees. This
  // documents the intended behavior for whichever of those toasts gets a
  // host; it must not be "fixed" by asserting silence is fine.
  test.fixme(
    'a failed theme import shows a visible error toast in the settings window',
    async ({ page, gotoSettings, toast }) => {
      await gotoSettings();
      await page.locator('input[type="file"]').setInputFiles({
        name: 'theme.json',
        mimeType: 'application/json',
        buffer: Buffer.from('{ not valid json'),
      });
      await expect(toast(/isn't valid theme JSON/i)).toBeVisible();
    },
  );
});

test.describe('Settings — Typography', () => {
  test('selecting a UI font persists it', async ({ page, gotoSettings }) => {
    await gotoSettings();
    await goToTypographyTab(page);
    await page.getByRole('group', { name: 'UI font selection' }).getByText('Geist', { exact: true }).click();

    await expect
      .poll(() =>
        page.evaluate(() =>
          window.__cmdexE2E!.callLog.some(
            (c) => c.method === 'SetSettings' && (c.args[0] as string).includes('"uiFont":"Geist"'),
          ),
        ),
      )
      .toBe(true);
  });

  test('selecting an editor (mono) font persists it', async ({ page, gotoSettings }) => {
    await gotoSettings();
    await goToTypographyTab(page);
    await page.getByRole('group', { name: 'Editor font selection' }).getByText('Fira Code', { exact: true }).click();

    await expect
      .poll(() =>
        page.evaluate(() =>
          window.__cmdexE2E!.callLog.some(
            (c) => c.method === 'SetSettings' && (c.args[0] as string).includes('"monoFont":"Fira Code"'),
          ),
        ),
      )
      .toBe(true);
  });
});

test.describe('Settings — General', () => {
  test('editing the working directory persists only on blur, not on every keystroke', async ({ page, gotoSettings }) => {
    await gotoSettings();
    await goToGeneralTab(page);
    const input = page.locator('input[type="text"]').first();
    await input.fill('/some/path');

    expect(await page.evaluate(() => window.__cmdexE2E!.callLog.filter((c) => c.method === 'SetSettings').length)).toBe(0);

    await input.blur();
    await expect
      .poll(() => page.evaluate(() => window.__cmdexE2E!.callLog.filter((c) => c.method === 'SetSettings').length))
      .toBeGreaterThan(0);
  });

  test('toggling shell integration persists immediately', async ({ page, gotoSettings }) => {
    await gotoSettings();
    await goToGeneralTab(page);
    await page.locator('#shell-integration-toggle').click();

    await expect
      .poll(() =>
        page.evaluate(() =>
          window.__cmdexE2E!.callLog.some(
            (c) => c.method === 'SetSettings' && (c.args[0] as string).includes('"shellIntegration"'),
          ),
        ),
      )
      .toBe(true);
  });

  test('Danger Zone: the two-step confirm resets all data via ResetAllData', async ({ page, seed, gotoSettings }) => {
    await seed({ commands: [{
      id: 'cmd-to-wipe',
      title: { String: 'Will Be Wiped', Valid: true },
      description: { String: '', Valid: false },
      scriptContent: 'echo x',
      tags: [], variables: [], presets: [], workingDir: {}, categoryId: '',
      position: 0, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(),
    }] });
    await gotoSettings();
    await goToGeneralTab(page);

    await page.locator(sel.dangerZoneResetButton).click();
    await expect(page.locator(sel.dangerZoneConfirm)).toBeVisible();

    await page.locator(sel.dangerZoneConfirm).click();

    await expect
      .poll(() => page.evaluate(() => window.__cmdexE2E!.callLog.some((c) => c.method === 'ResetAllData')))
      .toBe(true);
    expect(await page.evaluate(() => window.__cmdexE2E!.callLog.filter((c) => c.method === 'ResetAllData').length)).toBe(1);
  });

  test('Danger Zone: Cancel backs out without calling ResetAllData', async ({ page, gotoSettings }) => {
    await gotoSettings();
    await goToGeneralTab(page);
    await page.locator(sel.dangerZoneResetButton).click();
    await page.locator(sel.dangerZoneCancel).click();

    await expect(page.locator(sel.dangerZoneResetButton)).toBeVisible();
    expect(await page.evaluate(() => window.__cmdexE2E!.callLog.some((c) => c.method === 'ResetAllData'))).toBe(false);
  });
});
