import { describe, expect, it } from 'vitest';
import { createLatestAsyncQueue } from './asyncQueue';

describe('createLatestAsyncQueue', () => {
  it('serializes operations and marks only the latest response current', async () => {
    const queue = createLatestAsyncQueue();
    const events: string[] = [];
    let releaseFirst!: () => void;
    const first = queue.enqueue(async () => {
      events.push('first-start');
      await new Promise<void>(resolve => { releaseFirst = resolve; });
      events.push('first-end');
      return 'first';
    });
    const second = queue.enqueue(async () => {
      events.push('second-start');
      return 'second';
    });

    await Promise.resolve();
    expect(events).toEqual(['first-start']);
    releaseFirst();

    await expect(first).resolves.toEqual({ value: 'first', current: false });
    await expect(second).resolves.toEqual({ value: 'second', current: true });
    expect(events).toEqual(['first-start', 'first-end', 'second-start']);
  });

  it('marks a stale failed operation as non-current and continues', async () => {
    const queue = createLatestAsyncQueue();
    const first = queue.enqueue(async () => { throw new Error('failed'); });
    const second = queue.enqueue(async () => 'ok');

    const firstResult = await first;
    expect(firstResult.current).toBe(false);
    expect(firstResult.error).toBeInstanceOf(Error);
    expect((firstResult.error as Error).message).toBe('failed');
    await expect(second).resolves.toEqual({ value: 'ok', current: true });
  });

  it('still rejects a failed operation when it remains current', async () => {
    const queue = createLatestAsyncQueue();
    await expect(queue.enqueue(async () => { throw new Error('failed'); })).rejects.toThrow('failed');
  });
});
