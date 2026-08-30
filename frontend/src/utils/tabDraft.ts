import type { Command, TabDraft } from '../types';
import { mergeDetectedVariables, normalizeVariablesForCompare } from './templateVars';

export function emptyDraft(defaultCategoryId?: string): TabDraft {
  return {
    title: '',
    description: '',
    tags: [],
    categoryId: defaultCategoryId ?? '',
    scriptBody: '',
    variables: [],
    workingDir: {},
    revealed: { title: false, description: false, tags: false },
  };
}

export function draftFromCommand(cmd: Command, scriptBody: string): TabDraft {
  const variables = mergeDetectedVariables(scriptBody, cmd.variables);
  return {
    title: cmd.title?.Valid ? cmd.title.String : '',
    description: cmd.description?.Valid ? cmd.description.String : '',
    tags: [...cmd.tags],
    categoryId: cmd.categoryId,
    scriptBody,
    variables,
    workingDir: { ...(cmd.workingDir || {}) },
    revealed: {
      title: !!(cmd.title?.Valid && cmd.title.String.trim()),
      description: !!(cmd.description?.Valid && cmd.description.String.trim()),
      tags: (cmd.tags?.length ?? 0) > 0,
    },
  };
}

function osPathMapEqual(a: Record<string, string>, b: Record<string, string>): boolean {
  const aKeys = Object.keys(a).sort();
  const bKeys = Object.keys(b).sort();
  if (aKeys.length !== bKeys.length) return false;
  return aKeys.every(k => a[k] === b[k]);
}

/** Shallow equality for flat objects with primitive (string/boolean) values. */
function shallowEqual<T extends object>(a: T, b: T): boolean {
  const aKeys = Object.keys(a) as (keyof T)[];
  const bKeys = Object.keys(b) as (keyof T)[];
  if (aKeys.length !== bKeys.length) return false;
  return aKeys.every((k) => a[k] === b[k]);
}

/** Array equality using a per-item equality function, order-sensitive. */
function arraysEqual<T>(a: T[], b: T[], itemEqual: (x: T, y: T) => boolean): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (!itemEqual(a[i], b[i])) return false;
  }
  return true;
}

export function draftsEqual(a: TabDraft, b: TabDraft): boolean {
  if (
    a.title !== b.title ||
    a.description !== b.description ||
    a.categoryId !== b.categoryId ||
    a.scriptBody !== b.scriptBody
  ) {
    return false;
  }
  if (!arraysEqual(a.tags, b.tags, (x, y) => x === y)) return false;
  if (!shallowEqual(a.revealed, b.revealed)) return false;
  if (!osPathMapEqual(a.workingDir, b.workingDir)) return false;
  if (
    !arraysEqual(
      normalizeVariablesForCompare(a.variables),
      normalizeVariablesForCompare(b.variables),
      shallowEqual,
    )
  ) {
    return false;
  }
  return true;
}

export function cloneDraft(d: TabDraft): TabDraft {
  return JSON.parse(JSON.stringify(d)) as TabDraft;
}

export function makePlaceholderCommand(id: string, categoryId?: string): Command {
  return {
    id,
    title: { String: '', Valid: false },
    description: { String: '', Valid: false },
    scriptContent: '',
    tags: [],
    variables: [],
    presets: [],
    workingDir: {},
    categoryId: categoryId ?? '',
    position: 0,
    createdAt: '',
    updatedAt: '',
  };
}
