import { type Command } from '../types';
import { getCommandDisplayTitle } from './tab';

/**
 * Shared command filtering for the in-app command palette (Cmd+P) and the
 * global quick launcher, so both rank and match identically.
 *
 * A query starting with '#' filters by tag; anything else matches against the
 * display title, description and tags.
 */
export function matchesCommand(query: string, cmd: Command): boolean {
  const q = query.toLowerCase();
  if (q.startsWith('#')) {
    const tagQuery = q.slice(1);
    if (!tagQuery) return (cmd.tags || []).length > 0;
    return (cmd.tags || []).some((t) => t.toLowerCase().includes(tagQuery));
  }
  const displayTitle = getCommandDisplayTitle(cmd).toLowerCase();
  const desc = cmd.description?.Valid ? cmd.description.String : '';
  return (
    displayTitle.includes(q) ||
    desc.toLowerCase().includes(q) ||
    (cmd.tags || []).some((t) => t.toLowerCase().includes(q))
  );
}

/** Filter a command list by query, capped at `limit` results. */
export function filterCommands(query: string, commands: Command[], limit = 30): Command[] {
  const q = query.trim();
  const list = q ? commands.filter((c) => matchesCommand(q, c)) : commands;
  return list.slice(0, limit);
}

/** First non-empty script line, shebang stripped, truncated for preview rows. */
export function scriptSnippet(content: string, maxLength = 60): string {
  const body = content.replace(/^#!.*\n?/, '').trim();
  const firstLine = body.split('\n').find((l) => l.trim()) || '';
  return firstLine.length > maxLength ? firstLine.slice(0, maxLength - 3) + '…' : firstLine;
}
