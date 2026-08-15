# Issue #56: ctrl+enter shortcut in spotlight search executes command immediately

**Issue:** https://github.com/loco1842/cmdex/issues/56
**Category:** enhancement · **Priority:** P2 · **Effort:** S · **Status:** planned
**Author:** @loco1842 · **Opened:** 2026-08-15
**Spec:** none — plans directly, doesn't meet the escalation test
**PR:** _(filled in by --fix)_

## Problem

In the command palette (`CommandPalette.tsx`, opened via the spotlight search
shortcut), pressing Enter on a highlighted result only opens it as a tab —
the user still has to press the command's own execute shortcut afterward to
actually run it. The issue asks for a Ctrl+Enter (Cmd+Enter on macOS) in the
palette's search input that runs the selected command immediately, without
the extra step.

## Verification

- **Confirmed still applicable by reading the code:** `CommandPalette.tsx:105-123`
  (`handleKeyDown`) only branches on `ArrowDown` / `ArrowUp` / `Enter` / `Escape`
  — there's no modifier check at all, so Ctrl/Cmd+Enter and plain Enter behave
  identically today (both call `onOpen(cmd)` then `onClose()`, `CommandPalette.tsx:113-118`).
- **Checked whether this already exists elsewhere:** there IS a global
  "execute" shortcut already registered — `shortcuts.ts:52`
  (`execute: { keys: ['cmd', 'enter'] }`), handled in `App.tsx:1294-1308`. But
  its very first line bails whenever an `<input>`/`<textarea>` has focus
  (`App.tsx:1296`), and the palette's search field is exactly that
  (`CommandPalette.tsx:135`, focused whenever the palette is open). So today,
  Ctrl+Enter inside the palette is silently swallowed by neither handler —
  it's a real gap, not a duplicate of existing functionality.
- **Checked for a prior fix:** `git log --oneline -- frontend/src/components/CommandPalette.tsx`
  shows no commit touching keyboard handling in this file since it was added —
  nothing has addressed this.

## Current behavior

- `CommandPalette.tsx:105-123` — `handleKeyDown`, the only keyboard branch
  point in the palette's search `<input>` (wired at `CommandPalette.tsx:141`).
- `CommandPalette.tsx:113-118` — the `Enter` branch: `onOpen(cmd); onClose();`.
  `onOpen` is passed in from `App.tsx:1791` as `handleSelectCommand`
  (`App.tsx:1280-1282`), which just calls `openTab(cmd)`.
- `App.tsx:1294-1308` — the existing global execute shortcut. It only ever
  fires for `selectedCommand` (the currently open/active tab), using
  `resolvedVariables`/`currentResolvedValues`, which are UI state populated
  by the currently-rendered tab's `onResolvedValuesChange` callback
  (`App.tsx:1552`) — i.e. they don't exist yet for a command the user hasn't
  opened. This logic can't be reused as-is for a palette-selected command.
- `execution_service.go:63-90` — `ExecutionService.GetVariables(commandID)`
  returns `[]VariablePrompt` with a server-evaluated `DefaultValue` per
  variable (via `executor.EvalDefaults`, `models.go:125-133`). This is the
  right primitive for the palette case: it resolves CEL defaults (`now()`,
  `env()`, `date()`) without depending on any tab being open or rendered.
- `App.tsx:1027-1049` (`runCommandDirect`) — captures
  `execTabId = activeTabIdRef.current` and uses it to target the executing
  spinner and error toast. `activeTabIdRef` is a `useSyncedRef`
  (`useSyncedRef.ts:8-13`), which syncs `ref.current = value` **during
  render** — not via an effect, and not synchronously mid-handler. So calling
  `openTab(cmd)` (which calls `setActiveTabId(cmd.id)`) and then
  `runCommandDirect`/`handleExecute` in the same synchronous function body
  would read the **stale** `activeTabIdRef.current`, before React has
  re-rendered. See Risks.

## Approach

