import { afterEach, describe, expect, it } from 'vitest';

import { eventNames, mergeEventNames } from './events';

const fallbackNames = { ...eventNames };

afterEach(() => {
  Object.assign(eventNames, fallbackNames);
});

describe('mergeEventNames', () => {
  it('keeps fallback names for partial or malformed payloads', () => {
    mergeEventNames({ launcherShown: undefined, launcherHidden: '', dataReset: null });

    expect(eventNames.launcherShown).toBe(fallbackNames.launcherShown);
    expect(eventNames.launcherHidden).toBe(fallbackNames.launcherHidden);
    expect(eventNames.dataReset).toBe(fallbackNames.dataReset);
  });

  it('accepts non-empty backend names', () => {
    mergeEventNames({ launcherShown: 'custom-launcher-shown' });
    expect(eventNames.launcherShown).toBe('custom-launcher-shown');
  });
});
