import { test, expect } from '../fixtures';
import { sel } from '../utils/selectors';

function seededCommand(id: string, title: string, scriptContent = `echo ${id}`) {
  const now = new Date().toISOString();
  return {
    id,
    title: { String: title, Valid: true },
    description: { String: '', Valid: false },
    scriptContent,
    tags: id === 'cmd-tagged' ? ['deploy'] : [],
    variables: [],
    presets: [],
    workingDir: {},
    categoryId: '',
    position: 0,
    createdAt: now,
    updatedAt: now,
  };
}

test.describe('Command palette', () => {
  test('opens via Cmd/Ctrl+P and closes via Escape', async ({ page, gotoApp, pressCmdOrCtrl }) => {
    await gotoApp();
    await pressCmdOrCtrl('p');
    await expect(page.locator(sel.commandPalette)).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(page.locator(sel.commandPalette)).not.toBeVisible();
  });

  test('also opens via Cmd/Ctrl+F', async ({ page, gotoApp, pressCmdOrCtrl }) => {
    await gotoApp();
    await pressCmdOrCtrl('f');
    await expect(page.locator(sel.commandPalette)).toBeVisible();
  });

  test('also opens via the bare (unmodified) Ctrl+P', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.keyboard.press('Control+p');
    await expect(page.locator(sel.commandPalette)).toBeVisible();
  });

  test('filters results by title as you type', async ({ page, seed, gotoApp, pressCmdOrCtrl }) => {
    await seed({ commands: [seededCommand('cmd-alpha', 'Alpha Command'), seededCommand('cmd-beta', 'Beta Command')] });
    await gotoApp();
    await pressCmdOrCtrl('p');

    await page.locator(sel.paletteInput).fill('Alpha');
    await expect(page.locator(sel.paletteItem('cmd-alpha'))).toBeVisible();
    await expect(page.locator(sel.paletteItem('cmd-beta'))).toHaveCount(0);
  });

  test('a leading #tag filters to commands carrying that tag', async ({ page, seed, gotoApp, pressCmdOrCtrl }) => {
    await seed({ commands: [seededCommand('cmd-tagged', 'Deploy Thing'), seededCommand('cmd-untagged', 'Other Thing')] });
    await gotoApp();
    await pressCmdOrCtrl('p');

    await page.locator(sel.paletteInput).fill('#deploy');
    await expect(page.locator(sel.paletteItem('cmd-tagged'))).toBeVisible();
    await expect(page.locator(sel.paletteItem('cmd-untagged'))).toHaveCount(0);
  });

  test('shows the empty state when nothing matches', async ({ page, gotoApp, pressCmdOrCtrl }) => {
    await gotoApp();
    await pressCmdOrCtrl('p');
    await page.locator(sel.paletteInput).fill('nothing matches this at all');
    await expect(page.locator(sel.paletteEmpty)).toBeVisible();
  });

  test('ArrowDown/ArrowUp clamp at the first and last result rather than wrapping', async ({ page, seed, gotoApp, pressCmdOrCtrl }) => {
    await seed({ commands: [seededCommand('cmd-x1', 'X One'), seededCommand('cmd-x2', 'X Two')] });
    await gotoApp();
    await pressCmdOrCtrl('p');
    await page.locator(sel.paletteInput).fill('X ');

    // Starts at index 0 (first item active).
    await expect(page.locator(sel.paletteItem('cmd-x1'))).toHaveClass(/active/);

    await page.keyboard.press('ArrowDown');
    await expect(page.locator(sel.paletteItem('cmd-x2'))).toHaveClass(/active/);

    // Already at the last item — one more ArrowDown must not wrap to the first.
    await page.keyboard.press('ArrowDown');
    await expect(page.locator(sel.paletteItem('cmd-x2'))).toHaveClass(/active/);
    await expect(page.locator(sel.paletteItem('cmd-x1'))).not.toHaveClass(/active/);

    await page.keyboard.press('ArrowUp');
    await expect(page.locator(sel.paletteItem('cmd-x1'))).toHaveClass(/active/);
    await page.keyboard.press('ArrowUp');
    await expect(page.locator(sel.paletteItem('cmd-x1'))).toHaveClass(/active/);
  });

  test('Enter opens the highlighted command as a tab', async ({ page, seed, gotoApp, pressCmdOrCtrl }) => {
    await seed({ commands: [seededCommand('cmd-open-me', 'Open Me')] });
    await gotoApp();
    await pressCmdOrCtrl('p');
    await page.locator(sel.paletteInput).fill('Open Me');
    await page.keyboard.press('Enter');

    await expect(page.locator(sel.commandPalette)).not.toBeVisible();
    await expect(page.locator(sel.tabItem('cmd-open-me'))).toBeVisible();
  });
});