Add an `onExecute` prop to `CommandPalette`, fired on Ctrl/Cmd+Enter in the
search input (checked via the existing `isCmdOrCtrl` helper from
`shortcuts.ts:7-9`, so it's consistent with every other shortcut in the app).
In `App.tsx`, the new handler:

1. Fetches `GetVariables(cmd.id)` to get server-resolved defaults — sidesteps
   the tab-rendering-dependent `resolvedVariables`/`currentResolvedValues`
   state entirely, which don't exist for a command that isn't open yet.
2. Opens the command's tab (`openTab(cmd)`, same as plain Enter today) so the
   user sees what's running and so `runCommandDirect`'s tab-scoped state
   (executing spinner, error toast) targets the right tab once step 3 fires.
3. **Deferred to the next tick** (`setTimeout(fn, 0)`), execute or prompt:
   - No variables, or every variable has a resolved default → call
     `handleExecute(cmd.id, values)` directly.
   - Any variable is missing a default → call
     `handleFillVariablesByTab(cmd.id, values)` (`App.tsx:1108-1117`, already
     tab-id-parameterized rather than `selectedCommand`-based — exactly
     what's needed here) to prompt for the missing ones, same as the existing
     global shortcut's `hasEmpty` branch.

**Rejected alternative:** executing directly against the terminal without
opening a tab at all (a true "fire and forget"). Rejected because execution
state in this codebase (`executingTabIdRef`, per-tab error toasts) is
inherently tab-scoped — making execution tab-less would mean touching that
shared state model, which is much larger than a P2/S-effort UI shortcut fix
warrants. Opening the tab first is also consistent with what the plain-Enter
behavior already does.

## File map

| File | Change |
|---|---|
| `frontend/src/components/CommandPalette.tsx` | Add `onExecute` prop; branch on modifier in `handleKeyDown`; update footer hint |
| `frontend/src/App.tsx` | Add `handlePaletteExecute`; wire `onExecute` on `<CommandPalette>` |

## Tasks

### Task 1: Add modifier-aware execute handling to CommandPalette

**Files:** modify `frontend/src/components/CommandPalette.tsx`
**Depends on:** none

- [ ] Step 1: extend props and import the platform-aware modifier check

  ```tsx
  import { isCmdOrCtrl } from '../lib/shortcuts';

  interface CommandPaletteProps {
    open: boolean;
    commands: Command[];
    categories: Category[];
    onClose: () => void;
    onOpen: (cmd: Command) => void;
    onExecute: (cmd: Command) => void;
  }
  ```

  Destructure `onExecute` alongside the other props at `CommandPalette.tsx:56-63`.

- [ ] Step 2: branch on the modifier in `handleKeyDown` (`CommandPalette.tsx:105-123`)

  ```tsx
  } else if (e.key === 'Enter') {
    e.preventDefault();
    const cmd = filtered[activeIndex];
    if (!cmd) return;
    if (isCmdOrCtrl(e)) {
      onExecute(cmd);
    } else {
      onOpen(cmd);
    }
    onClose();
  } else if (e.key === 'Escape') {
  ```

  Update the `useCallback` dependency array (`CommandPalette.tsx:123`) to
  include `onExecute`.

- [ ] Step 3: surface the shortcut in the footer hint (`CommandPalette.tsx:202`)

  ```tsx
  <span className="palette-hint"><Kbd>↩</Kbd> open</span>
  <span className="palette-hint"><ShortcutLabel keys={['cmd', 'enter']} /> execute</span>
  ```

  (`ShortcutLabel` is already imported at `CommandPalette.tsx:10`.)

**Acceptance:** pressing Ctrl+Enter (Cmd+Enter on macOS) on a highlighted
palette result calls `onExecute` instead of `onOpen`; plain Enter is
unchanged; the footer shows both hints.
**Verify:** `make check`; manual check in `wails3 dev` — open the palette,
confirm both hints render and the modifier is read correctly on your OS.

### Task 2: Implement execute-immediately in App.tsx

**Files:** modify `frontend/src/App.tsx`
**Depends on:** Task 1

- [ ] Step 1: add `handlePaletteExecute`, placed near `handleSelectCommand`
  (`App.tsx:1280-1282`)

  ```tsx
  const handlePaletteExecute = useCallback(async (cmd: Command) => {
      openTab(cmd);
      const prompts = await GetVariables(cmd.id);
      const values: Record<string, string> = {};
      let hasEmpty = false;
      for (const p of prompts) {
          if (p.defaultValue) {
              values[p.name] = p.defaultValue;
          } else {
              hasEmpty = true;
          }
      }
      // Defer past this render so activeTabIdRef (synced during render,
      // see useSyncedRef.ts) reflects the tab openTab() just activated —
      // otherwise runCommandDirect targets the previously active tab.
      setTimeout(() => {
          if (hasEmpty) {
              handleFillVariablesByTab(cmd.id, values);
          } else {
              handleExecute(cmd.id, values);
          }
      }, 0);
  // eslint-disable-next-line react-hooks/exhaustive-deps -- refs via useSyncedRef are stable
  }, [openTab, handleFillVariablesByTab, handleExecute]);
  ```

- [ ] Step 2: wire the prop on `<CommandPalette>` (`App.tsx:1786-1793`)

  ```tsx
  <CommandPalette
      open={paletteOpen}
      commands={commands}
      categories={categories}
      onClose={() => setPaletteOpen(false)}
      onOpen={handleSelectCommand}
      onExecute={handlePaletteExecute}
  />
  ```

**Acceptance:** Ctrl+Enter on a palette result with no required variables (or
all variables have resolved defaults) opens the tab and runs the command
immediately — spinner and any error toast appear on the correct tab. A
command with an unresolved required variable opens its tab and shows the
fill-variables prompt instead of guessing.
**Verify:** `make check`; manual check in `wails3 dev` — try Ctrl+Enter on
(a) a command with no variables, (b) a command whose variables all have CEL
defaults (e.g. `now()`), and (c) a command with a required variable and no
default, confirming each behaves as described above.

### Checkpoint: after Task 2

- [ ] `make check` passes (`go build ./...` + `pnpm tsc --noEmit`)
- [ ] Manual check in `wails3 dev` covers all three variable scenarios above
- [ ] Plain Enter behavior in the palette is unchanged
- [ ] The existing global `cmdOrCtrl+enter` shortcut for an already-open tab still works unchanged

## Risks

| Risk | Impact | Mitigation |
|---|---|---|
| `activeTabIdRef` staleness — `runCommandDirect` reads the ref before React re-renders past `openTab`'s `setActiveTabId` | Executing spinner/error toast could target the wrong (previously active) tab | Defer the execute/prompt call one tick past `openTab` via `setTimeout(fn, 0)`, as in Task 2 |
| A command with a required variable and no default | "Execute immediately" can't blindly run it | Falls back to opening the tab + the existing fill-variables prompt, same UX as manually opening then hitting the execute shortcut |

## Out of scope

- Not changing the existing global `cmdOrCtrl+enter` shortcut's behavior for an already-open tab (`App.tsx:1294-1308`) — it stays as-is.
- Not changing plain-Enter behavior in the palette.
- Not adding a settings toggle to disable this shortcut.
- Not touching the terminal-execution model (`runCommandDirect`, `TerminalService`) beyond calling it — see issue #57 for the separate, unrelated terminal-setting regression.

## Open questions

- None — the fallback behavior for commands needing variable input follows the existing fill-variables UX, so there's nothing left ambiguous for this scope.
