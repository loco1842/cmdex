import { test, expect } from '../fixtures';
import { sel } from '../utils/selectors';

// The `managePresets` modal (VariablePrompt mode="manage") is declared in
// App.tsx's ModalState union and rendered, but no `setModal({type:
// 'managePresets'})` call exists anywhere in the app — it is unreachable
// dead code. These tests cover only the reachable preset UI: the inline
// chips + variable-value panel inside CommandDetail.

function seededCommandWithVars(id: string, title: string) {
  const now = new Date().toISOString();
  return {
    id,
    title: { String: title, Valid: true },
    description: { String: '', Valid: false },
    scriptContent: 'echo "Hello {{name}} from {{city}}"',
    tags: [],
    variables: [
      { name: 'name', description: '', example: '', default: '', sortOrder: 0 },
      { name: 'city', description: '', example: '', default: '', sortOrder: 1 },
    ],
    presets: [],
    workingDir: {},
    categoryId: '',
    position: 0,
    createdAt: now,
    updatedAt: now,
  };
}

test.describe('Presets', () => {
  test('adding a preset creates and auto-selects a chip in rename mode', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommandWithVars('cmd-p1', 'Preset Cmd 1')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Cmd 1' }).click();

    await page.locator(sel.presetChipAdd).click();

    // A newly-created preset opens directly in its rename input.
    await expect(page.locator('.preset-chip-renaming')).toBeVisible();
    await page.locator('.preset-chip-renaming').fill('My Preset');
    await page.locator('.preset-chip-renaming').press('Enter');

    await expect(page.locator('.preset-chip').filter({ hasText: 'My Preset' })).toBeVisible();
    await expect(page.locator('.preset-chip.active').filter({ hasText: 'My Preset' })).toBeVisible();
  });

  test('clicking the active chip deselects it', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [seededCommandWithVars('cmd-p2', 'Preset Cmd 2')],
      presets: { 'cmd-p2': [{ id: 'preset-1', name: 'Only Preset', position: 0, values: { name: 'Ada', city: 'NYC' } }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Cmd 2' }).click();

    const chip = page.locator(sel.presetChip('preset-1'));
    await expect(chip).toHaveClass(/active/);

    await chip.click();
    await expect(chip).not.toHaveClass(/active/);
  });

  test('renaming a preset via double-click commits on Enter', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [seededCommandWithVars('cmd-p3', 'Preset Cmd 3')],
      presets: { 'cmd-p3': [{ id: 'preset-2', name: 'Old Name', position: 0, values: {} }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Cmd 3' }).click();

    await page.locator(sel.presetChip('preset-2')).dblclick();
    const renameInput = page.locator(sel.presetChipRename('preset-2'));
    await expect(renameInput).toBeVisible();
    await renameInput.fill('New Name');
    await renameInput.press('Enter');

    await expect(page.locator('.preset-chip').filter({ hasText: 'New Name' })).toBeVisible();
    await expect(page.locator('.preset-chip').filter({ hasText: 'Old Name' })).toHaveCount(0);
  });

  test('deleting a preset via context menu asks for confirmation', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [seededCommandWithVars('cmd-p4', 'Preset Cmd 4')],
      presets: { 'cmd-p4': [{ id: 'preset-3', name: 'Delete Me', position: 0, values: {} }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Cmd 4' }).click();

    await page.locator(sel.presetChip('preset-3')).click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Delete' }).click();

    await expect(page.locator(sel.confirmDeletePresetDialog)).toBeVisible();
    await page.locator(sel.confirmDeletePresetConfirm).click();

    await expect(page.locator('.preset-chip').filter({ hasText: 'Delete Me' })).toHaveCount(0);
  });

  test('cancelling the delete-preset dialog keeps the preset', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [seededCommandWithVars('cmd-p5', 'Preset Cmd 5')],
      presets: { 'cmd-p5': [{ id: 'preset-4', name: 'Keep Me', position: 0, values: {} }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Cmd 5' }).click();

    await page.locator(sel.presetChip('preset-4')).click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Delete' }).click();
    await page.locator(sel.confirmDeletePresetCancel).click();

    await expect(page.locator('.preset-chip').filter({ hasText: 'Keep Me' })).toBeVisible();
  });

  test('editing a selected preset\'s variable value, Enter saves it and clears the unsaved-override bar', async ({ page, seed, gotoApp, toast }) => {
    await seed({
      commands: [seededCommandWithVars('cmd-p6', 'Preset Cmd 6')],
      presets: { 'cmd-p6': [{ id: 'preset-5', name: 'Values Preset', position: 0, values: { name: 'Ada', city: 'NYC' } }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Cmd 6' }).click();

    const nameInput = page.locator(sel.presetVarInput('name'));
    await expect(nameInput).toHaveValue('Ada');
    await nameInput.fill('Grace');

    await expect(page.locator(sel.presetValuesSave)).toBeVisible();
    await nameInput.press('Enter');

    await expect(toast(/preset saved/i)).toBeVisible();
    await expect(page.locator(sel.presetValuesSave)).toHaveCount(0);
    await expect(nameInput).toHaveValue('Grace');
  });

  test('Escape on a variable input reverts only that field\'s unsaved override', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [seededCommandWithVars('cmd-p7', 'Preset Cmd 7')],
      presets: { 'cmd-p7': [{ id: 'preset-6', name: 'Revert Preset', position: 0, values: { name: 'Ada', city: 'NYC' } }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Cmd 7' }).click();

    const nameInput = page.locator(sel.presetVarInput('name'));
    const cityInput = page.locator(sel.presetVarInput('city'));
    await nameInput.fill('Unsaved Change');
    await cityInput.fill('Also Unsaved');

    await nameInput.press('Escape');

    await expect(nameInput).toHaveValue('Ada');
    await expect(cityInput).toHaveValue('Also Unsaved');
  });

  test('the revert-all button in the unsaved-override bar discards every pending edit', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [seededCommandWithVars('cmd-p8', 'Preset Cmd 8')],
      presets: { 'cmd-p8': [{ id: 'preset-7', name: 'Revert All Preset', position: 0, values: { name: 'Ada', city: 'NYC' } }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Preset Cmd 8' }).click();

    await page.locator(sel.presetVarInput('name')).fill('Changed');
    await page.locator(sel.presetVarInput('city')).fill('Changed Too');
    await expect(page.locator(sel.presetValuesRevert)).toBeVisible();

    await page.locator(sel.presetValuesRevert).click();

    await expect(page.locator(sel.presetVarInput('name'))).toHaveValue('Ada');
    await expect(page.locator(sel.presetVarInput('city'))).toHaveValue('NYC');
    await expect(page.locator(sel.presetValuesRevert)).toHaveCount(0);
  });

  test.fixme(
    'pressing Escape while renaming a preset cancels rather than committing',
    async ({ page, seed, gotoApp }) => {
      // commitChipRename (CommandDetail.tsx) runs on Enter, Escape, AND blur —
      // there is no cancel path at all today, so Escape currently commits the
      // in-progress rename exactly like Enter. This test documents the
      // intended behavior for when that gets fixed; it must not be "fixed"
      // by asserting the current (wrong) commit-on-Escape behavior.
      await seed({
        commands: [seededCommandWithVars('cmd-p9', 'Preset Cmd 9')],
        presets: { 'cmd-p9': [{ id: 'preset-8', name: 'Original', position: 0, values: {} }] },
      });
      await gotoApp();
      await page.locator('.cmd-title').filter({ hasText: 'Preset Cmd 9' }).click();

      await page.locator(sel.presetChip('preset-8')).dblclick();
      const renameInput = page.locator(sel.presetChipRename('preset-8'));
      await renameInput.fill('Should Not Stick');
      await renameInput.press('Escape');

      await expect(page.locator('.preset-chip').filter({ hasText: 'Original' })).toBeVisible();
      await expect(page.locator('.preset-chip').filter({ hasText: 'Should Not Stick' })).toHaveCount(0);
    },
  );
});
