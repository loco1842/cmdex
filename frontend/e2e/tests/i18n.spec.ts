import { test, expect, type Page } from '../fixtures';
import { sel } from '../utils/selectors';

// Guards against the exact class of bug fixed in the correctness/polish
// track: 13 i18n keys were missing from en.json, so a malformed theme import
// literally rendered the toast text `settings.themeInvalidJson` instead of a
// sentence. i18next's fallbackLng: 'en' with no parseMissingKeyHandler
// (i18n.ts) means a missing key renders as the raw dotted key string, so any
// occurrence of `<namespace>.<keyName>` in rendered text is a real bug, not
// a coincidence — none of these namespaces otherwise appear in prose.
const I18N_NAMESPACES = [
  'common', 'sidebar', 'commandDetail', 'commandEditor', 'categoryEditor',
  'variablePrompt', 'app', 'toast', 'settings', 'resizablePanel', 'welcome',
];
const RAW_KEY_RE = new RegExp(`\\b(?:${I18N_NAMESPACES.join('|')})\\.[a-zA-Z][a-zA-Z0-9.]*\\b`);

async function assertNoRawI18nKeys(page: Page, where: string) {
  const text = await page.evaluate(() => document.body.innerText);
  const match = text.match(RAW_KEY_RE);
  expect(match, `found a raw i18n key in ${where}: ${match?.[0]}`).toBeNull();
}

function seededCommand(id: string, title: string, scriptContent = `echo ${id}`, variables: unknown[] = []) {
  const now = new Date().toISOString();
  return {
    id,
    title: { String: title, Valid: true },
    description: { String: '', Valid: false },
    scriptContent,
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

test.describe('i18n key resolution guard', () => {
  test('the welcome screen renders no raw i18n keys', async ({ page, gotoApp }) => {
    await gotoApp();
    await assertNoRawI18nKeys(page, 'the welcome screen');
  });

  test('a new command tab renders no raw i18n keys', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.locator(sel.sidebarAddCommand).click();
    await assertNoRawI18nKeys(page, 'a new command tab');
  });

  test('an open command with variables and presets renders no raw i18n keys', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [seededCommand('cmd-i18n-1', 'I18n Check', 'echo {{name}}', [
        { name: 'name', description: '', example: '', default: '', sortOrder: 0 },
      ])],
      presets: { 'cmd-i18n-1': [{ id: 'preset-i18n-1', name: 'A Preset', position: 0, values: { name: 'Ada' } }] },
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'I18n Check' }).click();
    await assertNoRawI18nKeys(page, 'an open command with variables/presets');
  });

  test('the category editor dialog renders no raw i18n keys', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.locator('.sidebar-content').click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'New Category' }).click();
    await assertNoRawI18nKeys(page, 'the category editor dialog');
  });

  test('the fill-variables dialog renders no raw i18n keys', async ({ page, seed, gotoApp }) => {
    await seed({
      commands: [seededCommand('cmd-i18n-2', 'Fill Dialog Check', 'echo {{name}}', [
        { name: 'name', description: '', example: '', default: '', sortOrder: 0 },
      ])],
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Fill Dialog Check' }).click();
    await page.locator(sel.commandRunBtn).click();
    await expect(page.locator(sel.fillVariablesDialog)).toBeVisible();
    await assertNoRawI18nKeys(page, 'the fill-variables dialog');
  });

  test('the discard-tab confirm dialog renders no raw i18n keys', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommand('cmd-i18n-3', 'Discard Dialog Check')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Discard Dialog Check' }).click();
    await page.locator(sel.commandTitle).fill('Dirty');
    await page.locator(sel.tabItem('cmd-i18n-3')).locator('.tab-close').click();
    await expect(page.locator(sel.confirmDiscardTabDialog)).toBeVisible();
    await assertNoRawI18nKeys(page, 'the discard-tab confirm dialog');
  });

  test('the command palette renders no raw i18n keys', async ({ page, seed, gotoApp, pressCmdOrCtrl }) => {
    await seed({ commands: [seededCommand('cmd-i18n-4', 'Palette Check')] });
    await gotoApp();
    await pressCmdOrCtrl('p');
    await assertNoRawI18nKeys(page, 'the command palette');
  });

  test('the settings window renders no raw i18n keys across all three tabs', async ({ page, gotoSettings }) => {
    await gotoSettings();
    await assertNoRawI18nKeys(page, 'the settings window (Appearance tab)');

    await page.getByRole('tab', { name: 'Typography' }).click();
    await assertNoRawI18nKeys(page, 'the settings window (Typography tab)');

    await page.getByRole('tab', { name: 'General' }).click();
    await assertNoRawI18nKeys(page, 'the settings window (General tab)');
  });

  test('the Danger Zone confirm panel renders no raw i18n keys', async ({ page, gotoSettings }) => {
    await gotoSettings();
    await page.getByRole('tab', { name: 'General' }).click();
    await page.locator(sel.dangerZoneResetButton).click();
    await assertNoRawI18nKeys(page, 'the Danger Zone confirm panel');
  });
});
