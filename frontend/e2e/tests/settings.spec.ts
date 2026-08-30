import { test, expect, type Page } from '../fixtures';
import { sel } from '../utils/selectors';

const VALID_THEME = {
  name: 'My Custom Theme',
  type: 'dark',
  colors: { background: '#111111', foreground: '#eeeeee', primary: '#ff8800' },
};

const SECOND_THEME = {
  name: 'Second Custom Theme',
  type: 'dark',
  colors: { background: '#222222', foreground: '#dddddd', primary: '#00ff88' },
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

async function goToAppearanceTab(page: Page) {
  await page.getByRole('tab', { name: 'Appearance' }).click();
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

  test('Danger Zone: reset clears this window\'s own custom-theme state, so a later import cannot resurrect the deleted theme', async ({ page, gotoSettings }) => {
    await gotoSettings();
    // Import a custom theme so this window's customThemes state/ref is non-empty.
    await page.locator('input[type="file"]').setInputFiles(fileFrom(VALID_THEME));
    await expect(page.getByRole('button', { name: 'My Custom Theme theme, dark' })).toHaveAttribute('aria-pressed', 'true');

    await goToGeneralTab(page);
    await page.locator(sel.dangerZoneResetButton).click();
    await page.locator(sel.dangerZoneConfirm).click();
    await expect
      .poll(() => page.evaluate(() => window.__cmdexE2E!.callLog.some((c) => c.method === 'ResetAllData')))
      .toBe(true);

    // Importing a second theme is the realistic post-reset action that
    // round-trips this window's customThemes ref (onImportTheme ->
    // syncCustomThemes([...stale, new]) -> persistSettings). If the ref was
    // never cleared by the reset, the deleted theme comes back alongside
    // the new one.
    await goToAppearanceTab(page);
    await page.locator('input[type="file"]').setInputFiles(fileFrom(SECOND_THEME));
    await expect(page.getByRole('button', { name: 'Second Custom Theme theme, dark' })).toHaveAttribute('aria-pressed', 'true');

    await expect(page.getByRole('button', { name: 'My Custom Theme theme, dark' })).toHaveCount(0);

    const lastPayload = await page.evaluate(() => {
      const calls = window.__cmdexE2E!.callLog.filter((c) => c.method === 'SetSettings');
      return JSON.parse(calls[calls.length - 1].args[0] as string);
    });
    const persistedNames = JSON.parse(lastPayload.customThemes).map((t: { name: string }) => t.name);
    expect(persistedNames).toEqual(['Second Custom Theme']);
  });

  test('Danger Zone: reset also clears the local theme id, so a same-session theme import cannot persist the deleted theme id', async ({ page, gotoSettings }) => {
    await gotoSettings();
    await page.locator('input[type="file"]').setInputFiles(fileFrom(VALID_THEME));
    await expect(page.getByRole('button', { name: 'My Custom Theme theme, dark' })).toHaveAttribute('aria-pressed', 'true');

    const deletedThemeId = await page.evaluate(() => {
      const calls = window.__cmdexE2E!.callLog.filter((c) => c.method === 'SetSettings');
      const payloads = calls.map((c) => JSON.parse(c.args[0] as string) as { theme?: string });
      return payloads.find((p) => p.theme?.startsWith('custom-'))?.theme;
    });
    expect(deletedThemeId).toMatch(/^custom-/);

    await goToGeneralTab(page);
    await page.locator(sel.dangerZoneResetButton).click();
    await page.locator(sel.dangerZoneConfirm).click();
    await expect
      .poll(() => page.evaluate(() => window.__cmdexE2E!.callLog.some((c) => c.method === 'ResetAllData')))
      .toBe(true);

    // Importing a second theme without reopening settings is the exact
    // scenario from the report: handleImportTheme and the onThemeChange
    // call right after it fire two unordered persistSettings() writes. If
    // the reset left `theme` pointing at the deleted custom theme id,
    // whichever of those two writes carries that stale closure value would
    // persist a theme id absent from customThemes, regardless of which one
    // lands last.
    const beforeCount = await page.evaluate(
      () => window.__cmdexE2E!.callLog.filter((c) => c.method === 'SetSettings').length,
    );
    await goToAppearanceTab(page);
    await page.locator('input[type="file"]').setInputFiles(fileFrom(SECOND_THEME));
    await expect(page.getByRole('button', { name: 'Second Custom Theme theme, dark' })).toHaveAttribute('aria-pressed', 'true');
    await expect
      .poll(() => page.evaluate(() => window.__cmdexE2E!.callLog.filter((c) => c.method === 'SetSettings').length))
      .toBeGreaterThan(beforeCount);

    const postResetThemeIds = await page.evaluate((from) => {
      const calls = window.__cmdexE2E!.callLog.filter((c) => c.method === 'SetSettings');
      return calls.slice(from).map((c) => (JSON.parse(c.args[0] as string) as { theme: string }).theme);
    }, beforeCount);
    expect(postResetThemeIds).not.toContain(deletedThemeId);
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
