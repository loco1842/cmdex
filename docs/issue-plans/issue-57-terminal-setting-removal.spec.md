# Spec: Remove the dead terminal-choice setting (Issue #57)

**Issue:** https://github.com/loco1842/cmdex/issues/57
**Decided:** 2026-08-15, by the user, via the github-issue-planner escalation gate

## Why this needed a spec, not just a plan

Issue #57's title/body gave a symptom ("terminal setting no longer seems to
be used") but not a desired fix. Two materially different, equally valid
responses existed:

1. Remove the setting, since nothing reads it anymore.
2. Restore the functionality the setting was meant to control (an explicit
   "open in external terminal" action).

These aren't a refactor of the same idea — they're opposite product
directions (deprecate a capability vs. re-invest in it), so this needed a
human decision before any code got planned. **Decided: remove the setting.**

## Objective

`AppSettings.Terminal` and everything downstream of it (`SettingsPage.tsx`'s
terminal dropdown, `SettingsService.GetAvailableTerminals`,
`Executor.GetAvailableTerminals`/`OpenInTerminal`, and the platform-specific
terminal-detection code that backs them) should be deleted, since nothing
has consulted this setting since commit `edb16c4` (~2026-06-18, milestone
v2.1) replaced external-terminal execution with the embedded PTY panel. The
Settings UI should stop offering a choice that silently does nothing.

## Scope

**In scope:**
- Backend: `AppSettings.Terminal` field, `TerminalInfo` type,
  `SettingsService.GetAvailableTerminals`,
  `Executor.OpenInTerminal`/`GetAvailableTerminals`/`terminalDefs`/
  `terminalExists`/`resolveDarwinBin`/`darwinTerminals`/`linuxTerminals`/
  `windowsTerminals` (all of `executor.go:53-350` except `shellQuoteDir`,
  see Boundaries below).
- Frontend: the terminal dropdown in `SettingsPage.tsx`, `TerminalInfo` in
  `types.ts`, `terminal`/`terminalAuto` locale strings in `en.json`.
- Regenerating Wails bindings after the backend change (`wails3 generate
  bindings`), since `GetAvailableTerminals` disappears from the generated
  `settingsservice.js`.

**Out of scope:**
- No schema migration. `AppSettings` is persisted as a single JSON blob in
  `app_settings.data` (confirmed: `db.go:1309-1343` reads/writes the whole
  struct as JSON, not individual columns) — removing a Go struct field just
  means old rows carry a harmless, ignored `"terminal"` key. No `migrations/`
  entry needed.
- No changes to the embedded PTY terminal panel (`TerminalService`,
  `Terminal.tsx`) — this only removes the *external*-terminal-launch code
  path, which is already unreachable from execution.
- No new "open in external terminal" feature — that was direction 2, not
  chosen.

## Boundaries

**Always:** keep `shellQuoteDir` (`executor.go:77-83`) — it's sandwiched
between functions being deleted but is actively used by the live PTY
execution path (`execution_service.go:133`) and has its own test coverage
(`execution_service_test.go`). Deleting it would break real, working code.

**Ask first:** if any other reference to `TerminalInfo`/`GetAvailableTerminals`
turns up during implementation that wasn't found during this research
(unlikely — confirmed via repo-wide grep, see the plan's Verification
section), stop and ask before deleting it.

**Never:** touch `TerminalService`/`terminal_service.go`/`pty_backend*.go` —
that's the embedded PTY system, a completely different (and very much
alive) piece of the codebase that happens to share the word "terminal."

## Success criteria

- `go build ./...` and `pnpm tsc --noEmit` pass with the setting fully removed.
- The Settings UI no longer shows a terminal-choice dropdown.
- `AppSettings`, `SettingsService`, and `Executor` no longer reference
  `Terminal`/`TerminalInfo`/`GetAvailableTerminals`/`OpenInTerminal`.
- Existing users' saved settings (with a stray `"terminal"` JSON key) still
  load without error.

## Open questions

None — the one real decision (remove vs. restore) was resolved above.
