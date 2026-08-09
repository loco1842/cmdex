import { test, expect } from '@playwright/test';
import '../utils/types';

const SCRIPT_TEXTAREA = '[data-testid="command-script"] textarea';
const SIDEBAR_CMD_TITLE = '.cmd-title';
const SAVE_BAR = '[data-testid="floating-save-bar"]';
const SAVE_BTN = '[data-testid="save-bar-save"]';

test.describe('Commands', () => {
  // ── Create ──────────────────────────────────────────────

  test('creates a new command with script only', async ({ page }) => {
    await page.goto('/');

    await page.locator('[data-testid="sidebar-add-command"]').click();
    await page.waitForTimeout(300);

    await expect(page.locator(SCRIPT_TEXTAREA)).toBeVisible();
    await page.locator(SCRIPT_TEXTAREA).fill('echo "hello world"');

    await expect(page.locator(SAVE_BAR)).toBeVisible();
    await page.locator(SAVE_BTN).click();

    await expect(
      page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'echo "hello world"' }),
    ).toBeVisible({ timeout: 5000 });
  });

  test('creates a new command with a title', async ({ page }) => {
    await page.goto('/');

    await page.locator('[data-testid="sidebar-add-command"]').click();
    await page.waitForTimeout(300);

    await page.locator(SCRIPT_TEXTAREA).fill('echo "hello"');

    // Reveal the title input by clicking the "Add title" pill. The pill has
    // pointer-events: none until its .hover-actions-host ancestor is hovered
    // (force:true skips Playwright's actionability checks but not the
    // browser's native pointer-events hit-test), so hover the ancestor first.
    await page.locator('.hover-actions-host.script-area-hover').hover();
    await page.locator('.add-title-pill').click();
    await expect(page.locator('[data-testid="command-title"]')).toBeVisible();

    // Enter an explicit title different from the script content
    await page.locator('[data-testid="command-title"]').fill('My Custom Title');

    await expect(page.locator(SAVE_BAR)).toBeVisible();
    await page.locator(SAVE_BTN).click();

    await expect(
      page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'My Custom Title' }),
    ).toBeVisible({ timeout: 5000 });
  });

  // ── Edit ────────────────────────────────────────────────

  test('edits command title inline', async ({ page }) => {
    const now = new Date().toISOString();
    await page.addInitScript((ts) => {
      window.__cmdexE2E_SEED__ = {
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
            createdAt: ts,
            updatedAt: ts,
          },
        ],
      };
    }, now);
    await page.goto('/');

    await page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'Original' }).click();
    await page.waitForTimeout(300);

    const titleEl = page.locator('[data-testid="command-title"]');
    await expect(titleEl).toBeVisible();
    await titleEl.fill('Renamed');

    await expect(page.locator(SAVE_BAR)).toBeVisible();
    await page.locator(SAVE_BTN).click();

    await expect(page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'Renamed' })).toBeVisible();
    await expect(page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'Original' })).not.toBeVisible();
  });

  // ── Delete ──────────────────────────────────────────────

  test('deletes a command via sidebar hover', async ({ page }) => {
    const now = new Date().toISOString();
    await page.addInitScript((ts) => {
      window.__cmdexE2E_SEED__ = {
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
            createdAt: ts,
            updatedAt: ts,
          },
        ],
      };
    }, now);
    await page.goto('/');

    await expect(page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'To Delete' })).toBeVisible();

    const cmdItem = page
      .locator('.command-item')
      .filter({ has: page.locator(SIDEBAR_CMD_TITLE, { hasText: 'To Delete' }) });
    await cmdItem.hover();
    await page.waitForTimeout(200);

    const trashBtn = cmdItem.locator('.cmd-trash-btn');
    await expect(trashBtn).toBeVisible();
    await trashBtn.click();
    await page.waitForTimeout(200);

    const deleteBtn = cmdItem.locator('.cmd-delete-icon-btn');
    await expect(deleteBtn).toBeVisible();
    await deleteBtn.click();

    await expect(page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'To Delete' })).not.toBeVisible();
  });

  // ── Open from sidebar ───────────────────────────────────

  test('opens existing command and shows content', async ({ page }) => {
    const now = new Date().toISOString();
    await page.addInitScript((ts) => {
      window.__cmdexE2E_SEED__ = {
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
            createdAt: ts,
            updatedAt: ts,
          },
        ],
      };
    }, now);
    await page.goto('/');

    await page.locator(SIDEBAR_CMD_TITLE).filter({ hasText: 'Open Test' }).click();
    await page.waitForTimeout(300);

    await expect(page.locator('[data-testid="tab-bar"]')).toBeVisible();
    await expect(page.locator('[data-testid="command-title"]')).toBeVisible();

    const preview = page.locator('.script-preview-compact');
    await expect(preview).toBeVisible();
    await expect(preview).toContainText('echo');
  });

  // ── Variables ───────────────────────────────────────────

  test('detects {{variables}} and shows inputs', async ({ page }) => {
    await page.goto('/');

    await page.locator('[data-testid="sidebar-add-command"]').click();
    await page.waitForTimeout(300);

    await page.locator(SCRIPT_TEXTAREA).fill('echo "Hello {{name}} from {{city}}"');

    await expect(page.locator(SAVE_BAR)).toBeVisible();
    await page.locator(SAVE_BTN).click();
    await page.waitForTimeout(800);

    const varInputs = page.locator('.preset-var-input');
    const count = await varInputs.count();
    expect(count).toBeGreaterThanOrEqual(2);
  });
});