test.describe('Global keyboard shortcuts', () => {
  test('Cmd/Ctrl+N opens a new command tab', async ({ page, gotoApp, pressCmdOrCtrl }) => {
    await gotoApp();
    await pressCmdOrCtrl('n');
    await expect(page.locator(sel.tabBar).locator('.tab-item .tab-title')).toHaveText('New Command');
  });

  test('Cmd/Ctrl+S saves the active dirty tab', async ({ page, seed, gotoApp, pressCmdOrCtrl }) => {
    await seed({ commands: [seededCommand('cmd-save', 'Save Me')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Save Me' }).click();
    await page.locator(sel.commandTitle).fill('Saved via shortcut');

    await pressCmdOrCtrl('s');

    await expect(page.locator(sel.floatingSaveBar)).not.toBeVisible();
    await expect(page.locator('.cmd-title').filter({ hasText: 'Saved via shortcut' })).toBeVisible();
  });

  test('Cmd/Ctrl+Shift+Backspace discards the active dirty tab', async ({ page, seed, gotoApp, pressCmdOrCtrl }) => {
    await seed({ commands: [seededCommand('cmd-discard', 'Discard Me')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Discard Me' }).click();
    await page.locator(sel.commandTitle).fill('Should not stick');

    await pressCmdOrCtrl('Shift+Backspace');

    // The dirty bar disappearing is the reliable signal that the discard
    // happened (draft reset to the baseline). The title <h1> itself is NOT
    // asserted here: CommandDetail's title-sync effect skips writing DOM
    // textContent while the element is focused, and the shortcut never
    // blurs it, so the visibly-typed text can be left stale in the DOM
    // indefinitely — a real (if minor/cosmetic) quirk, not something this
    // interaction can be made to "wait out".
    await expect(page.locator(sel.floatingSaveBar)).not.toBeVisible();
  });

  test('Ctrl+W closes the active command tab', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommand('cmd-close', 'Close Me')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Close Me' }).click();
    await expect(page.locator(sel.tabItem('cmd-close'))).toBeVisible();

    await page.keyboard.press('Control+w');

    await expect(page.locator(sel.tabItem('cmd-close'))).toHaveCount(0);
  });

  test('Cmd/Ctrl+1..3 jump directly to the Nth open tab', async ({ page, seed, gotoApp, pressCmdOrCtrl }) => {
    await seed({
      commands: [
        seededCommand('cmd-n1', 'Tab One'),
        seededCommand('cmd-n2', 'Tab Two'),
        seededCommand('cmd-n3', 'Tab Three'),
      ],
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Tab One' }).click();
    await page.locator('.cmd-title').filter({ hasText: 'Tab Two' }).click();
    await page.locator('.cmd-title').filter({ hasText: 'Tab Three' }).click();

    await pressCmdOrCtrl('1');
    await expect(page.locator(sel.tabItem('cmd-n1'))).toHaveClass(/active/);

    await pressCmdOrCtrl('3');
    await expect(page.locator(sel.tabItem('cmd-n3'))).toHaveClass(/active/);
  });

  test('Cmd/Ctrl+9 toggles back to the previously-active tab', async ({ page, seed, gotoApp, pressCmdOrCtrl }) => {
    await seed({ commands: [seededCommand('cmd-p9a', 'Prev A'), seededCommand('cmd-p9b', 'Prev B')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Prev A' }).click();
    await page.locator('.cmd-title').filter({ hasText: 'Prev B' }).click();
    await expect(page.locator(sel.tabItem('cmd-p9b'))).toHaveClass(/active/);

    await pressCmdOrCtrl('9');
    await expect(page.locator(sel.tabItem('cmd-p9a'))).toHaveClass(/active/);

    await pressCmdOrCtrl('9');
    await expect(page.locator(sel.tabItem('cmd-p9b'))).toHaveClass(/active/);
  });

  test('Cmd/Ctrl+0 jumps to the last open tab', async ({ page, seed, gotoApp, pressCmdOrCtrl }) => {
    await seed({
      commands: [seededCommand('cmd-l1', 'Last One'), seededCommand('cmd-l2', 'Last Two'), seededCommand('cmd-l3', 'Last Three')],
    });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Last One' }).click();
    await page.locator('.cmd-title').filter({ hasText: 'Last Two' }).click();
    await page.locator('.cmd-title').filter({ hasText: 'Last Three' }).click();
    await page.locator(sel.tabItem('cmd-l1')).click();

    await pressCmdOrCtrl('0');
    await expect(page.locator(sel.tabItem('cmd-l3'))).toHaveClass(/active/);
  });

  test('Ctrl+Tab cycles command tabs forward and wraps around', async ({ page, seed, gotoApp }) => {
    await seed({ commands: [seededCommand('cmd-c1', 'Cycle One'), seededCommand('cmd-c2', 'Cycle Two')] });
    await gotoApp();
    await page.locator('.cmd-title').filter({ hasText: 'Cycle One' }).click();
    await page.locator('.cmd-title').filter({ hasText: 'Cycle Two' }).click();
    await expect(page.locator(sel.tabItem('cmd-c2'))).toHaveClass(/active/);

    await page.keyboard.press('Control+Tab');
    await expect(page.locator(sel.tabItem('cmd-c1'))).toHaveClass(/active/);

    await page.keyboard.press('Control+Tab');
    await expect(page.locator(sel.tabItem('cmd-c2'))).toHaveClass(/active/);
  });

  test('Ctrl+`  toggles the terminal panel collapsed state', async ({ page, gotoApp }) => {
    await gotoApp();
    await expect(page.getByRole('button', { name: 'Collapse terminal panel' })).toBeVisible();

    await page.keyboard.press('Control+Backquote');
    await expect(page.getByRole('button', { name: 'Expand terminal panel' })).toBeVisible();

    await page.keyboard.press('Control+Backquote');
    await expect(page.getByRole('button', { name: 'Collapse terminal panel' })).toBeVisible();
  });

  test('Cmd/Ctrl+, opens the settings window', async ({ page, gotoApp, pressCmdOrCtrl }) => {
    await gotoApp();
    await pressCmdOrCtrl(',');

    await expect
      .poll(() => page.evaluate(() => window.__cmdexE2E!.callLog.some((c) => c.method === 'ShowSettingsWindow')))
      .toBe(true);
  });

  test('Cmd/Ctrl+T inside the terminal pane creates a new terminal session, not a command tab', async ({ page, gotoApp }) => {
    await gotoApp();
    await expect(page.locator(sel.terminalTabBar).locator('.tab-item')).toHaveCount(1);

    // xterm's real input target (`.xterm-helper-textarea`) is deliberately
    // sized to 0x0 and moved off-screen (so its IME/cursor never renders) —
    // Playwright's actionability check refuses to click a zero-size element.
    // Click the visible container instead; xterm delegates focus to the
    // hidden textarea internally, exactly as a real user's click would.
    await page.locator('.terminal-container:visible').click();
    await expect(page.getByRole('textbox', { name: 'Terminal input' })).toBeFocused();
    await page.keyboard.press('Control+t');

    await expect(page.locator(sel.terminalTabBar).locator('.tab-item')).toHaveCount(2);
    await expect(page.locator(sel.tabBar)).toHaveCount(0);
  });

  test('Cmd/Ctrl+T outside the terminal pane opens a new command tab, not a terminal session', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.locator('.sidebar').click();
    await page.keyboard.press('Control+t');

    await expect(page.locator(sel.tabBar).locator('.tab-item .tab-title')).toHaveText('New Command');
    await expect(page.locator(sel.terminalTabBar).locator('.tab-item')).toHaveCount(1);
  });

  // KeyboardShortcutsDialog and WelcomeTab both advertise Cmd/Ctrl+Shift+?
  // (SHORTCUTS.shortcuts, lib/shortcuts.ts) as the way to open the shortcuts
  // dialog, but it is never actually registered in App.tsx's
  // useKeyboardShortcuts call — the dialog only opens via the native menu's
  // `open-shortcuts` event. This documents the intended behavior; it must
  // not be "fixed" by asserting the shortcut does nothing.
  test.fixme('Cmd/Ctrl+Shift+? opens the keyboard shortcuts dialog', async ({ page, gotoApp, pressCmdOrCtrl }) => {
    await gotoApp();
    await pressCmdOrCtrl('Shift+?');
    await expect(page.getByText('Keyboard Shortcuts')).toBeVisible();
  });
});
