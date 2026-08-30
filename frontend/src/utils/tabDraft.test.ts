import { describe, it, expect } from 'vitest';
import { draftsEqual, cloneDraft, draftFromCommand, emptyDraft, makePlaceholderCommand } from './tabDraft';
import type { Command, TabDraft, VariableDefinition } from '../types';

function variable(overrides: Partial<VariableDefinition> = {}): VariableDefinition {
  return { name: 'foo', description: '', example: '', default: '', sortOrder: 0, ...overrides };
}

describe('draftsEqual', () => {
  it('treats two freshly built empty drafts as equal', () => {
    expect(draftsEqual(emptyDraft(), emptyDraft())).toBe(true);
  });

  it('detects a scalar field change', () => {
    const a = emptyDraft();
    const b = { ...emptyDraft(), title: 'changed' };
    expect(draftsEqual(a, b)).toBe(false);
  });

  it('is order-sensitive on tags', () => {
    const a: TabDraft = { ...emptyDraft(), tags: ['a', 'b'] };
    const b: TabDraft = { ...emptyDraft(), tags: ['b', 'a'] };
    expect(draftsEqual(a, b)).toBe(false);
  });

  it('treats identical tags in the same order as equal', () => {
    const a: TabDraft = { ...emptyDraft(), tags: ['a', 'b'] };
    const b: TabDraft = { ...emptyDraft(), tags: ['a', 'b'] };
    expect(draftsEqual(a, b)).toBe(true);
  });

  it('includes the revealed flags — toggling one alone makes drafts unequal', () => {
    // Regression guard: CommandDetail.tsx reveals the description field with
    // no text typed, and that alone must register as dirty (tabDraft.ts:69).
    const a = emptyDraft();
    const b: TabDraft = { ...emptyDraft(), revealed: { ...emptyDraft().revealed, description: true } };
    expect(draftsEqual(a, b)).toBe(false);
  });

  it('compares workingDir by key regardless of insertion order', () => {
    const a: TabDraft = { ...emptyDraft(), workingDir: { darwin: '/a', linux: '/b' } };
    const b: TabDraft = { ...emptyDraft(), workingDir: { linux: '/b', darwin: '/a' } };
    expect(draftsEqual(a, b)).toBe(true);
  });

  it('detects a workingDir value change', () => {
    const a: TabDraft = { ...emptyDraft(), workingDir: { darwin: '/a' } };
    const b: TabDraft = { ...emptyDraft(), workingDir: { darwin: '/different' } };
    expect(draftsEqual(a, b)).toBe(false);
  });

  it('detects a workingDir key added/removed', () => {
    const a: TabDraft = { ...emptyDraft(), workingDir: { darwin: '/a' } };
    const b: TabDraft = { ...emptyDraft(), workingDir: { darwin: '/a', linux: '/b' } };
    expect(draftsEqual(a, b)).toBe(false);
  });

  it('compares variables field-by-field, ignoring sortOrder differences from other fields', () => {
    const a: TabDraft = { ...emptyDraft(), variables: [variable({ name: 'x', default: '1' })] };
    const b: TabDraft = { ...emptyDraft(), variables: [variable({ name: 'x', default: '1' })] };
    expect(draftsEqual(a, b)).toBe(true);
  });

  it('detects a variable default value change', () => {
    const a: TabDraft = { ...emptyDraft(), variables: [variable({ default: '1' })] };
    const b: TabDraft = { ...emptyDraft(), variables: [variable({ default: '2' })] };
    expect(draftsEqual(a, b)).toBe(false);
  });

  it('detects a variable count change', () => {
    const a: TabDraft = { ...emptyDraft(), variables: [variable({ name: 'x' })] };
    const b: TabDraft = { ...emptyDraft(), variables: [variable({ name: 'x' }), variable({ name: 'y' })] };
    expect(draftsEqual(a, b)).toBe(false);
  });
});

describe('cloneDraft', () => {
  it('produces a deep copy that does not alias the original', () => {
    const original: TabDraft = { ...emptyDraft(), tags: ['a'], workingDir: { darwin: '/a' } };
    const clone = cloneDraft(original);
    clone.tags.push('b');
    clone.workingDir.darwin = '/changed';
    expect(original.tags).toEqual(['a']);
    expect(original.workingDir.darwin).toBe('/a');
  });

  it('round-trips to an equal draft', () => {
    const original: TabDraft = { ...emptyDraft(), title: 'x', tags: ['a', 'b'] };
    expect(draftsEqual(original, cloneDraft(original))).toBe(true);
  });
});

describe('draftFromCommand', () => {
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

  it('derives revealed flags from non-empty saved values', () => {
    const cmd = command({
      title: { String: 'My Title', Valid: true },
      description: { String: '  ', Valid: true }, // whitespace-only
      tags: ['a'],
    });
    const draft = draftFromCommand(cmd, cmd.scriptContent);
    expect(draft.revealed).toEqual({ title: true, description: false, tags: true });
  });

  it('leaves revealed false when title/description are invalid or blank', () => {
    const cmd = command();
    const draft = draftFromCommand(cmd, '');
    expect(draft.revealed).toEqual({ title: false, description: false, tags: false });
  });

  it('does not alias the command tags array', () => {
    const cmd = command({ tags: ['a', 'b'] });
    const draft = draftFromCommand(cmd, '');
    draft.tags.push('c');
    expect(cmd.tags).toEqual(['a', 'b']);
  });
});

describe('makePlaceholderCommand', () => {
  it('produces an invalid/empty command carrying only id and categoryId', () => {
    const placeholder = makePlaceholderCommand('__new_abc', 'cat-1');
    expect(placeholder.id).toBe('__new_abc');
    expect(placeholder.categoryId).toBe('cat-1');
    expect(placeholder.title.Valid).toBe(false);
    expect(placeholder.variables).toEqual([]);
  });

  it('defaults categoryId to empty string when omitted', () => {
    expect(makePlaceholderCommand('__new_abc').categoryId).toBe('');
  });
});
