import { test, expect } from '@playwright/test';
import '../utils/types';

// Regression coverage for the double-start bug: Terminal.tsx used to
// unconditionally call Start() on every mount, even when the backend
// (TerminalService.ServiceStartup / CreateSession) had already started the
// session's shell — tearing down the healthy PTY and spawning a redundant
// second shell. The fix threads the backend's already-known `running` state
// into Terminal.tsx as `initiallyRunning` and skips the mount-time Start()
// call when it's already true. The mock backend mirrors the real one:
// CreateSession's returned SessionInfo always has running: true.

test.describe('Terminal', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      window.__cmdexE2E_SEED__ = { settings: {} };
    });
  });

  test('does not call Start for the session the backend already started on first load', async ({ page }) => {
    await page.goto('/');

    // The app auto-creates a default session on first load (no sessions,
    // no active session) — wait for its tab to render.
    await expect(page.locator('.tab-item')).toHaveCount(1, { timeout: 5000 });

    const counts = await page.evaluate(() => window.__cmdexE2E!.terminalCallCounts);
    expect(counts.CreateSession).toBe(1);
    expect(counts.Start).toBe(0);
  });

  test('creating a new terminal tab does not call Start either', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.tab-item')).toHaveCount(1, { timeout: 5000 });

    await page.locator('.tab-new-session-btn').click();
    await expect(page.locator('.tab-item')).toHaveCount(2, { timeout: 5000 });

    const counts = await page.evaluate(() => window.__cmdexE2E!.terminalCallCounts);
    expect(counts.CreateSession).toBe(2);
    expect(counts.Start).toBe(0);
  });

  test('every session tab shows as running, never stuck in a stopped/restarting state', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.tab-item')).toHaveCount(1, { timeout: 5000 });

    // The status dot reflects SessionInfo.running from the backend — since
    // the mock (like the real backend) always starts a session before
    // returning it, this must read "running" immediately, not "stopped".
    await expect(page.locator('.tab-status-dot')).toHaveClass(/running/);
  });

  test('clicking the clear button calls Clear for the active session', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.tab-item')).toHaveCount(1, { timeout: 5000 });

    // The tab list (TerminalTabBar) and the xterm.js-backed TerminalComponent
    // mount independently — waiting only for the tab risks clicking Clear
    // before terminalRefs.current[activeSessionId] is populated, in which
    // case the click silently no-ops. xterm's own "Terminal input" textarea
    // appearing is a reliable signal that the component has finished mounting.
    await expect(page.getByRole('textbox', { name: 'Terminal input' })).toBeAttached({ timeout: 5000 });

    await page.locator('.terminal-clear-btn').click();

    await expect
      .poll(async () => (await page.evaluate(() => window.__cmdexE2E!.terminalCallCounts)).Clear)
      .toBe(1);
  });

  test('switching the active tab keeps the previous session mounted (Start still never called)', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.tab-item')).toHaveCount(1, { timeout: 5000 });

    await page.locator('.tab-new-session-btn').click();
    await expect(page.locator('.tab-item')).toHaveCount(2, { timeout: 5000 });

    // Switch back to the first tab and confirm no session ever needed a
    // Start() call — mounting, unmounting, and re-activating tabs must never
    // resurrect an already-running session's shell.
    await page.locator('.tab-item').first().click();

    const counts = await page.evaluate(() => window.__cmdexE2E!.terminalCallCounts);
    expect(counts.Start).toBe(0);
  });
});
