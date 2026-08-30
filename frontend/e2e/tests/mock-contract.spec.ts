import { test, expect } from '../fixtures';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Guards against the documented silent-failure trap: `e2e/mocks/runtime.ts`
// dispatches by numeric Wails `$Call.ByID` hash. Regenerating bindings can
// shift those hashes, and a mismatch used to fail silently — a spec would
// just see `[e2e mock] no handler for method ID …` in the console and a
// resolved `null`, not a test failure. This is a pure Node/fs check (no
// browser needed) that fails loudly instead.

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const bindingsDir = path.resolve(__dirname, '../../bindings/cmdex');
const runtimePath = path.resolve(__dirname, '../mocks/runtime.ts');

function extractBindingIds(): Map<number, string> {
  const ids = new Map<number, string>();
  for (const file of fs.readdirSync(bindingsDir)) {
    if (!file.endsWith('.js')) continue;
    const src = fs.readFileSync(path.join(bindingsDir, file), 'utf8');
    // export function MethodName(...) { ... $Call.ByID(12345 ...
    const re = /export function (\w+)\([^)]*\)\s*\{[\s\S]{0,400}?\$Call\.ByID\((\d+)/g;
    let m: RegExpExecArray | null;
    while ((m = re.exec(src))) {
      ids.set(Number(m[2]), m[1]);
    }
  }
  return ids;
}

function extractMockIds(): Map<number, string> {
  const src = fs.readFileSync(runtimePath, 'utf8');
  const tableMatch = src.match(/const METHOD_IDS = \{([\s\S]*?)\} as const;/);
  if (!tableMatch) throw new Error('Could not find METHOD_IDS table in runtime.ts');
  const ids = new Map<number, string>();
  const re = /(\w+):\s*(\d+),/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(tableMatch[1]))) {
    ids.set(Number(m[2]), m[1]);
  }
  return ids;
}

test.describe('e2e mock contract', () => {
  test('every generated binding method ID has a mock handler, and vice versa', () => {
    const bindingIds = extractBindingIds();
    const mockIds = extractMockIds();

    expect(bindingIds.size).toBeGreaterThan(0);
    expect(mockIds.size).toBeGreaterThan(0);

    const missingFromMock: string[] = [];
    for (const [id, name] of bindingIds) {
      if (!mockIds.has(id)) missingFromMock.push(`${id} (${name})`);
    }

    const staleInMock: string[] = [];
    for (const [id, name] of mockIds) {
      if (!bindingIds.has(id)) staleInMock.push(`${id} (${name})`);
    }

    expect(missingFromMock, 'bindings define methods the mock has no handler for').toEqual([]);
    expect(staleInMock, 'mock has handlers for method IDs no longer in the generated bindings').toEqual([]);
  });

  test('every mock method name matches its binding method name for the same ID', () => {
    const bindingIds = extractBindingIds();
    const mockIds = extractMockIds();

    const mismatches: string[] = [];
    for (const [id, bindingName] of bindingIds) {
      const mockName = mockIds.get(id);
      if (mockName && mockName !== bindingName) {
        mismatches.push(`ID ${id}: binding=${bindingName} mock=${mockName}`);
      }
    }
    expect(mismatches).toEqual([]);
  });
});
