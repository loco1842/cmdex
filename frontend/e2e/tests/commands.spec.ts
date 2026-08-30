import { test, expect } from '../fixtures';
import { sel } from '../utils/selectors';

const SCRIPT_TEXTAREA = sel.commandScriptTextarea;
const SIDEBAR_CMD_TITLE = '.cmd-title';
const SAVE_BAR = sel.floatingSaveBar;
const SAVE_BTN = sel.saveBarSave;

test.describe('Commands', () => {
  // ── Create ──────────────────────────────────────────────

  test('creates a new command with script only', async ({ page, gotoApp }) => {
    await gotoApp();

    await page.locator(sel.sidebarAddCommand).click();

    await expect(page.locator(SCRIPT_TEXTAREA)).toBeVisible();
    await page.locator(SCRIPT_TEXTAREA).fill('echo "hello world"');

    await expect(page.locator(SAVE_BAR)).toBeVisible();
    await page.locator(SAVE_BTN).click();

    await expect(
      page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'echo "hello world"' }),
    ).toBeVisible();
  });

  test('creates a new command with a title', async ({ page, gotoApp }) => {
    await gotoApp();

    await page.locator(sel.sidebarAddCommand).click();
    await page.locator(SCRIPT_TEXTAREA).fill('echo "hello"');

    // Reveal the title input by clicking the "Add title" pill. The pill has
    // pointer-events: none until its .hover-actions-host ancestor is hovered
    // (force:true skips Playwright's actionability checks but not the
    // browser's native pointer-events hit-test), so hover the ancestor first.
    await page.locator('.hover-actions-host.script-area-hover').hover();
    await page.locator('.add-title-pill').click();
    await expect(page.locator(sel.commandTitle)).toBeVisible();

    // Enter an explicit title different from the script content
    await page.locator(sel.commandTitle).fill('My Custom Title');

    await expect(page.locator(SAVE_BAR)).toBeVisible();
    await page.locator(SAVE_BTN).click();

    await expect(
      page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'My Custom Title' }),
    ).toBeVisible();
  });

  // ── Edit ────────────────────────────────────────────────

  test('edits command title inline', async ({ page, seed, gotoApp }) => {
    const now = new Date().toISOString();
    await seed({
      commands: [
        {
          id: 'cmd-edit-1',
          title: { String: 'Original', Valid: true },
          description: { String: '', Valid: false },
          scriptContent: 'echo original',
          tags: [],
          variables: [],
          presets: [],
          workingDir: {},
          categoryId: '',
          position: 0,
          createdAt: now,
          updatedAt: now,
        },
      ],
    });
    await gotoApp();

    await page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'Original' }).click();

    const titleEl = page.locator(sel.commandTitle);
    await expect(titleEl).toBeVisible();
    await titleEl.fill('Renamed');

    await expect(page.locator(SAVE_BAR)).toBeVisible();
    await page.locator(SAVE_BTN).click();

    await expect(page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'Renamed' })).toBeVisible();
    await expect(page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'Original' })).not.toBeVisible();
  });

  // ── Delete ──────────────────────────────────────────────

  test('deletes a command via sidebar hover', async ({ page, seed, gotoApp }) => {
    const now = new Date().toISOString();
    await seed({
      commands: [
        {
          id: 'cmd-del-1',
          title: { String: 'To Delete', Valid: true },
          description: { String: '', Valid: false },
          scriptContent: 'echo bye',
          tags: [],
          variables: [],
          presets: [],
          workingDir: {},
          categoryId: '',
          position: 0,
          createdAt: now,
          updatedAt: now,
        },
      ],
    });
    await gotoApp();

    await expect(page.locator(sel.commandItem('cmd-del-1'))).toBeVisible();

    const cmdItem = page.locator(sel.commandItem('cmd-del-1'));
    await cmdItem.hover();

    const trashBtn = cmdItem.locator('.cmd-trash-btn');
    await expect(trashBtn).toBeVisible();
    await trashBtn.click();

    const deleteBtn = cmdItem.locator('.cmd-delete-icon-btn');
    await expect(deleteBtn).toBeVisible();
    await deleteBtn.click();

    await expect(page.locator(sel.commandItem('cmd-del-1'))).not.toBeVisible();
  });

  // ── Open from sidebar ───────────────────────────────────

  test('opens existing command and shows content', async ({ page, seed, gotoApp }) => {
    const now = new Date().toISOString();
    await seed({
      commands: [
        {
          id: 'cmd-open-1',
          title: { String: 'Open Test', Valid: true },
          description: { String: 'Desc here', Valid: true },
          scriptContent: '#!/bin/bash\necho hello',
          tags: ['cli'],
          variables: [],
          presets: [],
          workingDir: {},
          categoryId: '',
          position: 0,
          createdAt: now,
          updatedAt: now,
        },
      ],
    });
    await gotoApp();

    await page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'Open Test' }).click();

    await expect(page.locator(sel.tabBar)).toBeVisible();
    await expect(page.locator(sel.commandTitle)).toBeVisible();

    const preview = page.locator('.script-preview-compact');
    await expect(preview).toBeVisible();
    await expect(preview).toContainText('echo');
  });

  // ── Variables ───────────────────────────────────────────

  test('detects {{variables}} and shows inputs', async ({ page, gotoApp }) => {
    await gotoApp();

    await page.locator(sel.sidebarAddCommand).click();
    await page.locator(SCRIPT_TEXTAREA).fill('echo "Hello {{name}} from {{city}}"');

    await expect(page.locator(SAVE_BAR)).toBeVisible();
    await page.locator(SAVE_BTN).click();

    await expect(page.locator(sel.presetVarInput('name'))).toBeVisible();
    await expect(page.locator(sel.presetVarInput('city'))).toBeVisible();
    await expect(page.locator('.preset-var-input')).toHaveCount(2);
  });
});
