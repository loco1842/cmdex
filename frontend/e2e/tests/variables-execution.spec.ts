import { test, expect } from '../fixtures';
import { sel } from '../utils/selectors';

function seededCommand(id: string, title: string, scriptContent: string, variables: unknown[] = [], presets: unknown[] = []) {
  const now = new Date().toISOString();
  return {
    id,
    title: { String: title, Valid: true },
    description: { String: '', Valid: false },
    scriptContent,
    tags: [],
    variables,
    presets,
    workingDir: {},
    categoryId: '',
    position: 0,
    createdAt: now,
    updatedAt: now,
  };
}

test.describe('Variables & execution', () => {
  test('the Template/Resolved toggle is hidden for a command with no variables', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommand('cmd-v1', 'No Vars', 'echo hello')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'No Vars' }).click();
    await expect(page.locator('.script-mode-toggle')).toBeHidden();
  });

  test('the Template/Resolved toggle is hidden for a new (unsaved) command', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.locator(sel.sidebarAddCommand).click();
    await expect(page.locator('.script-mode-toggle')).toBeHidden();
  });

  test('the Template/Resolved toggle switches labels and content', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [
        seededCommand(
          'cmd-v2',
          'Has Vars',
          'echo "Hello {{name}}"',
          [{ name: 'name', description: '', example: '', default: '', sortOrder: 0 }],
        ),
      ],
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Has Vars' }).click();

    // Starts in Template mode by default (no preset selected).
    const toggle = page.getByRole('button', { name: 'Show Preview' });
    await expect(toggle).toBeVisible();
    await expect(page.locator('.var-missing')).toContainText('{{name}}');

    await toggle.click();
    await expect(page.getByRole('button', { name: 'Show Template' })).toBeVisible();
  });

  test('variable focus highlighting adds var-focused to the matching preview span', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [
        seededCommand(
          'cmd-v3',
          'Focus Test',
          'echo "Hello {{name}}"',
          [{ name: 'name', description: '', example: '', default: '', sortOrder: 0 }],
        ),
      ],
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Focus Test' }).click();
    await page.getByRole('button', { name: 'Show Preview' }).click();

    await page.locator(sel.presetVarInput('name')).focus();
    await expect(page.locator('.var-focused')).toBeVisible();

    await page.locator(sel.presetVarInput('name')).blur();
    await expect(page.locator('.var-focused')).toHaveCount(0);
  });

  test('removing a variable that has a non-empty preset value prompts before saving', async ({ page, seed, gotoApp, toast }) => {
    await seed({
      commands: [
        seededCommand(
          'cmd-v4',
          'Removal Test',
          'echo "{{name}} {{city}}"',
          [
            { name: 'name', description: '', example: '', default: '', sortOrder: 0 },
            { name: 'city', description: '', example: '', default: '', sortOrder: 1 },
          ],
        ),
      ],
      presets: {
        'cmd-v4': [{ id: 'preset-v4', name: 'Has City', position: 0, values: { name: 'Ada', city: 'NYC' } }],
      },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Removal Test' }).click();

    await page.locator(sel.scriptEditEnterBtn).click();
    await page.locator(sel.commandScriptTextarea).fill('echo "{{name}}"');
    await page.keyboard.press('Enter');

    await expect(page.locator(sel.confirmVarRemovalDialog)).toBeVisible();
    await expect(page.locator(sel.confirmVarRemovalDialog)).toContainText('{{city}}');

    await page.locator(sel.confirmVarRemovalConfirm).click();
    await expect(toast(/command saved/i)).toBeVisible();
    await expect(page.locator(sel.presetVarInput('city'))).toHaveCount(0);
    await expect(page.locator(sel.presetVarInput('name'))).toBeVisible();
  });

  test('cancelling the variable-removal prompt does not persist the change', async ({ page, seed, gotoApp, toast }) => {
    await seed({
      commands: [
        seededCommand(
          'cmd-v5',
          'Removal Cancel Test',
          'echo "{{name}} {{city}}"',
          [
            { name: 'name', description: '', example: '', default: '', sortOrder: 0 },
            { name: 'city', description: '', example: '', default: '', sortOrder: 1 },
          ],
        ),
      ],
      presets: {
        'cmd-v5': [{ id: 'preset-v5', name: 'Has City', position: 0, values: { name: 'Ada', city: 'NYC' } }],
      },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Removal Cancel Test' }).click();

    await page.locator(sel.scriptEditEnterBtn).click();
    await page.locator(sel.commandScriptTextarea).fill('echo "{{name}}"');
    await page.keyboard.press('Enter');
    await expect(page.locator(sel.confirmVarRemovalDialog)).toBeVisible();

    await page.locator(sel.confirmVarRemovalCancel).click();

    await expect(page.locator(sel.confirmVarRemovalDialog)).not.toBeVisible();
    await expect(toast(/command saved/i)).toHaveCount(0);
  });

  test('the Run button fills variables first when some are empty, then executes once all are filled', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [
        seededCommand(
          'cmd-v6',
          'Fill Then Run',
          'echo "{{name}}"',
          [{ name: 'name', description: '', example: '', default: '', sortOrder: 0 }],
        ),
      ],
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Fill Then Run' }).click();

    await page.locator(sel.commandRunBtn).click();
    await expect(page.locator(sel.fillVariablesDialog)).toBeVisible();

    await page.locator(sel.fillVarInput('name')).fill('Ada');
    await page.locator(sel.fillVariablesExecute).click();

    await expect(page.locator(sel.fillVariablesDialog)).not.toBeVisible();
  });

  test('cancelling the fill-variables dialog does not execute', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [
        seededCommand(
          'cmd-v7',
          'Fill Cancel',
          'echo "{{name}}"',
          [{ name: 'name', description: '', example: '', default: '', sortOrder: 0 }],
        ),
      ],
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Fill Cancel' }).click();

    await page.locator(sel.commandRunBtn).click();
    await expect(page.locator(sel.fillVariablesDialog)).toBeVisible();
    await page.locator(sel.fillVariablesCancel).click();
    await expect(page.locator(sel.fillVariablesDialog)).not.toBeVisible();
  });

  test('running a command with all variables already filled executes directly, no fill dialog', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [
        seededCommand(
          'cmd-v8',
          'Direct Run',
          'echo "{{name}}"',
          [{ name: 'name', description: '', example: '', default: '', sortOrder: 0 }],
        ),
      ],
      presets: {
        'cmd-v8': [{ id: 'preset-v8', name: 'Preset', position: 0, values: { name: 'Ada' } }],
      },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Direct Run' }).click();

    await page.locator(sel.commandRunBtn).click();
    await expect(page.locator(sel.fillVariablesDialog)).toHaveCount(0);
  });

  test('editing a saved command shows "save before running" instead of executing stale content', async ({ page, seed, gotoApp, toast, pressCmdOrCtrl }) => {
    await seed({ commands: [seededCommand('cmd-v9', 'Dirty Run Guard', 'echo hello')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Dirty Run Guard' }).click();
    await page.locator(sel.commandTitle).fill('Now Dirty');

    await pressCmdOrCtrl('Enter');

    await expect(toast(/save your changes before running/i)).toBeVisible();
  });
});
