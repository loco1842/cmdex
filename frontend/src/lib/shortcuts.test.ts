import { describe, it, expect, afterEach, vi } from 'vitest';

// `isMac` (shortcuts.ts:1-2) is computed once at module load from
// navigator.userAgent, so each platform variant needs a fresh module
// instance with a stubbed navigator — hence vi.resetModules() + a dynamic
// import per case rather than a single top-level import.
async function loadShortcuts(userAgent: string) {
  vi.resetModules();
  vi.stubGlobal('navigator', { userAgent });
  return import('./shortcuts');
}

afterEach(() => {
  vi.unstubAllGlobals();
});

const MAC_UA = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)';
const WINDOWS_UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)';

describe('shortcutLabelParts / shortcutLabelString', () => {
  it('renders Mac-style symbols and space-joins on a Mac user agent', async () => {
    const { shortcutLabelParts, shortcutLabelString } = await loadShortcuts(MAC_UA);
    expect(shortcutLabelParts(['cmd', 'shift', 'backspace'])).toEqual(['⌘', '⇧', '⌫']);
    expect(shortcutLabelString(['cmd', 's'])).toBe('⌘ S');
  });

  it('renders "Ctrl" and plus-joins on a non-Mac user agent', async () => {
    const { shortcutLabelParts, shortcutLabelString } = await loadShortcuts(WINDOWS_UA);
    expect(shortcutLabelParts(['cmd', 's'])).toEqual(['Ctrl', 'S']);
    expect(shortcutLabelString(['cmd', 's'])).toBe('Ctrl+S');
  });

  it('distinguishes the physical ctrl key (^) from cmd/ctrl-or-meta on Mac', async () => {
    const { shortcutLabelParts } = await loadShortcuts(MAC_UA);
    expect(shortcutLabelParts(['ctrl'])).toEqual(['^']);
  });

  it('uppercases a single unrecognized character', async () => {
    const { shortcutLabelParts } = await loadShortcuts(WINDOWS_UA);
    expect(shortcutLabelParts(['p'])).toEqual(['P']);
  });

  it('passes an unrecognized multi-character token through unchanged', async () => {
    const { shortcutLabelParts } = await loadShortcuts(WINDOWS_UA);
    expect(shortcutLabelParts(['f1'])).toEqual(['f1']);
  });
});

describe('isCmdOrCtrl', () => {
  it('checks metaKey on Mac', async () => {
    const { isCmdOrCtrl } = await loadShortcuts(MAC_UA);
    expect(isCmdOrCtrl({ metaKey: true, ctrlKey: false } as KeyboardEvent)).toBe(true);
    expect(isCmdOrCtrl({ metaKey: false, ctrlKey: true } as KeyboardEvent)).toBe(false);
  });

  it('checks ctrlKey on non-Mac', async () => {
    const { isCmdOrCtrl } = await loadShortcuts(WINDOWS_UA);
    expect(isCmdOrCtrl({ metaKey: true, ctrlKey: false } as KeyboardEvent)).toBe(false);
    expect(isCmdOrCtrl({ metaKey: false, ctrlKey: true } as KeyboardEvent)).toBe(true);
  });
});

describe('shortcutLabel', () => {
  it('renders a registered shortcut id using the platform-appropriate labels', async () => {
    const { shortcutLabel } = await loadShortcuts(MAC_UA);
    expect(shortcutLabel('save')).toBe('⌘ S');
  });

  it('renders the same shortcut id differently on non-Mac', async () => {
    const { shortcutLabel } = await loadShortcuts(WINDOWS_UA);
    expect(shortcutLabel('save')).toBe('Ctrl+S');
  });
});
