import { useEffect, useRef } from 'react';

export { isMac, cmdSymbol, cmdOrCtrl, isCmdOrCtrl, shortcutLabelParts, shortcutLabelString, SHORTCUTS, shortcutLabel } from '@/lib/shortcuts';

type Handler = (e: KeyboardEvent) => void;
export type ShortcutMap = Record<string, Handler | undefined>;

function buildKey(e: KeyboardEvent): string {
  const mods: string[] = [];
  if (e.metaKey) mods.push('meta');
  if (e.ctrlKey) mods.push('ctrl');
  if (e.altKey) mods.push('alt');
  if (e.shiftKey) mods.push('shift');
  mods.push(e.key.toLowerCase());
  return mods.join('+');
}

/**
 * Register global keyboard shortcuts. Pass a stable object (or let the hook
 * track the latest via ref so callers don't need to memoize).
 *
 * Modifier combos fire even when focus is inside an input.
 * Bare keys (e.g. '?') only fire when focus is NOT in an editable element.
 */
export function useKeyboardShortcuts(shortcuts: ShortcutMap): void {
  const ref = useRef<ShortcutMap>(shortcuts);
  useEffect(() => { ref.current = shortcuts; });

  useEffect(() => {
    const handle = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      const isEditing =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      const key = buildKey(e);
      const fn = ref.current[key];
      if (!fn) return;

      const hasModifier = e.metaKey || e.ctrlKey || e.altKey;
      if (hasModifier || !isEditing) {
        e.preventDefault();
        e.stopPropagation();
        fn(e);
      }
    };

    window.addEventListener('keydown', handle, { capture: true });
    return () => window.removeEventListener('keydown', handle, { capture: true });
  }, []);
}
