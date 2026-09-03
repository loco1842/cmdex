import { describe, expect, it, vi } from 'vitest';

import { applyTheme, CUSTOM_THEME_VAR_KEYS, parseCustomThemes } from './theme-apply';

function colors() {
  return Object.fromEntries(CUSTOM_THEME_VAR_KEYS.map(key => [key, '#000'])) as Record<string, string>;
}

function theme(overrides: Record<string, unknown> = {}) {
  return {
    id: 'custom-test',
    name: 'Test theme',
    type: 'dark',
    colors: colors(),
    ...overrides,
  };
}

describe('parseCustomThemes', () => {
  it('keeps complete custom color maps', () => {
    expect(parseCustomThemes(JSON.stringify([theme()]))).toHaveLength(1);
  });

  it('accepts partial custom color maps', () => {
    const incomplete = colors();
    delete incomplete.background;

    expect(parseCustomThemes(JSON.stringify([theme({ colors: incomplete })]))).toHaveLength(1);
  });

  it('rejects custom color maps with non-string provided values', () => {
    const invalidValue = { ...colors(), foreground: null };

    expect(parseCustomThemes(JSON.stringify([theme({ colors: invalidValue })]))).toEqual([]);
  });

  it('discards malformed entries without rejecting valid themes', () => {
    const valid = theme({ colors: { background: '#101014' } });

    expect(parseCustomThemes(JSON.stringify([valid, null, { id: 'missing-fields' }]))).toEqual([valid]);
  });
});

describe('applyTheme', () => {
  it('tolerates legacy null custom colors', () => {
    const setAttribute = vi.fn();
    const removeProperty = vi.fn();
    const setProperty = vi.fn();
    vi.stubGlobal('document', {
      documentElement: {
        setAttribute,
        style: { removeProperty, setProperty },
      },
    });

    try {
      expect(() => applyTheme('dark', null)).not.toThrow();
      expect(setAttribute).toHaveBeenCalledWith('data-theme', 'dark');
      expect(removeProperty).toHaveBeenCalledTimes(CUSTOM_THEME_VAR_KEYS.length);
      expect(setProperty).not.toHaveBeenCalled();
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
