# Issue #57: terminal setting in system settings no longer used

**Issue:** https://github.com/loco1842/cmdex/issues/57
**Category:** bug _(recategorized from enhancement/question — see TRIAGE.md notes)_ · **Priority:** P2 · **Effort:** L (7 files; revised up from the initial M guess once the full removal scope was grounded)
**Author:** @loco1842 · **Opened:** 2026-08-15
**Spec:** [issue-57-terminal-setting-removal.spec.md](issue-57-terminal-setting-removal.spec.md) — direction (remove vs. restore) decided there
**PR:** _(filled in by --fix)_

## Problem

The Settings UI still offers a terminal-emulator dropdown (`SettingsPage.tsx`)
that persists to `AppSettings.Terminal`, but nothing has read that value at
execution time since command execution moved to the embedded PTY panel.
Users who set a preferred terminal get no error and no indication — it's
just silently ignored.

## Verification

- **Confirmed a genuine regression, not "never wired up":** `git log -S"OpenInTerminal"`
  shows the only call site (`ExecutionService.RunInTerminal`) was deleted in
  commit `edb16c4` ("Milestone v2.1: Terminal Sessions (#42)", ~2026-06-18),
  when execution switched to `terminalSvc.Write(session.ID, cmdLine)`
  (`execution_service.go:116-153`, the current `RunCommand`). `settings_service.go`
  was never updated in that commit — it kept exposing `GetAvailableTerminals`,
  so the UI still offers a choice with nothing behind it.
- **Confirmed nothing else reads it:** repo-wide grep for `OpenInTerminal(`
  and `GetAvailableTerminals(` (both Go and frontend) shows call sites only
  in the dead feature's own definition chain — `executor.go:55` /
  `executor.go:95` → `settings_service.go:41` → `SettingsPage.tsx:277,674`.
  No other consumer exists.
- **Direction confirmed with the user** via the spec (see link above): remove
  the setting rather than restore external-terminal launching.

## Current behavior

- `models.go:148-152` — `TerminalInfo` struct (only used by this feature).
- `models.go:157` — `AppSettings.Terminal string` field, comment `// terminal ID; empty = auto-detect`.
- `settings_service.go:39-42` — `SettingsService.GetAvailableTerminals()`, delegates to the executor.
- `executor.go:53-350` — the entire terminal-launch subsystem: `OpenInTerminal` (53-75), `terminalDef` type (85-92), `GetAvailableTerminals` (94-107), `resolveDarwinBin` (109-114), `terminalExists` (116-129), `terminalDefs` (131-141), `darwinTerminals` (143-239), `linuxTerminals` (240-308), `windowsTerminals` (309-350).
  - **Exception, do not delete:** `shellQuoteDir` (`executor.go:77-83`) sits inside this range but is actively used by the live PTY execution path (`execution_service.go:133`) and has dedicated test coverage in `execution_service_test.go`. This is the one easy way to get this task wrong — deleting a range instead of the specific functions in it.
- `db.go:1310` (`Terminal: ""` in `GetSettings` defaults), `db.go:1352-1354` (`if s.Terminal != "" { existing.Terminal = s.Terminal }` in `SetSettings`), `db.go:1434` (`Terminal: "",` in the factory-reset default-settings construction).
- `frontend/src/types.ts:80` (`TerminalInfo` interface), `frontend/src/types.ts:125` (`terminal?: string` on the settings type).
- `frontend/src/components/SettingsPage.tsx` — import (`:10,13`), state (`:155,157,178,187`), `changeTerminal` (`:254-256`), fetch-on-mount (`:277-279`), load-from-settings (`:288,290`), the dropdown JSX (`:577-591`), and a second reload call site (`:672,674`).
- `frontend/src/locales/en.json:200-201` — `settings.terminal` / `settings.terminalAuto` strings.

No SQLite schema migration is involved: `app_settings` stores the whole
`AppSettings` struct as one JSON blob in a `data` column (`db.go:1309-1343`),
so removing a Go struct field just leaves old rows with a harmless, ignored
`"terminal"` key.

## Approach

Delete top-down: backend first (so the frontend's `GetAvailableTerminals`
import fails to compile against stale bindings if anything is missed, which
is a useful forcing function), then frontend, then regenerate bindings and
confirm they're consistent, then remove the now-orphaned locale strings last
(so nothing references them mid-way through).

**Rejected alternative:** leave the backend functions in place (unreachable
but harmless) and just hide the frontend dropdown. Rejected — dead code that
looks reachable is exactly what caused this issue to go unnoticed for two
months; deleting it outright is the more honest fix and was the whole point
of the chosen direction.

## File map

| File | Change |
|---|---|
| `models.go` | Delete `TerminalInfo` struct, `AppSettings.Terminal` field |
| `settings_service.go` | Delete `GetAvailableTerminals` method |
| `executor.go` | Delete `OpenInTerminal`, `terminalDef`, `GetAvailableTerminals`, `resolveDarwinBin`, `terminalExists`, `terminalDefs`, `darwinTerminals`, `linuxTerminals`, `windowsTerminals`. Keep `shellQuoteDir`. |
| `db.go` | Remove the three `Terminal`/`Terminal: ""` references in `GetSettings`/`SetSettings`/factory-reset |
| `frontend/src/types.ts` | Delete `TerminalInfo` interface, `terminal?` field |
| `frontend/src/components/SettingsPage.tsx` | Delete terminal state, handlers, dropdown JSX, related imports |
| `frontend/src/locales/en.json` | Delete `settings.terminal` / `settings.terminalAuto` keys |
| `frontend/bindings/cmdex/*` | Regenerated via `wails3 generate bindings`, not hand-edited |

## Tasks

### Task 1: Remove the dead terminal-launch subsystem (backend)

