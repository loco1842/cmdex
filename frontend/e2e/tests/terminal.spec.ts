import { test, expect, type Page } from '../fixtures';

// Extracts the first terminal tab's session id from its `terminal-tab-<id>`
// testid, rather than hardcoding mock-internal ids like `term-1`.
async function activeSessionIdFromTab(page: Page): Promise<string> {
  const testId = await page.locator('.tab-item').first().getAttribute('data-testid');
  const id = testId?.replace('terminal-tab-', '');
  if (!id) throw new Error('Could not resolve a session id from the first terminal tab');
  return id;
}

// Regression coverage for the double-start bug: Terminal.tsx used to
// unconditionally call Start() on every mount, even when the backend
// (TerminalService.ServiceStartup / CreateSession) had already started the
// session's shell — tearing down the healthy PTY and spawning a redundant
// second shell. The fix threads the backend's already-known `running` state
// into Terminal.tsx as `initiallyRunning` and skips the mount-time Start()
// call when it's already true. The mock backend mirrors the real one:
// CreateSession's returned SessionInfo always has running: true.

test.describe('Terminal', () => {
  test.beforeEach(async ({ seed }) => {
    await seed({ settings: {} });
  });

  test('does not call Start for the session the backend already started on first load', async ({ page, gotoApp }) => {
    await gotoApp();

    // The app auto-creates a default session on first load (no sessions,
    // no active session) — wait for its tab to render.
    await expect(page.locator('.tab-item')).toHaveCount(1);

    const counts = await page.evaluate(() => window.__cmdexE2E!.terminalCallCounts);
    expect(counts.CreateSession).toBe(1);
    expect(counts.Start).toBe(0);
  });

  test('creating a new terminal tab does not call Start either', async ({ page, gotoApp }) => {
    await gotoApp();
    await expect(page.locator('.tab-item')).toHaveCount(1);

    await page.locator('.tab-new-session-btn').click();
    await expect(page.locator('.tab-item')).toHaveCount(2);

    const counts = await page.evaluate(() => window.__cmdexE2E!.terminalCallCounts);
    expect(counts.CreateSession).toBe(2);
    expect(counts.Start).toBe(0);
  });

  test('every session tab shows as running, never stuck in a stopped/restarting state', async ({ page, gotoApp }) => {
    await gotoApp();
    await expect(page.locator('.tab-item')).toHaveCount(1);

    // The status dot reflects SessionInfo.running from the backend — since
    // the mock (like the real backend) always starts a session before
    // returning it, this must read "running" immediately, not "stopped".
    await expect(page.locator('.tab-status-dot')).toHaveClass(/running/);
  });

  test('clicking the clear button calls Clear for the active session', async ({ page, gotoApp }) => {
    await gotoApp();
    await expect(page.locator('.tab-item')).toHaveCount(1);

    // The tab list (TerminalTabBar) and the xterm.js-backed TerminalComponent
    // mount independently — waiting only for the tab risks clicking Clear
    // before terminalRefs.current[activeSessionId] is populated, in which
    // case the click silently no-ops. xterm's own "Terminal input" textarea
    // appearing is a reliable signal that the component has finished mounting.
    await expect(page.getByRole('textbox', { name: 'Terminal input' })).toBeAttached();

    await page.locator('.terminal-clear-btn').click();

    await expect
      .poll(async () => (await page.evaluate(() => window.__cmdexE2E!.terminalCallCounts)).Clear)
      .toBe(1);
  });

  test('switching the active tab keeps the previous session mounted (Start still never called)', async ({ page, gotoApp }) => {
    await gotoApp();
    await expect(page.locator('.tab-item')).toHaveCount(1);

    await page.locator('.tab-new-session-btn').click();
    await expect(page.locator('.tab-item')).toHaveCount(2);

    // Switch back to the first tab and confirm no session ever needed a
    // Start() call — mounting, unmounting, and re-activating tabs must never
    // resurrect an already-running session's shell.
    await page.locator('.tab-item').first().click();

    const counts = await page.evaluate(() => window.__cmdexE2E!.terminalCallCounts);
    expect(counts.Start).toBe(0);
  });

  test('renaming a session: Enter commits the new name', async ({ page, gotoApp }) => {
    await gotoApp();
    const tab = page.locator('.tab-item').first();
    await tab.click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Rename Session' }).click();

    const renameInput = page.locator('.tab-rename-input');
    await expect(renameInput).toBeVisible();
    await renameInput.fill('My Renamed Session');
    await renameInput.press('Enter');

    await expect(tab.locator('.tab-title')).toHaveText('My Renamed Session');
  });

  test('renaming a session: Escape cancels without committing', async ({ page, gotoApp }) => {
    await gotoApp();
    const tab = page.locator('.tab-item').first();
    const originalName = (await tab.locator('.tab-title').textContent())?.trim();

    await tab.click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Rename Session' }).click();
    const renameInput = page.locator('.tab-rename-input');
    await renameInput.fill('Should Not Stick');
    await renameInput.press('Escape');

    await expect(tab.locator('.tab-title')).toHaveText(originalName ?? '');
  });

  test('renaming a session: blur commits the new name', async ({ page, gotoApp }) => {
    await gotoApp();
    const tab = page.locator('.tab-item').first();
    await tab.click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Rename Session' }).click();

    const renameInput = page.locator('.tab-rename-input');
    await renameInput.fill('Committed On Blur');
    // Click elsewhere to blur, rather than pressing Enter/Escape.
    await page.locator('.terminal-pane').click();

    await expect(tab.locator('.tab-title')).toHaveText('Committed On Blur');
  });

  test('the last remaining session cannot be closed, and its close button is hidden', async ({ page, gotoApp }) => {
    await gotoApp();
    await expect(page.locator('.tab-item')).toHaveCount(1);
    await expect(page.locator('.tab-item').first().locator('.tab-close')).toHaveCount(0);
  });

  test('closing a session removes its tab (with more than one open)', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.locator('.tab-new-session-btn').click();
    await expect(page.locator('.tab-item')).toHaveCount(2);

    await page.locator('.tab-item').nth(1).locator('.tab-close').click();
    await expect(page.locator('.tab-item')).toHaveCount(1);
  });

  test('creating an 11th session fails with the max-sessions toast', async ({ page, gotoApp, toast }) => {
    await gotoApp();
    // One session already exists on load; create 9 more to reach 10.
    for (let i = 0; i < 9; i++) {
      await page.locator('.tab-new-session-btn').click();
    }
    await expect(page.locator('.tab-item')).toHaveCount(10);

    await page.locator('.tab-new-session-btn').click();

    await expect(page.locator('.tab-item')).toHaveCount(10);
    await expect(toast(/maximum number of terminal sessions/i)).toBeVisible();
  });

  // xterm.js renders the character grid to a <canvas> (or WebGL layer),
  // which has no text content Playwright can assert against, and this app
  // does not enable xterm's `screenReaderMode` (which would otherwise mirror
  // the buffer into an accessible DOM tree) — enabling it purely for tests
  // would be an a11y-motivated behavior change outside this track's scope.
  // So "did pty-output actually reach xterm" is instead verified indirectly
  // through the app's own "Copy terminal output" button, whose fallback path
  // scrapes xterm's internal buffer (Terminal.tsx's getLastOutput) when the
  // backend capture is unavailable — exactly the mock's default GetLastOutput
  // response. A non-empty scrape (the "Output copied" toast, not "Nothing to
  // copy") is proof the written bytes landed in xterm's buffer.
  test('pty-output renders into xterm — verified via a non-empty copy-last-output scrape', async ({ page, gotoApp, toast }) => {
    await gotoApp();
    await expect(page.getByRole('textbox', { name: 'Terminal input' })).toBeAttached();

    const sessionId = await activeSessionIdFromTab(page);
    // A lone prompt, one line of output, then a second prompt — the scrape
    // heuristic (Terminal.tsx's getLastOutput) needs a prompt-like line
    // before AND after the output to bound the "last command's output".
    await page.evaluate(
      (id) => window.__cmdexE2E!.emitPtyOutput(id, '$ \r\noutput line 1\r\n$ '),
      sessionId,
    );

    await page.locator('.terminal-copy-btn').click();
    await expect(toast(/output copied/i)).toBeVisible();
  });

  test('pty-exit with wasIntentional flips the tab status dot to stopped', async ({ page, gotoApp }) => {
    await gotoApp();
    // Exit the *first* (non-active) session rather than the active one: the
    // active session's own onShellExit callback collapses the whole terminal
    // panel on an intentional exit (App.tsx), which would unmount
    // TerminalTabBar out from under this assertion. Creating a second
    // session makes it the active one and leaves the first alone.
    const firstSessionId = await activeSessionIdFromTab(page);
    await page.locator('.tab-new-session-btn').click();
    await expect(page.locator('.tab-item')).toHaveCount(2);

    // App.tsx's own pty-exit:<id> subscription (distinct from Terminal.tsx's)
    // registers in a useEffect gated on eventsInitialized — wait for it, the
    // same way themes.spec.ts waits before emitting settings-changed.
    await expect
      .poll(() => page.evaluate((id) => window.__cmdexE2E?.hasListener(`pty-exit:${id}`) ?? false, firstSessionId))
      .toBe(true);

    await page.evaluate((id) => window.__cmdexE2E!.emitPtyExit(id, 0, true), firstSessionId);

    await expect(page.locator(`[data-testid="terminal-tab-status-${firstSessionId}"]`)).toHaveClass(/stopped/);
    // The second (active) session is unaffected and the panel stays open.
    await expect(page.locator('.tab-item')).toHaveCount(2);
  });

  test('pty-cleared clears xterm — a prior non-empty copy becomes "nothing to copy"', async ({ page, gotoApp, toast }) => {
    await gotoApp();
    await expect(page.getByRole('textbox', { name: 'Terminal input' })).toBeAttached();
    const sessionId = await activeSessionIdFromTab(page);

    await page.evaluate(
      (id) => window.__cmdexE2E!.emitPtyOutput(id, '$ \r\noutput line 1\r\n$ '),
      sessionId,
    );
    await page.locator('.terminal-copy-btn').click();
    await expect(toast(/output copied/i)).toBeVisible();

    await page.evaluate((id) => window.__cmdexE2E!.emitPtyCleared(id), sessionId);
    await page.locator('.terminal-copy-btn').click();
    await expect(toast(/nothing to copy/i)).toBeVisible();
  });

  test('copy last output: shows "empty" toast when there is nothing to copy', async ({ page, gotoApp, toast }) => {
    await gotoApp();
    await expect(page.getByRole('textbox', { name: 'Terminal input' })).toBeAttached();

    await page.locator('.terminal-copy-btn').click();
    await expect(toast(/nothing to copy/i)).toBeVisible();
  });

  test('collapsing the terminal panel persists to localStorage and can be re-expanded', async ({ page, gotoApp }) => {
    await gotoApp();
    await page.getByRole('button', { name: 'Collapse terminal panel' }).click();

    await expect(page.getByRole('button', { name: 'Expand terminal panel' })).toBeVisible();
    expect(await page.evaluate(() => localStorage.getItem('cmdex-terminal-height-collapsed'))).toBe('true');

    await page.getByRole('button', { name: 'Expand terminal panel' }).click();
    await expect(page.getByRole('button', { name: 'Collapse terminal panel' })).toBeVisible();
    expect(await page.evaluate(() => localStorage.getItem('cmdex-terminal-height-collapsed'))).toBe('false');
  });

  test('resizing the terminal panel persists the new height on mouseup', async ({ page, gotoApp }) => {
    await gotoApp();
    const divider = page.locator('.terminal-divider');
    const box = await divider.boundingBox();
    expect(box).not.toBeNull();
    if (!box) return;

    // Not the horizontal center: the collapse button sits there (and the
    // clear/copy buttons sit at fixed offsets from the right edge), each
    // stopping propagation on mousedown so clicking the divider through them
    // never starts a drag. The far-left edge is always clear of all three.
    const startX = box.x + 10;
    const startY = box.y + box.height / 2;

    await page.mouse.move(startX, startY);
    await page.mouse.down();
    // useResizable attaches its window mousemove/mouseup listeners from a
    // useEffect keyed on isDragging — give React a chance to commit and run
    // that effect before dispatching moves, or they land before anything is
    // listening. `.dragging` is added to the divider in the same render.
    await expect(divider).toHaveClass(/dragging/);
    // Dragging up (direction: -1 on the y axis) increases the terminal's height.
    await page.mouse.move(startX, startY - 60, { steps: 5 });
    await page.mouse.up();

    const stored = await page.evaluate(() => localStorage.getItem('cmdex-terminal-height'));
    expect(stored).not.toBeNull();
    expect(Number(stored)).toBeGreaterThan(0);
  });
});
