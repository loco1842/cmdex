import { describe, expect, it } from 'vitest';

import {
  applyRefreshedPresets,
  createLauncherResizeQueue,
  clearPendingShortcutIfCurrent,
  isCurrentLauncherPresetRefresh,
  isCurrentLauncherRequest,
  transitionLauncherQuery,
} from './launcher';
import { type Command } from '../types';

function command(id: string): Command {
  return {
    id,
    title: { String: id, Valid: true },
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
  };
}

describe('transitionLauncherQuery', () => {
  it('returns to search and clears execution state when typing over output', () => {
    expect(transitionLauncherQuery('running', 'next command')).toEqual({
      query: 'next command',
      stage: 'search',
      clearRunState: true,
    });
  });

  it('keeps variable prompts and search navigation intact while typing', () => {
    expect(transitionLauncherQuery('variables', 'typed value')).toEqual({
      query: 'typed value',
      stage: 'variables',
      clearRunState: false,
    });
    expect(transitionLauncherQuery('search', 'typed value')).toEqual({
      query: 'typed value',
      stage: 'search',
      clearRunState: false,
    });
  });
});

describe('applyRefreshedPresets', () => {
  const commandA = command('A');
  const presetA = [{ id: 'preset-a', name: 'A', position: 0, values: {} }];

  it('does not restore a canceled command from a stale refresh', () => {
    expect(applyRefreshedPresets(null, commandA, presetA)).toBeNull();
  });

  it('does not replace command B with a stale refresh for command A', () => {
    const commandB = command('B');
    expect(applyRefreshedPresets(commandB, commandA, presetA)).toBe(commandB);
  });

  it('refreshes presets when the pending command is still the same command', () => {
    expect(applyRefreshedPresets(commandA, commandA, presetA)).toEqual({
      ...commandA,
      presets: presetA,
    });
  });
});

describe('launcher async request guards', () => {
  it('rejects a launcher data response once a newer request starts', () => {
    expect(isCurrentLauncherRequest(1, 2)).toBe(false);
    expect(isCurrentLauncherRequest(2, 2)).toBe(true);
  });

  it('requires both the refresh and prompt activation to remain current', () => {
    expect(isCurrentLauncherPresetRefresh(2, 2, 7, 7)).toBe(true);
    // Same command reopened: the ID can still match, but its old activation
    // must not be allowed to update the new prompt.
    expect(isCurrentLauncherPresetRefresh(2, 2, 6, 7)).toBe(false);
    // Same prompt with overlapping refreshes: only the newest generation may
    // update its preset list.
    expect(isCurrentLauncherPresetRefresh(1, 2, 7, 7)).toBe(false);
  });

  it('does not clear a newer shortcut capture when an older save settles', () => {
    expect(clearPendingShortcutIfCurrent('Ctrl+K', 'Ctrl+K')).toBe('');
    expect(clearPendingShortcutIfCurrent('Ctrl+L', 'Ctrl+K')).toBe('Ctrl+L');
  });

  it('applies resize requests in order so a reset wins over an older resize', async () => {
    const calls: boolean[] = [];
    let releaseResize!: () => void;
    const queue = createLauncherResizeQueue(async (expanded) => {
      calls.push(expanded);
      if (expanded) await new Promise<void>(resolve => { releaseResize = resolve; });
    });

    const expand = queue.enqueue(true);
    await new Promise<void>(resolve => setTimeout(resolve, 0));
    const reset = queue.enqueue(false);
    expect(calls).toEqual([true]);

    releaseResize();
    await expand;
    await reset;
    expect(calls).toEqual([true, false]);
  });

  it('coalesces resize requests that have not started yet', async () => {
    const calls: boolean[] = [];
    const queue = createLauncherResizeQueue(async (expanded) => {
      calls.push(expanded);
    });

    await Promise.all([queue.enqueue(true), queue.enqueue(false), queue.enqueue(true)]);
    expect(calls).toEqual([true]);
  });
});