**Files:** modify `models.go`, `settings_service.go`, `executor.go`, `db.go`
**Depends on:** none

- [ ] Step 1: delete `TerminalInfo` (`models.go:148-152`) and the `Terminal`
  field + comment from `AppSettings` (`models.go:157`).

- [ ] Step 2: delete `GetAvailableTerminals` from `settings_service.go:39-42`.

- [ ] Step 3: in `executor.go`, delete `OpenInTerminal` (53-75),
  `terminalDef` (85-92), `GetAvailableTerminals` (94-107), `resolveDarwinBin`
  (109-114), `terminalExists` (116-129), `terminalDefs` (131-141),
  `darwinTerminals` (143-239), `linuxTerminals` (240-308), `windowsTerminals`
  (309-350). **Leave `shellQuoteDir` (77-83) exactly where it is** — it's
  used by `execution_service.go:133` and covered by
  `execution_service_test.go`.

- [ ] Step 4: in `db.go`, remove the three `Terminal` references:

  ```go
  // GetSettings defaults (db.go:1310) — before:
  Locale: "en", Terminal: "",
  // after:
  Locale: "en",
  ```
  ```go
  // SetSettings (db.go:1352-1354) — delete entirely:
  if s.Terminal != "" {
      existing.Terminal = s.Terminal
  }
  ```
  ```go
  // factory-reset defaults (db.go:1434) — delete the line:
  Terminal:       "",
  ```

**Acceptance:** `go build ./...` succeeds; no remaining references to
`Terminal`, `TerminalInfo`, `GetAvailableTerminals`, or `OpenInTerminal`
anywhere in the Go codebase (`grep -rn` confirms empty); `shellQuoteDir`
still exists and is unchanged.
**Verify:** `go build ./...`; `go test ./...` (confirms
`execution_service_test.go`'s `shellQuoteDir` tests still pass); `grep -rn
"TerminalInfo\|GetAvailableTerminals\|OpenInTerminal" --include="*.go" .`
returns nothing.

### Task 2: Remove the dropdown and regenerate bindings (frontend)

**Files:** modify `frontend/src/types.ts`, `frontend/src/components/SettingsPage.tsx`, `frontend/src/locales/en.json`
**Depends on:** Task 1 (bindings regeneration needs the backend change first)

- [ ] Step 1: run `wails3 generate bindings` — this updates
  `frontend/bindings/cmdex/settingsservice.js` and `models.js` to drop
  `GetAvailableTerminals`/`TerminalInfo` automatically; do not hand-edit
  generated files.

- [ ] Step 2: delete from `frontend/src/types.ts`: the `TerminalInfo`
  interface (`:80`) and the `terminal?: string` field (`:125`).

- [ ] Step 3: in `frontend/src/components/SettingsPage.tsx`, remove:
  - the `GetAvailableTerminals` import (`:10`) and `TerminalInfo` type import (`:13`)
  - `terminals`/`setTerminals` state (`:155`), `terminal`/`setTerminal` state (`:157`), `terminalRef` (`:178,187`)
  - `terminal: terminalRef.current` from the persisted-settings payload (`:207`)
  - `changeTerminal` (`:254-256`)
  - the `GetAvailableTerminals()` fetch-on-mount effect (`:277-279`)
  - `const term = s?.terminal || ''` / `setTerminal(term)` (`:288,290`)
  - the dropdown block (`:577-591`):
    ```tsx
    <div className="space-y-2">
      <Label>{t('settings.terminal')}</Label>
      <Select value={terminal || '__auto__'} onValueChange={(v) => changeTerminal(v === '__auto__' ? '' : v)}>
        ...
      </Select>
    </div>
    ```
  - the second reload call site (`:672,674`)

- [ ] Step 4: delete `settings.terminal` and `settings.terminalAuto` from
  `frontend/src/locales/en.json:200-201`. Run the i18n-checker agent (or a
  manual grep for `t('settings.terminal`) afterward to confirm nothing else
  references these keys.

**Acceptance:** `pnpm tsc --noEmit` passes; the Settings page no longer
renders a terminal dropdown; no remaining reference to `terminal`/
`TerminalInfo` in frontend source (excluding the unrelated embedded
`TerminalService`/`Terminal.tsx` PTY code, which this task never touches).
**Verify:** `pnpm tsc --noEmit`; manual check in `wails3 dev` — open
Settings, confirm the dropdown is gone and nothing else in the panel shifted
unexpectedly; `grep -rn "settings.terminal" frontend/src` returns nothing.

### Checkpoint: after Task 2

- [ ] `make check` passes (`go build ./...` + `pnpm tsc --noEmit`)
- [ ] `go test ./...` passes
- [ ] Settings UI opens cleanly with no terminal dropdown and no console errors
- [ ] An existing `~/.cmdex/cmdex.db` with a saved `"terminal"` value still loads settings without error (confirms the JSON-blob backward-compat assumption)

## Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Deleting `shellQuoteDir` along with the surrounding dead functions | Breaks live PTY command execution (`execution_service.go:133`) and its tests | Explicitly called out in Task 1 Step 3 and the Approach section; verify with `go test ./...` before considering Task 1 done |
| Missing a reference during frontend cleanup (stale import, dangling locale key) | TypeScript build fails or a console warning about a missing i18n key | `pnpm tsc --noEmit` catches dead imports; grep for `settings.terminal` catches locale keys |

## Out of scope

- No new "open in external terminal" feature (that was the rejected direction — see the spec).
- No changes to `TerminalService`, `terminal_service.go`, `pty_backend*.go`, or `Terminal.tsx` — the embedded PTY panel is untouched.
- No database migration — confirmed unnecessary (see Current behavior).

## Open questions

None — resolved in the spec.
