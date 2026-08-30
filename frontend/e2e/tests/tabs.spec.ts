import { test, expect } from '../fixtures';
import { sel } from '../utils/selectors';

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

test.describe('Tab system', () => {
  test('editing a saved command marks the tab dirty, and saving clears the dot', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommand('cmd-a', 'Command A')] });
    await gotoApp();

    await page.locator('.cmd-title').filter({ hasText: 'Command A' }).click();
    await expect(page.locator(sel.tabDirtyDot('cmd-a'))).toHaveCount(0);

    await page.locator(sel.commandTitle).fill('Command A Edited');
    await expect(page.locator(sel.tabDirtyDot('cmd-a'))).toBeVisible();

    await page.locator(sel.saveBarSave).click();
    await expect(page.locator(sel.tabDirtyDot('cmd-a'))).toHaveCount(0);
    await expect(page.locator('.cmd-title').filter({ hasText: 'Command A Edited' })).toBeVisible();
  });

  test('editing a script directly (Enter to save) auto-persists without the floating save bar', async ({ page, seed, gotoApp, toast }) => {
    await seed({ commands: [seededCommand('cmd-a2', 'Command A2', 'echo original')] });
    await gotoApp();

    await page.locator('.cmd-title').filter({ hasText: 'Command A2' }).click();
    await page.locator(sel.scriptEditEnterBtn).click();
    await page.locator(sel.commandScriptTextarea).fill('echo changed directly');
    await page.keyboard.press('Enter');

    // doSaveScriptEdit (CommandDetail.tsx) calls onSaveScript directly, which
    // persists and resets both draft and baseline immediately — this path
    // never touches the floating save bar at all.
    await expect(toast(/command saved/i)).toBeVisible();
    await expect(page.locator(sel.floatingSaveBar)).not.toBeVisible();
    await expect(page.locator('.script-preview-compact')).toContainText('echo changed directly');
  });

  test('discarding via the floating save bar reverts to the saved baseline', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommand('cmd-b', 'Command B')] });
    await gotoApp();

    await page.locator('.cmd-title').filter({ hasText: 'Command B' }).click();
    await page.locator(sel.commandTitle).fill('Edited Title');
    await expect(page.locator(sel.floatingSaveBar)).toBeVisible();

    await page.locator(sel.saveBarDiscard).click();

    await expect(page.locator(sel.floatingSaveBar)).not.toBeVisible();
    await expect(page.locator(sel.commandTitle)).toHaveText('Command B');
  });

  test('closing a dirty tab prompts to discard; Cancel keeps it open and dirty', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommand('cmd-c', 'Command C')] });
    await gotoApp();

    await page.locator('.cmd-title').filter({ hasText: 'Command C' }).click();
    await page.locator(sel.commandTitle).fill('Dirty Title');

    await page.locator(sel.tabItem('cmd-c')).locator('.tab-close').click();
    await expect(page.locator(sel.confirmDiscardTabDialog)).toBeVisible();

    await page.locator(sel.confirmDiscardTabCancel).click();
    await expect(page.locator(sel.confirmDiscardTabDialog)).not.toBeVisible();
    await expect(page.locator(sel.tabItem('cmd-c'))).toBeVisible();
    await expect(page.locator(sel.tabDirtyDot('cmd-c'))).toBeVisible();
  });

  test('closing a dirty tab and confirming discards the changes and closes it', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommand('cmd-d', 'Command D')] });
    await gotoApp();

    await page.locator('.cmd-title').filter({ hasText: 'Command D' }).click();
    await page.locator(sel.commandTitle).fill('Dirty Title');

    await page.locator(sel.tabItem('cmd-d')).locator('.tab-close').click();
    await page.locator(sel.confirmDiscardTabConfirm).click();

    await expect(page.locator(sel.tabItem('cmd-d'))).toHaveCount(0);
  });

  test('re-opening an already-open tab re-titles it but does not clobber unsaved edits', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommand('cmd-e', 'Command E')] });
    await gotoApp();

    await page.locator('.cmd-title').filter({ hasText: 'Command E' }).click();
    await page.locator(sel.commandTitle).fill('Not Yet Saved');
    await expect(page.locator(sel.floatingSaveBar)).toBeVisible();

    // Click the same sidebar item again — openTab's isExisting guard must
    // skip the script refetch that would otherwise overwrite the draft.
    await page.locator(sel.commandItem('cmd-e')).click();

    await expect(page.locator(sel.commandTitle)).toHaveText('Not Yet Saved');
    await expect(page.locator(sel.floatingSaveBar)).toBeVisible();
  });

  test('a new command tab shows "New Command" until a script is typed, then a script-derived title', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.locator(sel.sidebarAddCommand).click();

    await expect(page.locator(sel.tabBar).locator('.tab-item .tab-title')).toHaveText('New Command');

    await page.locator(sel.commandScriptTextarea).fill('echo from the new tab');
    await expect(page.locator(sel.tabBar).locator('.tab-item .tab-title')).toHaveText('echo from the new tab');
  });

  test('a long script body is truncated to 50 chars plus an ellipsis in the tab title', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.locator(sel.sidebarAddCommand).click();

    const long = 'x'.repeat(60);
    await page.locator(sel.commandScriptTextarea).fill(long);
    await expect(page.locator(sel.tabBar).locator('.tab-item .tab-title')).toHaveText('x'.repeat(50) + '...');
  });

  test('an explicit title always wins over the script-derived fallback', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.locator(sel.sidebarAddCommand).click();
    await page.locator(sel.commandScriptTextarea).fill('echo hello');

    await page.locator('.hover-actions-host.script-area-hover').hover();
    await page.locator('.add-title-pill').click();
    await page.locator(sel.commandTitle).fill('Explicit Title');

    await expect(page.locator(sel.tabBar).locator('.tab-item .tab-title')).toHaveText('Explicit Title');
  });

  test('switching away from an unsaved tab and back preserves the in-progress edit', async ({ page, seed, gotoApp, visibleTabShell }) => {
    await seed({ commands: [seededCommand('cmd-f', 'Command F'), seededCommand('cmd-g', 'Command G')] });
    await gotoApp();

    // Both tabs stay mounted once opened, so command-title matches once per
    // open tab — scope every assertion through the currently-visible shell.
    await page.locator('.cmd-title').filter({ hasText: 'Command F' }).click();
    await visibleTabShell().locator(sel.commandTitle).fill('Half-typed edit');

    await page.locator('.cmd-title').filter({ hasText: 'Command G' }).click();
    await expect(visibleTabShell().locator(sel.commandTitle)).toHaveText('Command G');

    await page.locator(sel.tabItem('cmd-f')).click();
    await expect(visibleTabShell().locator(sel.commandTitle)).toHaveText('Half-typed edit');
  });

  test('saving a brand-new command tab keeps exactly one tab open (no duplicate)', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.locator(sel.sidebarAddCommand).click();
    await page.locator(sel.commandScriptTextarea).fill('echo brand new');
    await page.locator(sel.saveBarSave).click();

    // Scope to the command tab bar: the terminal pane's own tab strip also
    // renders `.tab-item` (one auto-created default terminal session).
    await expect(page.locator(sel.tabBar).locator('.tab-item')).toHaveCount(1);
    await expect(page.locator('.cmd-title').filter({ hasText: 'echo brand new' })).toBeVisible();
  });

  test('closing the only open tab returns to the welcome screen', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommand('cmd-h', 'Command H')] });
    await gotoApp();

    await page.locator('.cmd-title').filter({ hasText: 'Command H' }).click();
    await expect(page.locator(sel.tabBar)).toBeVisible();

    await page.locator(sel.tabItem('cmd-h')).locator('.tab-close').click();

    await expect(page.locator(sel.tabBar)).toHaveCount(0);
    await expect(page.locator('.welcome-tab-subtitle')).toBeVisible();
  });

  test('closing the active middle tab of three selects the tab to its right, not the left', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [
        seededCommand('cmd-i', 'Command I'),
        seededCommand('cmd-j', 'Command J'),
        seededCommand('cmd-k', 'Command K'),
      ],
    });
    await gotoApp();

    await page.locator('.cmd-title').filter({ hasText: 'Command I' }).click();
    await page.locator('.cmd-title').filter({ hasText: 'Command J' }).click();
    await page.locator('.cmd-title').filter({ hasText: 'Command K' }).click();
    // Re-activate J (the middle tab) without closing I or K.
    await page.locator(sel.tabItem('cmd-j')).click();
    await expect(page.locator(sel.tabItem('cmd-j'))).toHaveClass(/active/);

    await page.locator(sel.tabItem('cmd-j')).locator('.tab-close').click();

    await expect(page.locator(sel.tabItem('cmd-j'))).toHaveCount(0);
    await expect(page.locator(sel.tabItem('cmd-k'))).toHaveClass(/active/);
  });
});
