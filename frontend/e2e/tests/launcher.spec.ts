import { test, expect, type Page } from '../fixtures';

function command(id: string, title: string, variables: unknown[] = []) {
  const now = new Date().toISOString();
  return {
    id,
    title: { String: title, Valid: true },
    description: { String: '', Valid: false },
    scriptContent: 'echo hello',
    tags: [],
    variables,
    presets: [],
    workingDir: {},
    categoryId: '',
    position: 0,
    createdAt: now,
    updatedAt: now,
  };
}

async function openLauncher(page: Page) {
  await page.goto('/?window=launcher');
  await expect(page.locator('.launcher-root')).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => window.__cmdexE2E!.hasListener('launcher-shown')))
    .toBe(true);
}

test.describe('Launcher', () => {
  test('the mock preserves visibility and emits the production launcher event contract', async ({ page }) => {
    await openLauncher(page);

    await page.evaluate(() => window.__cmdexE2E!.invokeLauncher('Show'));
    await page.evaluate(() => window.__cmdexE2E!.invokeLauncher('Toggle'));
    await page.evaluate(() => window.__cmdexE2E!.invokeLauncher('Toggle'));

    const state = await page.evaluate(() => ({
      visible: window.__cmdexE2E!.launcherVisible,
      events: window.__cmdexE2E!.launcherEventLog.map((event) => ({
        name: event.name,
        hasPayload: event.data !== undefined,
      })),
    }));
    expect(state.visible).toBe(true);
    expect(state.events).toEqual([
      { name: 'launcher-shown', hasPayload: false },
      { name: 'launcher-hidden', hasPayload: false },
      { name: 'launcher-shown', hasPayload: false },
    ]);
  });

  test('showing the persistent launcher resets an in-progress variable prompt', async ({ page, seed }) => {
    await seed({
      commands: [command('launcher-vars', 'Needs variables', [{ name: 'name', description: '', default: '' }])],
    });
    await openLauncher(page);

    await page.getByRole('option', { name: /Needs variables/ }).click();
    await expect(page.locator('[data-testid="fill-variables-dialog"]')).toBeVisible();

    await page.evaluate(() => window.__cmdexE2E!.invokeLauncher('Show'));

    await expect(page.locator('[data-testid="fill-variables-dialog"]')).toHaveCount(0);
    await expect(page.locator('.launcher-results')).toBeVisible();
    await expect(page.getByRole('option', { name: /Needs variables/ })).toBeVisible();
  });

  test('showing the persistent launcher resets the running output stage', async ({ page, seed }) => {
    await seed({ commands: [command('launcher-run', 'Run me')] });
    await openLauncher(page);

    await page.getByRole('option', { name: /Run me/ }).click();
    await expect(page.locator('.launcher-run-panel')).toBeVisible();

    await page.evaluate(() => window.__cmdexE2E!.invokeLauncher('Show'));

    await expect(page.locator('.launcher-run-panel')).toHaveCount(0);
    await expect(page.locator('.launcher-results')).toBeVisible();
    await expect(page.getByRole('option', { name: /Run me/ })).toBeVisible();
  });

  test('typing after command output immediately returns to search and ignores stale PTY output', async ({ page, seed }) => {
    await seed({
      commands: [command('launcher-run', 'Run me'), command('launcher-search', 'Search me')],
    });
    await openLauncher(page);

    await page.getByRole('option', { name: /Run me/ }).click();
    await expect(page.locator('.launcher-run-panel')).toBeVisible();
    const sessionId = await page.evaluate(() => window.__cmdexE2E!.launcherSessionId);

    await page.getByRole('combobox', { name: /Search CmDex commands/ }).fill('Search me');
    await expect(page.locator('.launcher-run-panel')).toHaveCount(0);
    await expect(page.getByRole('option', { name: /Search me/ })).toBeVisible();

    await page.evaluate((id) => window.__cmdexE2E!.emitPtyOutput(id, 'stale output after search'), sessionId);
    await expect(page.locator('.launcher-results')).toBeVisible();
    await expect(page.getByRole('option', { name: /Search me/ })).toBeVisible();
  });

  test('typing after an in-band execution failure remains a normal search', async ({ page, seed }) => {
    await seed({
      commands: [command('launcher-fail', 'Fail me'), command('launcher-search', 'Search me')],
    });
    await openLauncher(page);
    await page.evaluate(() => window.__cmdexE2E!.setLauncherRunResult({ error: 'mock execution failed', exitCode: -1 }));

    await page.getByRole('option', { name: /Fail me/ }).click();
    await expect(page.locator('.launcher-run-panel')).toHaveCount(0);
    await page.getByRole('combobox', { name: /Search CmDex commands/ }).fill('Search me');
    await expect(page.getByRole('option', { name: /Search me/ })).toBeVisible();

    const sessionId = await page.evaluate(() => window.__cmdexE2E!.launcherSessionId);
    await page.evaluate((id) => window.__cmdexE2E!.emitPtyOutput(id, 'stale failed output'), sessionId);
    await expect(page.locator('.launcher-results')).toBeVisible();
    await expect(page.getByRole('option', { name: /Search me/ })).toBeVisible();
  });

  test('reuses one launcher session across repeated executions while preserving normal terminal actions', async ({ page, seed }) => {
    await seed({ commands: [command('launcher-run', 'Run me')] });
    await openLauncher(page);

    await page.getByRole('option', { name: /Run me/ }).click();
    await expect(page.locator('.launcher-run-panel')).toBeVisible();
    const firstID = await page.evaluate(() => window.__cmdexE2E!.launcherSessionId);

    await page.evaluate(() => window.__cmdexE2E!.invokeLauncher('Show'));
    await expect(page.locator('.launcher-results')).toBeVisible();
    await page.getByRole('option', { name: /Run me/ }).click();
    await expect(page.locator('.launcher-run-panel')).toBeVisible();
    const secondID = await page.evaluate(() => window.__cmdexE2E!.launcherSessionId);

    expect(secondID).toBe(firstID);
    const launcherSessionCalls = await page.evaluate(() =>
      window.__cmdexE2E!.callLog.filter((entry) => entry.method === 'GetSessionID').length,
    );
    expect(launcherSessionCalls).toBe(2);
  });
});
