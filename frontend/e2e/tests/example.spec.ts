import { test, expect } from '../fixtures';
import { sel } from '../utils/selectors';

test.describe('App smoke test', () => {
  test('loads and shows the welcome screen', async ({ page, gotoApp }) => {
    await gotoApp();
    await expect(page.locator('.sidebar-header h1')).toContainText('CmDex');
    await expect(page.locator('.sidebar')).toBeVisible();
  });

  test('can open a new command tab via sidebar + button', async ({ page, gotoApp }) => {
    await gotoApp();

    await page.locator(sel.sidebarAddCommand).click();

    await expect(page.locator(sel.tabBar)).toBeVisible();
  });
});
