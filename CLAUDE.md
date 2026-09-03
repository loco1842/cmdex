# CLAUDE.md

## Project Overview

Cmdex is a cross-platform desktop app for saving, organizing, and executing CLI commands as shell snippets with dynamic variable arguments. Built with Go + Wails v3 (backend/desktop) and React + TypeScript + Vite (frontend).

Commands run in a real **PTY-backed terminal** embedded in the app (xterm.js on the frontend, `TerminalService` on the backend) — not in a scraped output pane. Data is stored locally in a SQLite database at `~/.cmdex/cmdex.db` using `modernc.org/sqlite` (pure Go, no CGo).

## Prerequisites

- Go 1.26+ (see `go.mod`)
- Wails v3 CLI, pinned to `v3.0.0-beta.12` (see `.github/workflows/ci.yml`'s `WAILS_VERSION`): `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.12`
- Node.js 24+ with pnpm (frontend package manager; see `frontend/package.json`, `build/config.yml`)
- `wails.json` at the repo root is a leftover Wails v2 config file and is not read by the v3 toolchain — project config lives in `build/config.yml`

## Commands

```bash
# Development (hot-reloads frontend, restart needed for Go changes)
wails3 dev                # or: make dev  or: task dev

# Production build (output: bin/, not build/bin/)
wails3 build              # or: make build  or: task build

# Regenerate TypeScript bindings after Go changes (also happens on wails3 dev)
wails3 generate bindings  # or: make generate

# Type-check Go and TypeScript
make check                # mkdir -p frontend/dist; go build ./...; pnpm tsc --noEmit

# Format and lint (both cover Go *and* frontend; not wired into `make check`)
make fmt                  # golangci-lint fmt (goimports + golines) + pnpm lint:fix
make lint                 # golangci-lint run + pnpm lint  (same configs CI uses; fails on findings)
make lint-fix             # golangci-lint run --fix + pnpm lint:fix

# Tests (Go, then frontend Vitest unit tests, then Playwright e2e)
make test                 # go test ./... && cd frontend && pnpm test && pnpm test:e2e

# Clean build artifacts
make clean                # removes bin/ and frontend/dist/, then restores the
                          # frontend/dist/.gitkeep placeholder that
                          # //go:embed all:frontend/dist in main.go requires

# Frontend dependencies (pnpm required)
cd frontend && pnpm install
```

**Tests:** Go tests run via `go test ./...` (see the file list under **Tests** below); frontend unit tests via `cd frontend && pnpm test` (Vitest, covers pure logic in `src/utils`/`src/lib`); frontend Playwright e2e tests via `cd frontend && pnpm test:e2e`. CI's `typecheck` and `e2e` jobs run on every push/PR (type checks + lint, and Vitest + Playwright, respectively); `go-test` and `test-windows` (both `go test -race ./...`) and `build-check` are gated to manual `workflow_dispatch` runs, not triggered on every PR. `make check` runs `go build ./...` + `pnpm tsc --noEmit`, while CI's `typecheck` job additionally runs both linters as a separate, faster gate. CI never invokes the Makefile — `make check`/`fmt`/`lint` are local conveniences that mirror its individual steps.

## Architecture

### Wails Bindings (Go ↔ Frontend)

Eight services are registered as `application.Service` in `main.go` — not a single monolithic `App` struct. Wails auto-generates TypeScript bindings per service under `frontend/bindings/cmdex/<servicename>.js` (**not** `frontend/wailsjs/`, which doesn't exist in this v3 project). To add a backend feature:

1. Add a method to the relevant service struct (or create a new service and register it in `main.go`'s `Services` slice)
2. Run `wails3 generate bindings` (or `wails3 dev`, which regenerates automatically)
3. Import the generated function in React: `import { SomeMethod } from '../bindings/cmdex/servicename'` (path is depth-relative to the importing file)

`frontend/bindings/` is generated output — **never hand-edit it** — but it **is** committed, so a fresh clone type-checks without the Wails CLI installed. Regenerate and commit alongside the Go change that caused it.

### Backend (Go)

| File | Responsibility |
| --- | --- |
| `main.go` | Entry point: `application.New(...)`, registration of all 8 services, native menu, main window config |
| `app.go` | `App` service: lifecycle (`ServiceStartup`/`ServiceShutdown`), settings-window management, `GetOS`, `PickDirectory`; owns the package-level `db`/`executor`/`terminalSvc`/`wailsApp` vars other services read |
| `command_service.go` | `CommandService`: CRUD for categories/commands/presets, reordering, FTS search, `ResetAllData` |
| `execution_service.go` | `ExecutionService`: `GetVariables` (CEL defaults), `RunCommand`, working-directory resolution |
| `settings_service.go` | `SettingsService`: `GetSettings`/`SetSettings` (settings are a single JSON blob) |
| `importexport_service.go` | `ImportExportService`: `ExportCommands`/`ImportCommands`, `SaveThemeTemplate` |
| `event_service.go` | `EventService`: `GetEventNames` — exposes the `EventNames` constants so the frontend never hardcodes event strings |
| `terminal_service.go` | `TerminalService`: multi-session PTY terminals (create/close/rename/activate, write, resize, clear, start/stop). `MaxSessions = 10` |
| `launcher_service.go` | `LauncherService`: global launcher window, shortcut registration, launch-at-login, and its one persistent internal terminal session (eagerly started after `TerminalService`) |
| `pty_backend.go`, `pty_backend_unix.go`, `pty_backend_windows.go`, `pty_backend_mock.go` | PTY abstraction (`ptyHandle`/`ptyProcess`) per platform: `creack/pty` on Unix, `charmbracelet/x/conpty` on Windows, plus a mock backend for tests |
| `pty_env.go` | `buildPtyEnv`: supplies `TERM`/`COLORTERM`/`LANG` etc. that launchd-started GUI apps don't inherit, and merges shell-integration env |
| `shell_integration.go` | Materializes the embedded `shell-integration/` scripts to `~/.cmdex` and activates OSC 133 markers in the session shell via a per-session nonce |
| `terminal_capture.go` | Scans the raw PTY stream for OSC 133 `C`/`D` markers to record exact last-command output; `TerminalService.GetLastOutput` |
| `ansi.go` | ANSI/CSI stripping for captured output, including removal of ConPTY's injected wrap artifacts (see the long comment at the top — it explains why) |
| `models.go` | Data types: `Category`, `Command`, `VariableDefinition`, `VariablePreset`, `VariablePrompt`, `ExecutionRecord`, `AppSettings`, `OSPathMap` |
| `db.go` | SQLite layer: connection (WAL + foreign keys), migration runner, FTS5 search, all SQL |
| `script.go` | `GenerateScript`, `ParseScriptBody`, `ExtractTemplateVars`, `ReplaceTemplateVars`, `MergeDetectedVars` — pure functions, no I/O |
| `executor.go` | Shell selection (`$SHELL -lc` on Unix, `cmd /C` on Windows), `stripShebang`, `shellQuoteDir`, `shellDialectFor`/`buildCommandLine` (per-shell cd prefix + line-submit key), and `EvalDefaults` (CEL) |
| `migrations/` | Versioned migrations `0001`–`0010`; `migration.go` holds the ordered `Migrations` slice |

**`executor.go` no longer executes anything.** It is a helper type: shell detection plus CEL default evaluation. Nothing writes temp scripts or spawns subprocesses any more.

### How a command actually runs

`ExecutionService.RunCommand(commandID, variables)`:

1. Loads the command, applies `ReplaceTemplateVars`, then `stripShebang` and trims trailing newlines.
2. Determines the active session's shell (`SessionInfo.ShellPath`, falling back to `detectShell()` for a not-yet-started session) and passes it, the script, and the resolved working directory (`""` if none is configured) to `buildCommandLine` (`executor.go`).
3. `buildCommandLine` composes the final line: a cd prefix syntactically correct for the shell's dialect — POSIX `cd '<dir>' && `, cmd.exe `cd /d "<dir>" && `, or PowerShell `Set-Location -LiteralPath '<dir>' -ErrorAction Stop; ` (no `&&` operator on Windows PowerShell 5.1; `-ErrorAction Stop` keeps the short-circuit-on-failure behavior) — then the script, each line terminated by the shell's actual submit key: `\n` for POSIX shells (tty line discipline), `\r` for cmd.exe/PowerShell (ConPTY has no line discipline; it delivers LF as a literal Ctrl+J, not Enter). The finished line is written to the **active terminal session's PTY** via `terminalSvc.Write`.

Output therefore streams back through that session's `pty-output:<id>` event and is rendered by xterm.js in `Terminal.tsx` — `RunCommand` does not capture output. The returned `ExecutionRecord` carries `FinalCmd` and errors only; `Output`/`ExitCode` are not populated on the success path, and nothing is persisted to history. Ctrl+C works because the PTY has a real foreground process group.

If there is no active session (or `terminalSvc` is nil), `RunCommand` returns an `ExecutionRecord` with `ExitCode: -1` and an `Error` string rather than failing the promise.

### Events

Named events come from `event_service.go`'s `EventNames` struct — the frontend fetches them once via `GetEventNames()` (`frontend/src/wails/events.ts`) instead of hardcoding strings:

| Field | Event | Emitted by |
| --- | --- | --- |
| `openSettings` | `open-settings` | native menu |
| `openShortcuts` | `open-shortcuts` | native menu (`main.go`) |
| `settingsChanged` | `settings-changed` | frontend, after a settings write |
| `settingsWindowClosing` | `settings-window-closing` | `app.go` window hook |

Per-session terminal events are **not** in `EventNames` — they are built by string concatenation in `terminal_service.go` and subscribed to per session ID:

- `pty-output:<sessionId>` — `{ data: string }`, raw PTY bytes
- `pty-exit:<sessionId>` — `{ exitCode: number, wasIntentional: boolean }`
- `pty-cleared:<sessionId>` — no payload

There is no `cmd-output` event. Older docs referenced one; it does not exist.

### Frontend (React + TypeScript)

- **`App.tsx`** — central state: categories, commands, tabs, modals (discriminated union `ModalState`), theme/font/density, event subscriptions
- **`types.ts`** — TypeScript interfaces mirroring the Go models in `models.go`
- **`i18n.ts`** — i18next setup; translations in `src/locales/en.json`
- **`main.tsx`** — routes on `?window=settings`: main window lazy-loads `<App />`, settings window lazy-loads `<SettingsPage />`
- **Components** in `src/components/`: `Sidebar`, `CommandDetail`, `CommandDetailTab`, `TabBar`, `WelcomeTab`, `Terminal`, `TerminalTabBar`, `CategoryEditor`, `VariablePrompt`, `CommandPalette`, `KeyboardShortcutsDialog`, `SettingsDialog`, `SettingsPage`, `ResizablePanel`, plus inline-edit helpers (`InlineEditField`, `HoverActionButton`, `FloatingSaveBar`)
- **shadcn/ui primitives** in `src/components/ui/` (Radix UI + Tailwind CSS + CVA), kebab-case filenames
- **Hooks** in `src/hooks/`: `useKeyboardShortcuts`, `useResizable`, `useSyncedRef`, `useCopyToClipboard`
- **Utils** in `src/utils/`: `tab.ts` (tab IDs + display titles), `tabDraft.ts` (draft/baseline/dirty), `templateVars.ts`, `clipboard.ts`, `path.ts`
- **Lib** in `src/lib/`: `utils.ts` (`cn`), `shortcuts.ts` (shortcut registry), `theme-apply.ts` (`applyTheme`/`applyDensity`/`applyFonts` write CSS custom properties)
- Styling: Tailwind CSS v4 with custom CSS variables in `style.css` (`--bg-primary`, `--accent-primary`, …)

There is no `OutputPane` or `HistoryPane` — both were removed when execution moved into the PTY terminal.

### Adding a New Field to Command or Category

1. Update the struct in `models.go`
2. Add a migration in `migrations/` and update the queries/scan helpers in `db.go`
3. Update method signatures in the relevant `*_service.go` file
4. Run `wails3 generate bindings`
5. Update TypeScript interfaces in `frontend/src/types.ts`
6. Update the draft/baseline plumbing in `frontend/src/utils/tabDraft.ts` if the field is editable per tab
7. Update the relevant UI (`CommandDetail.tsx` or `CategoryEditor.tsx`)
8. Update call sites in `App.tsx`

### Tab-Based Editor Architecture

The editor is tab-based (it replaced a modal `CommandEditor`):

- **Tab identification**: the welcome tab uses the literal ID `'__welcome__'`; new-command tabs use the `__new_` prefix via `createNewTabId()` / `isNewCommandTabId()` from **`utils/tab.ts`** (not `types.ts`); saved commands use their DB ID
- **Dirty state**: each tab compares its draft against a baseline snapshot (`draftsEqual`, `cloneDraft` in `utils/tabDraft.ts`); a dot on the tab indicates unsaved changes
- **State management**: tabs live in an array, `activeTabId` controls focus, and `App` holds draft + baseline per tab for inline editing and batch save
- **Inactive tabs stay mounted**: `CommandDetailTab` is rendered with `display: none/flex` rather than unmounted, so editor state survives tab switches
- **Drafts/baselines are keyed by tab ID**: `tabDrafts` and `tabBaselines` are `Record<string, TabDraft>` state, mirrored into refs via `useSyncedRef` so event handlers read current values without re-subscribing
- **The terminal is not per-tab.** It is one shared, resizable/collapsible bottom panel with its own session tabs (`sessions`, `activeSessionId`, `terminalOrderRef`, `terminalRefs`), independent of which editor tab is focused; its height and collapsed state persist in `localStorage` under `TERM_STORAGE_KEY`. `executingTabIdRef`/`executingTabIdState` track only which editor tab triggered the current run. Earlier revisions kept per-tab output in `tabOutputRef`/`tabPaneStateRef` — those are gone, along with `applyPaneState`.
- **Adding a new tab type**: add an ID pattern/constant and handle it in `handleSelectTab`, `openTab`, and the tab rendering logic; clean up its draft/baseline entries in `finalizeCloseTab`

### Terminal & Shell Integration

- `TerminalService` owns up to `MaxSessions` (10) concurrent sessions. Each has its own PTY, read-loop goroutine, and event namespace; `sessionState` is guarded by a per-session mutex.
- **Shell integration** (`shell_integration.go`) activates OSC 133 semantic-prompt markers by pointing the session shell at extra startup files embedded from `shell-integration/` (bash, zsh, fish, pwsh) and materialized under `~/.cmdex` — it never edits the user's own dotfiles. `//go:embed all:shell-integration` needs the `all:` prefix, or zsh's dot-prefixed files (`.zshenv`, `.zshrc`, …) are silently excluded and integration never activates.
- Each session gets a fresh random **nonce** passed via a mode-0600 file (`oscNonceFileEnvVar`), not an env var value — the scripts read it and delete the file before sourcing any user profile, so spawned processes can't read it back out of `/proc/<pid>/environ` and forge markers.
- `terminal_capture.go` scans the raw stream for `C`/`D` markers and records the bytes between them as the last command's output (bounded by `maxCaptureBytes`, 1 MiB, keeping the tail). `GetLastOutput` returns `TerminalLastOutput{Available, Text, ExitCode, Truncated}`; `Available: false` means the shell has no integration and the frontend should fall back to scraping the xterm buffer. Exit code never gates `Available` — output is captured verbatim regardless of whether the command succeeded or failed. `Available: true` with blank `Text` is also possible (e.g. a stale "D" with no preceding "C"); the frontend's `copyLastOutput` (`App.tsx`) falls back to the xterm scrape in that case too, not only when `Available` is false.
- `ansi.go`'s `stripANSI` collapses bare-`\r` redraws per line by keeping the **last non-empty** segment between carriage returns, not unconditionally the text after the final one — a trailing `\r` with nothing after it (a ConPTY repaint, or PowerShell's own error-record rendering) must not wipe the line. Getting this wrong previously made "copy last output" return blank lines instead of the real error text for a failed Windows command.
- Toggled by `AppSettings.ShellIntegration` (nil = enabled). A change applies to **newly started sessions only**.
- `captureScan` must be called only from the session's single read-loop goroutine, and it treats the byte slice it is handed as immutable — it may retain a trailing partial marker in `capCarry`.

### Preset & Variable UX Patterns

**Preset management** (inline editing in `CommandDetail`):

- Presets display as chips with a context menu (rename, delete)
- The "+" chip creates an empty preset with immediate name-edit mode
- Variables render as card rows with a name label and value input
- **Keyboard navigation**: Tab/Shift+Tab between variable inputs; Enter saves; Escape cancels
- **Visual feedback**: the focused variable is highlighted in the command preview; a TEMPLATE badge marks the placeholder view

**Variable preview system**:

- Dual preview: Template (with `{{var}}` placeholders) + Resolved (with actual values)
- Focus highlight follows the variable being edited; `var-focused` CSS class draws the outline
- Preset save: check-icon button in the Preview header; auto-save on Enter

## Key Design Decisions

- Commands use `{{variableName}}` template syntax (e.g. `echo "Hello {{name}}"`) — deliberately not shell syntax, so substitution is unambiguous before the text reaches a shell
- Variables are auto-detected from `{{var}}` patterns in the script body, and can also be added manually; `MergeDetectedVars` keeps detected ones first and preserves manual-only ones
- At execution time `{{var}}` placeholders are replaced by simple string substitution, then the resolved line is written to the active PTY
- Variable defaults support CEL (Google Common Expression Language) with custom functions `now()`, `env("KEY")`, `date("2006-01-02")`. A default that fails to compile or evaluate falls back to the raw expression string
- **Scripts are stored without a shebang.** `GenerateScript` only trims and adds a trailing newline; `ParseScriptBody` and `stripShebang` strip a leading `#!` line for backward compatibility with older DB records
- Commands can have named presets (saved sets of variable values)
- Settings are one JSON blob in a single `app_settings` row, not a column per setting (migration 0009)
- Working directory resolution is a fallback chain: per-command → global default → `$HOME` → `os.Getwd()` → `os.TempDir()`; it never returns empty
- `OSPathMap` stores per-OS paths (`darwin`/`linux`/`windows`) so a synced command works on every machine
- SQLite with FTS5 for full-text search on title, description, and script content, with a `LIKE` fallback if FTS5 fails
- Foreign keys with `ON DELETE CASCADE` for referential integrity — except `commands.category_id`, which uses `ON DELETE SET NULL` so deleting a category uncategorizes its commands rather than deleting them
- Settings live in a separate native window (`?window=settings`), not a modal, so the editor keeps its context
- Theme colors use CSS variables — modify those rather than hardcoding colors
- `time.Time` fields (e.g. `CreatedAt`, `ExecutedAt`) generate as plain `string`-typed fields in the bindings — no separate `time` package output, no warnings expected

## Gotchas

- `category_id` in `commands` is nullable (`NULL` = uncategorized). Use the `nullableString()` helper when inserting/updating and `sql.NullString` when scanning. `Command.Title`/`Description` are `sql.NullString`, so the frontend sees `{ String, Valid }`.
- After changing Go structs or method signatures, add a migration in `migrations/` (see the pattern below) — or delete `~/.cmdex/cmdex.db` during dev. There is no `schemaVersion` constant in `db.go`: the applied version lives in the `schema_version` table and is driven by the ordered `Migrations` slice in `migrations/migration.go`.
- Schema migrations must recreate tables (SQLite has no `ALTER COLUMN`) and are wrapped in a transaction by the runner.
- `//go:embed all:frontend/dist` in `main.go` requires `frontend/dist` to exist and be non-empty, or `go build`/`go vet`/`go test`/`wails3 build` all fail with `pattern all:frontend/dist: no matching files found`. A tracked `frontend/dist/.gitkeep` keeps a fresh checkout working; `make check`/`make test`/`make clean` all `mkdir -p` it. Only bare `go build`/`go test` invocations can hit this.
- `RenameCommand` is a metadata-only DB method — don't round-trip scripts through `UpdateCommand` just to change a title.
- `frontend/tsconfig.json` has `strict: false`. Don't assume strict-mode enforcement.
- Mixed frontend punctuation: `App.tsx` and most components use semicolons + single quotes; `components/ui/` uses double quotes and no semicolons. **Match the file you are editing.**
- `frontend/e2e/mocks/runtime.ts` mocks `@wailsio/runtime` by hardcoding each generated binding's numeric `$Call.ByID` method hash. If you regenerate bindings and IDs shift, update that handler table or tests fail quietly with `[e2e mock] no handler for method ID …` console warnings.
- Terminal event names are per session (`pty-output:<id>`) — subscribe after you know the session ID, and clean up on session close.

### Schema Migration Pattern (SQLite)

SQLite doesn't support `ALTER COLUMN`, so schema changes require table recreation. Migrations live in the `migrations/` package (`migrations/NNNN_description.go`), not inline in `db.go`. Each file defines a `Migration` struct (see `migrations/migration.go`):

```go
package migrations

import "database/sql"

var migrationNNNN = Migration{
	Version:     N, // schema_version value after this migration
	Description: "what this migration does",
	Up: func(tx *sql.Tx) error {
		stmts := []string{
			`CREATE TABLE commands_new (...)`,                  // New schema
			`INSERT INTO commands_new SELECT * FROM commands`,  // Copy data
			`DROP TABLE commands`,
			`ALTER TABLE commands_new RENAME TO commands`,
			// Re-create triggers, indexes, FTS table/triggers
		}
		for _, s := range stmts {
			if _, err := tx.Exec(s); err != nil {
				return err
			}
		}
		return nil
	},
}
```

Then append the new migration to the `Migrations` slice in `migrations/migration.go`.

**Key rules:**

- The runner (`db.go`) wraps each migration's `Up` in a transaction automatically — don't `BEGIN`/`COMMIT` inside `Up`.
- `Migrations` is compared by `Migration.Version`, not slice index — version 4 was intentionally merged into 5, so versions can skip.
- Set `DisableFKDuringMigration: true` only if the migration needs `PRAGMA foreign_keys = OFF` before its transaction begins (see `migration0005`).
- After recreating the `commands` table, rebuild the FTS index: `INSERT INTO commands_fts(commands_fts) VALUES('rebuild')` — skipping this leaves FTS out of sync.
- Recreate FTS triggers after table changes.
- Handle data transformation (e.g. empty string → NULL) in the migration.

## Tests

| Area | Files |
| --- | --- |
| Migrations | `db_test.go` — `TestFreshDBMigrations`, `TestExistingDBIdempotent`, `TestRollbackTo` |
| Terminal sessions | `terminal_service_test.go`, `terminal_service_race_test.go`, `terminal_service_stress_test.go`, `terminal_service_max_sessions_test.go` — CRUD, cwd inheritance, 100-cycle stress, `MaxSessions` limit. Stress and max-sessions variants are `//go:build darwin` and use the mock backend |
| PTY backends | `pty_backend_unix_test.go`, `pty_backend_windows_test.go` (real conpty integration), `pty_backend_windows_conpty_spike_test.go` (raw library), `pty_backend_mock_test.go` |
| Execution | `execution_service_test.go` — working-dir resolution, `FinalCmd` construction |
| Capture & ANSI | `terminal_capture_test.go`, `ansi_test.go`, `shell_integration_test.go` |
| Env | `pty_env_test.go` |
| Frontend e2e | `frontend/e2e/tests/*.spec.ts` (Playwright) |

`TestTerminalShutdown`/`TestTerminalExit` are Windows-skipped — they call the raw `ptyStart` helper with Unix shell syntax, not because the Windows backend is unsupported. See **Known Cross-Platform Gaps** in `AGENTS.md`. `TestRunCommand_FinalCmdWithWorkingDir`/`FinalCmdMultilineScript` run on every platform; `TestBuildCommandLine` (`execution_service_test.go`) is the platform-independent table test covering POSIX/cmd.exe/PowerShell command-line construction, and `shell_integration_test.go`'s `TestPwshIntegration_*` suite includes Windows-only ConPTY execution tests gated on `pwshRealPTYSkipReason`.

## Conventions

- **Go formatting**: `make fmt` (`golangci-lint fmt`), **not** `gofmt` — `gofmt` is deliberately commented out in `.golangci.yml`'s `formatters:` block. The enabled formatters are `goimports` (with `local-prefixes: cmdex`, so local imports group after third-party) and `golines` (**max line length 120**, which plain `gofmt` does not enforce). Tabs for indentation, as usual.
- **Go naming**: exported, Wails-bound API in PascalCase; unexported helpers in camelCase; JSON tags in camelCase (`json:"categoryId"`). Imports: stdlib, blank line, third-party, blank line, local `cmdex/...`. Section banners (`// ========== Category Operations ==========`) organize the larger service files.
- **Go error handling**: read-style bound methods log with `fmt.Println`/`Printf` and return empty slices rather than surfacing an error to the frontend; mutations return `(T, error)` or `error` and propagate. `forbidigo` is disabled in `.golangci.yml` precisely because of that `fmt.Println` logging convention.
- **TypeScript**: PascalCase component files under `src/components/`, kebab-case under `src/components/ui/`, `use`-prefixed hooks, `@/*` → `frontend/src/*`. Page-level components use default exports; shared symbols use named exports.
- **Lint**: `.golangci.yml` (golangci-lint v2) and `frontend/eslint.config.mjs` both block CI's `typecheck` job on any finding. Run both locally with `make lint`.

## Further Reading

- `AGENTS.md` — condensed agent quick reference, plus **Known Cross-Platform Gaps (Windows)** and how to exercise CI locally with `act`
- `docs/ARCHITECTURE.md` — system design, data flow, schema
- `docs/API.md` — every bound service method and event, with TypeScript examples
- `docs/DEVELOPMENT.md`, `docs/TESTING.md`, `docs/CONFIGURATION.md`, `docs/DEPLOYMENT.md`, `docs/GETTING-STARTED.md`
