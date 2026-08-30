import { test, expect, type Page } from '../fixtures';
import { sel } from '../utils/selectors';
import type { MethodName } from '../mocks/runtime';

function seededCommand(id: string, title: string, scriptContent = `echo ${id}`) {
  const now = new Date().toISOString();
  return {
    id,
    title: { String: title, Valid: true },
    description: { String: '', Valid: false },
    scriptContent,
    tags: [],
    variables: [],
    presets: [],
    workingDir: {},
    categoryId: '',
    position: 0,
    createdAt: now,
    updatedAt: now,
  };
}

async function failNext(page: Page, method: MethodName, message = 'boom') {
  await page.evaluate(({ method, message }) => window.__cmdexE2E!.failNext(method, message), { method, message });
}

// Each test injects a one-shot RPC failure via failNext and asserts a real
// sentence renders — not a raw i18n key, and not silence (the pre-track
// behavior for all of these was console.error-only, so the user believed
// the action succeeded).
test.describe('Error toasts', () => {
  test('saving an existing command shows commandSaveFailed on UpdateCommand failure', async ({ page, seed, gotoApp, toast }) => {
    await seed({ commands: [seededCommand('cmd-e1', 'Save Fail')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Save Fail' }).click();
    await page.locator(sel.commandTitle).fill('Edited');
    await failNext(page, 'UpdateCommand');

    await page.locator(sel.saveBarSave).click();

    await expect(toast(/couldn't save the command/i)).toBeVisible();
  });

  test('creating a category shows categoryCreateFailed on CreateCategory failure', async ({ page, gotoApp, toast }) => {
    await gotoApp();
    await failNext(page, 'CreateCategory');

    await page.locator('.sidebar-content').click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'New Category' }).click();
    await page.locator(sel.categoryNameInput).fill('Will Fail');
    await page.locator(sel.categoryEditor).locator('button').filter({ hasText: 'Create' }).click();

    await expect(toast(/couldn't create the category/i)).toBeVisible();
  });

  test('editing a category shows categoryUpdateFailed on UpdateCategory failure', async ({ page, seed, gotoApp, toast }) => {
    await seed({ categories: [{ id: 'cat-e1', name: 'Original', icon: '', color: '#000', createdAt: '', updatedAt: '' }] });
    await gotoApp();
    await failNext(page, 'UpdateCategory');

    await page.locator('.sidebar-section-header[data-category-id="cat-e1"]').click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Edit category' }).click();
    await page.locator(sel.categoryNameInput).fill('Renamed');
    await page.locator(sel.categoryEditor).locator('button').filter({ hasText: 'Save' }).click();

    await expect(toast(/couldn't save the category/i)).toBeVisible();
  });

  test('deleting a category shows categoryDeleteFailed on DeleteCategory failure', async ({ page, seed, gotoApp, toast }) => {
    await seed({ categories: [{ id: 'cat-e2', name: 'To Delete', icon: '', color: '#000', createdAt: '', updatedAt: '' }] });
    await gotoApp();
    await failNext(page, 'DeleteCategory');

    await page.locator('.sidebar-section-header[data-category-id="cat-e2"]').click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Delete' }).click();
    await page.locator(sel.confirmDeleteCategoryConfirm).click();

    await expect(toast(/couldn't delete the category/i)).toBeVisible();
  });

  test('deleting a command shows commandDeleteFailed on DeleteCommand failure', async ({ page, seed, gotoApp, toast }) => {
    await seed({ commands: [seededCommand('cmd-e2', 'Delete Fail')] });
    await gotoApp();
    await failNext(page, 'DeleteCommand');

    const item = page.locator(sel.commandItem('cmd-e2'));
    await item.hover();
    await item.locator('.cmd-trash-btn').click();
    await item.locator('.cmd-delete-icon-btn').click();

    await expect(toast(/couldn't delete the command/i)).toBeVisible();
  });

  test('saving a script directly shows scriptSaveFailed on UpdateCommand failure', async ({ page, seed, gotoApp, toast }) => {
    await seed({ commands: [seededCommand('cmd-e3', 'Script Save Fail')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Script Save Fail' }).click();
    await failNext(page, 'UpdateCommand');

    await page.locator(sel.scriptEditEnterBtn).click();
    await page.locator(sel.commandScriptTextarea).fill('echo changed');
    await page.keyboard.press('Enter');

    await expect(toast(/couldn't save the script/i)).toBeVisible();
  });

  test('adding a preset shows presetAddFailed on SavePreset failure', async ({ page, seed, gotoApp, toast }) => {
    await seed({
      commands: [{ ...seededCommand('cmd-e4', 'Preset Add Fail'), variables: [{ name: 'x', description: '', example: '', default: '', sortOrder: 0 }] }],
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Add Fail' }).click();
    await failNext(page, 'SavePreset');

    await page.locator(sel.presetChipAdd).click();

    await expect(toast(/couldn't add preset/i)).toBeVisible();
  });

  test('renaming a preset shows presetRenameFailed on UpdatePreset failure', async ({ page, seed, gotoApp, toast }) => {
    await seed({
      commands: [{ ...seededCommand('cmd-e5', 'Preset Rename Fail'), variables: [{ name: 'x', description: '', example: '', default: '', sortOrder: 0 }] }],
      presets: { 'cmd-e5': [{ id: 'preset-e5', name: 'Original', position: 0, values: {} }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Rename Fail' }).click();
    await failNext(page, 'UpdatePreset');

    await page.locator(sel.presetChip('preset-e5')).dblclick();
    const renameInput = page.locator(sel.presetChipRename('preset-e5'));
    await renameInput.fill('New Name');
    await renameInput.press('Enter');

    await expect(toast(/couldn't rename preset/i)).toBeVisible();
  });

  test('deleting a preset shows presetDeleteFailed on DeletePreset failure', async ({ page, seed, gotoApp, toast }) => {
    await seed({
      commands: [{ ...seededCommand('cmd-e6', 'Preset Delete Fail'), variables: [{ name: 'x', description: '', example: '', default: '', sortOrder: 0 }] }],
      presets: { 'cmd-e6': [{ id: 'preset-e6', name: 'Delete Me', position: 0, values: {} }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Delete Fail' }).click();
    await failNext(page, 'DeletePreset');

    await page.locator(sel.presetChip('preset-e6')).click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Delete' }).click();
    await page.locator(sel.confirmDeletePresetConfirm).click();

    await expect(toast(/couldn't delete preset/i)).toBeVisible();
  });

  test('saving preset values shows presetSaveFailed on UpdatePreset failure', async ({ page, seed, gotoApp, toast }) => {
    await seed({
      commands: [{ ...seededCommand('cmd-e7', 'Preset Save Fail'), variables: [{ name: 'x', description: '', example: '', default: '', sortOrder: 0 }] }],
      presets: { 'cmd-e7': [{ id: 'preset-e7', name: 'Values', position: 0, values: { x: 'a' } }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Save Fail' }).click();
    await failNext(page, 'UpdatePreset');

    const input = page.locator(sel.presetVarInput('x'));
    await input.fill('b');
    await input.press('Enter');

    await expect(toast(/couldn't save preset/i)).toBeVisible();
  });

  test('copying terminal output shows outputCopyFailed when both the Clipboard API and execCommand fallback fail', async ({ page, context, gotoApp, toast }) => {
    await context.grantPermissions([]);
    await page.addInitScript(() => {
      // Force both copyText() paths (clipboard.ts) to fail: deny the
      // Clipboard API by making writeText reject, and make the
      // document.execCommand('copy') fallback report failure too.
      Object.defineProperty(navigator, 'clipboard', {
        value: { writeText: () => Promise.reject(new Error('denied')) },
        configurable: true,
      });
      document.execCommand = () => false;
    });
    await gotoApp();
    await expect(page.getByRole('textbox', { name: 'Terminal input' })).toBeAttached();

    const sessionId = await page.locator('.tab-item').first().getAttribute('data-testid').then((v) => v!.replace('terminal-tab-', ''));
    await page.evaluate((id) => window.__cmdexE2E!.emitPtyOutput(id, '$ \r\noutput line 1\r\n$ '), sessionId);

    await page.locator('.terminal-copy-btn').click();

    await expect(toast(/failed to copy/i)).toBeVisible();
  });
});
