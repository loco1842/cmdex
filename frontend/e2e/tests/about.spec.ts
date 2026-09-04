import { test, expect, type Page } from '../fixtures';

async function openAbout(page: Page, gotoApp: () => Promise<void>) {
  await gotoApp();
  await expect
    .poll(() => page.evaluate(() => window.__cmdexE2E!.hasListener('open-about')))
    .toBe(true);
  await page.evaluate(() => window.__cmdexE2E!.emit('open-about', null));
  await expect(page.getByTestId('about-dialog')).toBeVisible();
}

test.describe('About dialog', () => {
  test('dev builds show the dev state with a disabled check button', async ({ page, gotoApp }) => {
    await openAbout(page, gotoApp);

    await expect(page.getByText('Development build — updates are unavailable')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Check for Updates' })).toBeDisabled();
    await expect(page.locator('#beta-channel-toggle')).toHaveCount(0);
  });

  test('a configured build shows the version and drives a full update flow', async ({ page, seed, gotoApp }) => {
    await seed({ updateInfo: { version: '0.4.0', enabled: true } });
    await openAbout(page, gotoApp);

    await expect(page.getByText('Version 0.4.0 (arm64)')).toBeVisible();
    const checkButton = page.getByRole('button', { name: 'Check for Updates' });
    await expect(checkButton).toBeEnabled();
    await checkButton.click();
    await expect
      .poll(() => page.evaluate(() => window.__cmdexE2E!.callLog.some((c) => c.method === 'CheckForUpdates')))
      .toBe(true);

    await page.evaluate(() => window.__cmdexE2E!.emit('wails:updater:check-started', null));
    await expect(page.getByTestId('about-status')).toContainText('Checking…');

    await page.evaluate(() => window.__cmdexE2E!.emit('wails:updater:update-available', { version: '0.5.0' }));
    await expect(page.getByTestId('about-status')).toContainText('Downloading update…');

    await page.evaluate(() =>
      window.__cmdexE2E!.emit('wails:updater:download-progress', { written: 50, total: 100 }),
    );
    await expect(page.getByTestId('about-status')).toContainText('50%');

    await page.evaluate(() => window.__cmdexE2E!.emit('wails:updater:update-ready', { version: '0.5.0' }));
    await expect(page.getByTestId('about-status')).toContainText('Version 0.5.0 is ready');

    const restartButton = page.getByRole('button', { name: 'Restart to Update' });
    await restartButton.click();
    await expect
      .poll(() => page.evaluate(() => window.__cmdexE2E!.callLog.some((c) => c.method === 'RestartToUpdate')))
      .toBe(true);
  });

  test('a check with no update shows the up-to-date state', async ({ page, seed, gotoApp }) => {
    await seed({ updateInfo: { version: '0.4.0', enabled: true } });
    await openAbout(page, gotoApp);

    await page.getByRole('button', { name: 'Check for Updates' }).click();
    await page.evaluate(() => window.__cmdexE2E!.emit('wails:updater:no-update', null));
    await expect(page.getByTestId('about-status')).toContainText('You have the latest version');
  });

  test('an update error surfaces in the status line', async ({ page, seed, gotoApp }) => {
    await seed({ updateInfo: { version: '0.4.0', enabled: true } });
    await openAbout(page, gotoApp);

    await page.getByRole('button', { name: 'Check for Updates' }).click();
    await page.evaluate(() =>
      window.__cmdexE2E!.emit('wails:updater:error', { stage: 'check', message: 'boom' }),
    );
    await expect(page.getByTestId('about-status')).toContainText("Couldn't check for updates: boom");
  });

  test('the beta toggle persists via SetBetaChannel', async ({ page, seed, gotoApp }) => {
    await seed({ updateInfo: { version: '0.4.0', enabled: true } });
    await openAbout(page, gotoApp);

    await page.locator('#beta-channel-toggle').click();
    await expect
      .poll(() =>
        page.evaluate(() =>
          window.__cmdexE2E!.callLog.some(
            (c) => c.method === 'SetBetaChannel' && (c.args[0] as boolean) === true,
          ),
        ),
      )
      .toBe(true);
  });

  test('a previous check renders as last-checked', async ({ page, seed, gotoApp }) => {
    const twoHoursAgo = new Date(Date.now() - 2 * 3600 * 1000).toISOString();
    await seed({ updateInfo: { version: '0.4.0', enabled: true, lastCheck: twoHoursAgo } });
    await openAbout(page, gotoApp);

    await expect(page.getByTestId('about-status')).toContainText('Last checked 2 hours ago.');
  });

  test('the release-notes link opens the current version tag', async ({ page, seed, gotoApp }) => {
    await seed({ updateInfo: { version: '0.4.0', enabled: true } });
    await openAbout(page, gotoApp);

    await page.getByRole('button', { name: 'release notes' }).click();
    await expect
      .poll(() => page.evaluate(() => window.__cmdexE2E!.openedUrls))
      .toEqual(['https://github.com/loco1842/cmdex/releases/tag/v0.4.0']);
  });
});
