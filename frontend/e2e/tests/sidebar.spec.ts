import { test, expect } from '../fixtures';
import { sel } from '../utils/selectors';

function seededCategory(id: string, name: string) {
  const now = new Date().toISOString();
  return { id, name, icon: '', color: '#7c6aef', createdAt: now, updatedAt: now };
}

function seededCommand(id: string, title: string, categoryId = '') {
  const now = new Date().toISOString();
  return {
    id,
    title: { String: title, Valid: true },
    description: { String: '', Valid: false },
    scriptContent: `echo ${id}`,
    tags: [],
    variables: [],
    presets: [],
    workingDir: {},
    categoryId,
    position: 0,
    createdAt: now,
    updatedAt: now,
  };
}

test.describe('Sidebar', () => {
  test('a category collapses and expands, persisting to localStorage', async ({ page, seed, gotoApp }) => {
    await seed({ categories: [seededCategory('cat-1', 'My Category')] });
    await gotoApp();

    const header = page.locator('.sidebar-section-header[data-category-id="cat-1"]');
    await expect(header).toBeVisible();
    // Freshly seeded/auto-expanded categories are NOT written to localStorage
    // until the user actually toggles something — persistExpanded() is only
    // called from toggleCategory, not from the auto-expand-on-new effect —
    // so there is nothing meaningful to assert about storage before the
    // first click below.
    await header.click();
    let stored = await page.evaluate((k) => JSON.parse(localStorage.getItem(k) || '[]'), 'cmdex-expanded-categories');
    expect(stored).not.toContain('cat-1');

    await header.click();
    stored = await page.evaluate((k) => JSON.parse(localStorage.getItem(k) || '[]'), 'cmdex-expanded-categories');
    expect(stored).toContain('cat-1');
  });

  test('the Uncategorized bucket is always rendered, even with zero categories and commands', async ({ page, gotoApp }) => {
    await gotoApp();
    await expect(page.getByText('Uncategorized')).toBeVisible();
  });

  test('right-clicking a category header opens the category-scoped menu (edit/export/import/delete)', async ({ page, seed, gotoApp }) => {
    await seed({ categories: [seededCategory('cat-2', 'Scoped Menu Cat')] });
    await gotoApp();

    await page.locator('.sidebar-section-header[data-category-id="cat-2"]').click({ button: 'right' });
    const menu = page.locator('[role="menu"]');
    await expect(menu.getByRole('menuitem', { name: 'Edit category' })).toBeVisible();
    await expect(menu.getByRole('menuitem', { name: 'Delete' })).toBeVisible();
  });

  test('right-clicking empty sidebar space opens a menu with no per-category items', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.locator('.sidebar-content').click({ button: 'right' });
    const menu = page.locator('[role="menu"]');
    await expect(menu.getByRole('menuitem', { name: 'New Command' })).toBeVisible();
    await expect(menu.getByRole('menuitem', { name: 'New Category' })).toBeVisible();
    // No "Edit category" or per-category "Delete" item exists on this menu.
    await expect(menu.getByRole('menuitem', { name: 'Edit category' })).toHaveCount(0);
  });

  test('deleting a category moves its commands to Uncategorized rather than deleting them', async ({ page, seed, gotoApp }) => {
    await seed({
      categories: [seededCategory('cat-3', 'To Delete')],
      commands: [seededCommand('cmd-in-cat', 'Command In Category', 'cat-3')],
    });
    await gotoApp();

    await page.locator('.sidebar-section-header[data-category-id="cat-3"]').click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Delete' }).click();
    await page.locator(sel.confirmDeleteCategoryConfirm).click();

    await expect(page.getByText('To Delete')).not.toBeVisible();
    // The command survives, now listed under Uncategorized.
    await expect(page.locator('.cmd-title').filter({ hasText: 'Command In Category' })).toBeVisible();
  });

  test('exporting a category succeeds silently (no error toast)', async ({ page, seed, gotoApp, toast }) => {
    await seed({
      categories: [seededCategory('cat-4', 'Export Cat')],
      commands: [seededCommand('cmd-export', 'Exportable', 'cat-4')],
    });
    await gotoApp();

    await page.locator('.sidebar-section-header[data-category-id="cat-4"]').click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Export commands' }).click();

    await expect(toast(/couldn't export/i)).toHaveCount(0);
  });

  test('a failed export shows the export-failed toast', async ({ page, seed, gotoApp, toast }) => {
    await seed({
      categories: [seededCategory('cat-5', 'Export Fail Cat')],
      commands: [seededCommand('cmd-export-fail', 'Will Fail', 'cat-5')],
    });
    await gotoApp();
    await page.evaluate(() => window.__cmdexE2E!.failNext('ExportCommands', 'disk full'));

    await page.locator('.sidebar-section-header[data-category-id="cat-5"]').click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Export commands' }).click();

    await expect(toast(/couldn't export/i)).toBeVisible();
  });

  test('cancelling the import dialog (the default mock result) shows no error and imports nothing', async ({ page, gotoApp, toast }) => {
    await gotoApp();
    await page.locator('.sidebar-content').click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Import commands' }).click();

    await expect(toast(/couldn't import/i)).toHaveCount(0);
    await expect(page.locator('.command-item')).toHaveCount(0);
  });

  test('a successful import refreshes the sidebar with the imported commands', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.evaluate(
      (cmd) => window.__cmdexE2E!.setImportResult([cmd]),
      seededCommand('cmd-imported', 'Freshly Imported'),
    );

    await page.locator('.sidebar-content').click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Import commands' }).click();

    await expect(page.locator('.cmd-title').filter({ hasText: 'Freshly Imported' })).toBeVisible();
  });

  test('a failed import shows the import-failed toast', async ({ page, gotoApp, toast }) => {
    await gotoApp();
    await page.evaluate(() => window.__cmdexE2E!.failNext('ImportCommands', 'bad file'));

    await page.locator('.sidebar-content').click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Import commands' }).click();

    await expect(toast(/couldn't import/i)).toBeVisible();
  });

  test.fixme(
    'deleting a category does not leave a stale categoryId in an open, dirty tab for one of its commands',
    async ({ page, seed, gotoApp }) => {
      // App.tsx's handleDeleteCategory only clears selectedCommand if it
      // belonged to the deleted category — it does not touch openTabs/
      // tabDrafts for that command, so a still-open tab keeps referencing
      // the now-deleted categoryId in its draft, which can show as a
      // phantom-dirty tab. This documents the intended behavior; it must
      // not be "fixed" by asserting the stale categoryId is fine.
      await seed({
        categories: [seededCategory('cat-stale', 'Stale Cat')],
        commands: [seededCommand('cmd-stale', 'Stale Tab Cmd', 'cat-stale')],
      });
      await gotoApp();

      await page.locator('.cmd-title').filter({ hasText: 'Stale Tab Cmd' }).click();
      await expect(page.locator(sel.tabItem('cmd-stale'))).toBeVisible();

      await page.locator('.sidebar-section-header[data-category-id="cat-stale"]').click({ button: 'right' });
      await page.locator('[role="menuitem"]').filter({ hasText: 'Delete' }).click();
      await page.locator(sel.confirmDeleteCategoryConfirm).click();

      await expect(page.locator(sel.tabDirtyDot('cmd-stale'))).toHaveCount(0);
    },
  );
});
