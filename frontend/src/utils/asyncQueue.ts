export interface AsyncQueueResult<T> {
  value?: T;
  current: boolean;
  error?: unknown;
}

/**
 * Serializes asynchronous settings operations and marks only the newest
 * request as current. This keeps partial SetSettings writes ordered while
 * letting callers ignore stale responses from older requests.
 */
export function createLatestAsyncQueue() {
  let tail: Promise<unknown> = Promise.resolve();
  let generation = 0;

  return {
    invalidate() {
      generation += 1;
      return generation;
    },

    isCurrent(request: number) {
      return request === generation;
    },

    enqueue<T>(operation: () => Promise<T>): Promise<AsyncQueueResult<T>> {
      const request = ++generation;
      const result = tail.then(operation, operation);
      tail = result.then(() => undefined, () => undefined);
      return result.then(
        value => ({ value, current: request === generation }),
        error => {
          // A request that was superseded while running must not turn into a
          // user-visible failure. Return the same result shape as successful
          // stale work so callers can consistently check `current`.
          if (request !== generation) {
            return { current: false, error };
          }
          throw error;
        },
      );
    },
  };
}
