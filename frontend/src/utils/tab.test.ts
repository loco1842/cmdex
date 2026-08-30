import { describe, it, expect } from 'vitest';
import { isNewCommandTabId, createNewTabId, getCommandDisplayTitle, NEW_TAB_PREFIX } from './tab';
import type { Command } from '../types';

function command(overrides: Partial<Command> = {}): Command {
  return {
    id: 'cmd-1',
    title: { String: '', Valid: false },
    description: { String: '', Valid: false },
    scriptContent: '',
    tags: [],
    variables: [],
    presets: [],
    workingDir: {},
    categoryId: '',
    position: 0,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  };
}

describe('isNewCommandTabId / createNewTabId', () => {
  it('createNewTabId always produces an id isNewCommandTabId accepts', () => {
    expect(isNewCommandTabId(createNewTabId())).toBe(true);
  });

  it('rejects a saved-command-style id (no prefix)', () => {
    expect(isNewCommandTabId('some-uuid-from-the-db')).toBe(false);
  });

  it('rejects the welcome tab id', () => {
    expect(isNewCommandTabId('__welcome__')).toBe(false);
  });

  it('createNewTabId produces distinct ids on each call', () => {
    expect(createNewTabId()).not.toBe(createNewTabId());
  });

  it('NEW_TAB_PREFIX matches what isNewCommandTabId checks for', () => {
    expect(isNewCommandTabId(`${NEW_TAB_PREFIX}anything`)).toBe(true);
  });
});

describe('getCommandDisplayTitle', () => {
  it('returns empty string for a null/undefined command', () => {
    expect(getCommandDisplayTitle(null)).toBe('');
    expect(getCommandDisplayTitle(undefined)).toBe('');
  });

  it('prefers a valid, non-blank title', () => {
    expect(getCommandDisplayTitle(command({ title: { String: '  My Title  ', Valid: true } }))).toBe('My Title');
  });

  it('falls back to the script body when title is invalid', () => {
    expect(getCommandDisplayTitle(command({ scriptContent: 'echo hello' }))).toBe('echo hello');
  });

  it('falls back to the script body when title is valid but blank', () => {
    expect(
      getCommandDisplayTitle(command({ title: { String: '   ', Valid: true }, scriptContent: 'echo hi' })),
    ).toBe('echo hi');
  });

  it('strips a leading #!/bin/bash shebang line before deriving the fallback', () => {
    expect(getCommandDisplayTitle(command({ scriptContent: '#!/bin/bash\necho hello' }))).toBe('echo hello');
  });

  it('strips a bare #!/bin/bash with no trailing newline', () => {
    expect(getCommandDisplayTitle(command({ scriptContent: '#!/bin/bash' }))).toBe('');
  });

  it('collapses newlines to spaces', () => {
    expect(getCommandDisplayTitle(command({ scriptContent: 'line one\nline two' }))).toBe('line one line two');
  });

  it('truncates to 50 chars with an ellipsis for a longer script', () => {
    const long = 'x'.repeat(60);
    const result = getCommandDisplayTitle(command({ scriptContent: long }));
    expect(result).toBe('x'.repeat(50) + '...');
  });

  it('does not truncate a script of exactly 50 chars', () => {
    const exact = 'x'.repeat(50);
    expect(getCommandDisplayTitle(command({ scriptContent: exact }))).toBe(exact);
  });

  it('returns empty string for an empty script and no title', () => {
    expect(getCommandDisplayTitle(command())).toBe('');
  });
});
