import { describe, it, expect } from 'vitest';
import { normalizeOS, getOSPath, setOSPath, shortenPath } from './path';

describe('normalizeOS', () => {
  it('passes through known OS strings', () => {
    expect(normalizeOS('darwin')).toBe('darwin');
    expect(normalizeOS('linux')).toBe('linux');
    expect(normalizeOS('windows')).toBe('windows');
  });

  it('maps anything else to unknown', () => {
    expect(normalizeOS('freebsd')).toBe('unknown');
    expect(normalizeOS('')).toBe('unknown');
  });
});

describe('getOSPath', () => {
  it('returns the path for the given OS', () => {
    expect(getOSPath({ darwin: '/a' }, 'darwin')).toBe('/a');
  });

  it('returns empty string when the OS key is absent', () => {
    expect(getOSPath({ darwin: '/a' }, 'linux')).toBe('');
  });

  it('returns empty string for an undefined map', () => {
    expect(getOSPath(undefined, 'darwin')).toBe('');
  });

  it('always returns empty string for the unknown OS, regardless of map contents', () => {
    expect(getOSPath({ unknown: '/should-not-be-read' }, 'unknown')).toBe('');
  });
});

describe('setOSPath', () => {
  it('sets a new path for the given OS without mutating the input map', () => {
    const original = { darwin: '/a' };
    const result = setOSPath(original, 'linux', '/b');
    expect(result).toEqual({ darwin: '/a', linux: '/b' });
    expect(original).toEqual({ darwin: '/a' });
  });

  it('deletes the key entirely when path is empty, rather than storing an empty string', () => {
    const result = setOSPath({ darwin: '/a' }, 'darwin', '');
    expect(result).toEqual({});
    expect('darwin' in result).toBe(false);
  });

  it('is a no-op for the unknown OS', () => {
    expect(setOSPath({ darwin: '/a' }, 'unknown', '/whatever')).toEqual({ darwin: '/a' });
  });

  it('handles an undefined input map', () => {
    expect(setOSPath(undefined, 'darwin', '/a')).toEqual({ darwin: '/a' });
  });
});

describe('shortenPath', () => {
  it('returns empty string for an empty path', () => {
    expect(shortenPath('')).toBe('');
  });

  it('returns the path unchanged when it has fewer segments than requested', () => {
    expect(shortenPath('/a', 2)).toBe('/a');
  });

  it('keeps only the last N segments, prefixed with an ellipsis', () => {
    expect(shortenPath('/a/b/c/d', 2)).toBe('.../c/d');
  });

  it('uses backslash as the separator for a Windows-style path', () => {
    expect(shortenPath('C:\\Users\\me\\projects\\app', 2)).toBe('...\\projects\\app');
  });
});
