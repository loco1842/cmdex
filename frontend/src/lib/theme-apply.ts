import type { CustomTheme } from '../types';

export const CUSTOM_THEME_VAR_KEYS = [
  'background', 'foreground', 'card', 'card-foreground', 'popover', 'popover-foreground',
  'primary', 'primary-foreground', 'secondary', 'secondary-foreground', 'muted', 'muted-foreground',
  'accent', 'accent-foreground', 'destructive', 'destructive-foreground', 'success', 'success-foreground',
  'border', 'input', 'ring', 'tab-bar-bg', 'tab-active-bg', 'tab-inactive-bg',
  'tab-active-indicator', 'status-bar-bg', 'status-bar-fg',
];

export function applyTheme(themeId: string, customColors?: Record<string, string> | null) {
  document.documentElement.setAttribute('data-theme', themeId);
  // Always clear first: a custom palette may omit keys the previous one set,
  // and those would otherwise stay behind and mix two themes together.
  CUSTOM_THEME_VAR_KEYS.forEach((key) => {
    document.documentElement.style.removeProperty(`--${key}`);
  });
  Object.entries(customColors ?? {}).forEach(([key, value]) => {
    document.documentElement.style.setProperty(`--${key}`, value);
  });
}

/**
 * Whether a parsed entry is actually shaped like a `CustomTheme`.
 *
 * The stored blob is untrusted: it survives app upgrades and can be hand-edited,
 * so entries like `null` would otherwise reach callers that dereference `.id` or
 * feed `.colors` to `setProperty`.
 */
function isCustomTheme(value: unknown): value is CustomTheme {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const theme = value as Record<string, unknown>;
  if (
    typeof theme.id !== 'string' ||
    typeof theme.name !== 'string' ||
    (theme.type !== 'dark' && theme.type !== 'light') ||
    !theme.colors ||
    typeof theme.colors !== 'object' ||
    Array.isArray(theme.colors)
  ) {
    return false;
  }
  const colors = theme.colors as Record<string, unknown>;
  return Object.values(colors).every((c) => typeof c === 'string');
}

/**
 * Parse the stored `customThemes` JSON blob.
 *
 * Settings hold this as a string, and a malformed value must not break theming —
 * every caller gets a list of well-formed themes, or an empty one.
 */
export function parseCustomThemes(raw?: string): CustomTheme[] {
  if (!raw || raw === '[]') return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!Array.isArray(parsed)) return [];
  const themes = parsed.filter(isCustomTheme);
  if (themes.length !== parsed.length) {
    console.warn(`theme-apply: discarded ${parsed.length - themes.length} malformed custom theme(s)`);
  }
  return themes;
}

/** The stored custom theme matching `themeId`, if the active theme is a custom one. */
export function resolveActiveCustomTheme(raw: string | undefined, themeId: string): CustomTheme | undefined {
  return parseCustomThemes(raw).find((c) => c.id === themeId);
}

export function applyDensity(density: string) {
  document.documentElement.setAttribute('data-density', density);
}

export function applyFonts(uiFont: string, monoFont: string) {
  const fontValue = uiFont === 'System Default'
    ? 'system-ui, -apple-system, sans-serif'
    : `'${uiFont}', system-ui, sans-serif`;
  document.documentElement.style.setProperty('--font-sans', fontValue);
  document.documentElement.style.setProperty('--font-mono', "'" + monoFont + "', monospace");
}
