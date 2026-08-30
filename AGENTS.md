# Agents documentation

**Cmdex** — cross-platform desktop app for saving and executing CLI commands with `{{variable}}` placeholders.

Stack: Go + Wails **v3** + React 19 + Vite + TypeScript + SQLite (`modernc.org/sqlite`).

## Essential Commands

```bash
# Development (auto-restarts on Go changes, HMR on frontend)
wails3 dev                              # or: make dev  or: task dev

# Regen frontend bindings after Go service changes
wails3 generate bindings                # or: make generate

# Production build
wails3 build                            # or: make build  or: task build

# Full checks (run before committing)
make check                              # go build ./... && cd frontend && pnpm tsc --noEmit

# Format + lint (not wired into `make check`; blocking in CI on any finding).
# Both targets cover Go *and* the frontend.
make fmt                                # golangci-lint fmt (goimports + golines) + pnpm lint:fix
make lint                               # golangci-lint run + pnpm lint (same configs CI uses)
make lint-fix                           # golangci-lint run --fix + pnpm lint:fix

# Frontend lint on its own
cd frontend && pnpm lint                # pnpm lint:fix for auto-fix

# Go tests
go test ./...                           # go test -run TestFreshDBMigrations -v ./...

# Frontend unit tests (Vitest)
cd frontend && pnpm test

# Frontend e2e tests (Playwright)
cd frontend && pnpm test:e2e

# Go tests + frontend unit tests + frontend e2e tests together
make test
```

**Note:** The Makefile targets `make dev/build/generate/check/fmt/lint/lint-fix/test/clean` exist. `make check`, `make test`, and `make clean` all `mkdir -p frontend/dist` so the `//go:embed all:frontend/dist` directive in `main.go` always has something to embed; `make clean` also restores the tracked `frontend/dist/.gitkeep` placeholder after removing `bin/` (the real build output dir) and `frontend/dist/`. The Taskfile (`task dev`, `task build`) provides more options (Docker cross-compile, server mode). Both call `wails3` under the hood.

## Architecture (Wails v3)

### Service registration (not the old v2 single-App pattern)

In `main.go`, seven services are registered as `application.Service`:

| Service struct | File | Frontend binding import |
|---|---|---|
| `App` | `app.go` | `../bindings/cmdex/app` |
| `CommandService` | `command_service.go` | `../bindings/cmdex/commandservice` |
| `ExecutionService` | `execution_service.go` | `../bindings/cmdex/executionservice` |
| `SettingsService` | `settings_service.go` | `../bindings/cmdex/settingsservice` |
| `ImportExportService` | `importexport_service.go` | `../bindings/cmdex/importexportservice` |
| `EventService` | `event_service.go` | `../bindings/cmdex/eventservice` |
| `TerminalService` | `terminal_service.go` (+ `pty_backend*.go`) | `../bindings/cmdex/terminalservice` |

Each service is a struct implementing `ServiceStartup(ctx, options) error`. Wails generates bindings from exported methods into `frontend/bindings/cmdex/<servicename>.js` (JS with JSDoc types, plus `models.js` and a barrel `index.js`). **Never hand-edit `frontend/bindings/`** — it's generated output, but it **is** committed (so a fresh clone type-checks without the Wails CLI installed); regenerate with `wails3 generate bindings` and commit the result alongside the Go change that caused it.

### Adding a new feature

1. Add/update the method on the relevant service struct (or create a new service).
2. If a new service, register it in `main.go` `Services` slice.
3. Run `wails3 dev` (or `wails3 generate bindings`) to regenerate `frontend/bindings/`.
4. Import the generated function in your React code:
   ```ts
   import { SomeMethod } from '../bindings/cmdex/servicename';
   ```
5. Update TS types in `frontend/src/types.ts` and call sites in `App.tsx` or components.

### Adding a new field to Command or Category

1. Update the struct in `models.go`.
2. Add a migration in the `migrations/` package and append it to the ordered `Migrations` slice in `migration.go` (during dev you can instead delete `~/.cmdex/cmdex.db`).
3. Update `db.go` CRUD queries and scan helpers.
4. Update the service method signatures in the relevant `*_service.go` file.
5. Run `wails3 dev` to regenerate bindings.
6. Update TypeScript types in `frontend/src/types.ts`.
7. Update `CommandDetail.tsx` / `CommandDetailTab.tsx` or `CategoryEditor.tsx`.
8. Update `App.tsx` where the create/update calls are made.

### Event system

Events bridge Go and the frontend. There are two kinds.

**Named events** — declared in `event_service.go`'s `EventNames` struct, fetched once by the frontend via `GetEventNames()` (see `frontend/src/wails/events.ts`) so no event string is hardcoded:

| Field | Event | Emitted by |
|---|---|---|
| `openSettings` | `open-settings` | native menu |
| `openShortcuts` | `open-shortcuts` | native menu (`main.go`) |
| `settingsChanged` | `settings-changed` | frontend, after a settings write |
| `settingsWindowClosing` | `settings-window-closing` | `app.go` window hook |

**Per-session terminal events** — built by string concatenation in `terminal_service.go`, *not* part of `EventNames`, so they must be subscribed per session ID:

```ts
import { Events } from '@wailsio/runtime';
Events.On(`pty-output:${sessionId}`, handler);   // { data: string } — raw PTY bytes
Events.On(`pty-exit:${sessionId}`, handler);     // { exitCode: number, wasIntentional: boolean }
Events.On(`pty-cleared:${sessionId}`, handler);  // no payload
```

`Terminal.tsx` feeds `pty-output` straight into xterm.js. **There is no `cmd-output` event** — command output no longer flows through a Go-side streaming callback at all.

## Key Files & Responsibilities

| File | Purpose |
|---|---|
| `main.go` | Entry point, service registration, native menus, window config |
| `app.go` | App lifecycle (`ServiceStartup`/`Shutdown`), settings window management; `db` and `executor` are package-level vars initialized here |
| `command_service.go` | Category + Command CRUD, presets, reordering, FTS search, `ResetAllData` |
| `execution_service.go` | `GetVariables` (CEL defaults), `RunCommand`, working-directory resolution |
| `settings_service.go` | `GetSettings`, `SetSettings` (settings are one JSON blob) |
| `importexport_service.go` | Import/export commands, theme templates |
| `event_service.go` | `GetEventNames` — exposes event name strings to frontend |
| `db.go` | SQLite access, schema DDL, migrations runner, FTS5 search, all SQL queries |
| `models.go` | Go domain types for Wails/JSON and SQL scanning |
| `script.go` | `{{var}}` parsing and substitution, shebang *stripping* (pure functions, no I/O) |
| `executor.go` | Shell selection, `stripShebang`, `shellQuoteDir`, `shellDialectFor`/`buildCommandLine` (per-shell cd prefix + line-submit key), CEL default evaluation. **Executes nothing** |
| `terminal_service.go` | `TerminalService`: multi-session PTY terminals, `MaxSessions = 10` |
| `pty_backend*.go` | PTY abstraction per platform: `creack/pty` (Unix), `charmbracelet/x/conpty` (Windows), plus a mock for tests |
| `pty_env.go` | `buildPtyEnv` — supplies `TERM`/`COLORTERM`/`LANG` that launchd-started GUI apps don't inherit |
| `shell_integration.go` | Materializes embedded `shell-integration/` scripts to `~/.cmdex`; activates OSC 133 markers via a per-session nonce |
| `terminal_capture.go` | OSC 133 `C`/`D` marker scanner; `GetLastOutput` for exact last-command output |
| `ansi.go` | ANSI/CSI stripping, including removal of ConPTY's injected line-wrap artifacts |
| `migrations/` | Versioned migration files (`0001_initial.go` … `0010_working_dir.go`), `migration.go` defines the ordered `Migrations` slice |
| `shell-integration/` | Embedded bash/zsh/fish/pwsh startup scripts that emit the OSC 133 markers |
| `frontend/src/App.tsx` | Central state: tabs, modals (discriminated union `ModalState`), terminal sessions, data loading, event subscriptions |
| `frontend/src/components/Terminal.tsx` | xterm.js host; subscribes to the session's `pty-*` events |
| `frontend/src/types.ts` | TS mirrors of Go domain types |
| `frontend/src/utils/tab.ts` | Tab ID helpers (`isNewCommandTabId`, `createNewTabId`) and `getCommandDisplayTitle` |
| `frontend/src/utils/tabDraft.ts` | Tab draft/baseline state and dirty comparison |
| `frontend/src/utils/templateVars.ts` | Variable detection and merging for UI |
| `frontend/src/lib/theme-apply.ts` | `applyTheme`/`applyDensity`/`applyFonts` — write CSS custom properties |
| `frontend/src/hooks/useKeyboardShortcuts.ts` | Global keyboard shortcuts |

## Data & Storage

- SQLite at `~/.cmdex/cmdex.db`, opened with WAL + foreign keys enabled.
- Commands use `{{variableName}}` template syntax (double braces).
- Variables auto-detected from `{{var}}` patterns; can also be added manually.
- Variable defaults support CEL expressions: `now()`, `env("KEY")`, `date("2006-01-02")`.
- **Scripts are stored without a shebang.** `GenerateScript` only trims and adds a trailing newline; `ParseScriptBody`/`stripShebang` strip a leading `#!` line for backward compatibility with older DB records.
- Settings are a single JSON blob in one `app_settings` row (migration 0009), not a column per setting.
- `commands.category_id` is nullable + `ON DELETE SET NULL` (deleting a category uncategorizes its commands).

## How a command runs

`ExecutionService.RunCommand` resolves `{{vars}}`, strips any shebang, then calls `buildCommandLine(shellPath, script, workingDir)` (`executor.go`), which composes a shell-dialect-correct line: a cd prefix (only when a working directory is configured for the current OS) — POSIX `cd '<dir>' && `, cmd.exe `cd /d "<dir>" && `, PowerShell `Set-Location -LiteralPath '<dir>' -ErrorAction Stop; ` — followed by the script, each line terminated by the shell's actual submit key (`\n` for POSIX, `\r` for cmd.exe/PowerShell, since ConPTY has no tty line discipline and treats LF as a literal Ctrl+J rather than Enter). The finished line is then **written to the active terminal session's PTY** via `terminalSvc.Write`. Output streams back over `pty-output:<id>` into xterm.js; `RunCommand` captures nothing and persists no history. Ctrl+C works because the PTY has a real foreground process group.

With no active session, it returns an `ExecutionRecord` with `ExitCode: -1` and an `Error` string rather than rejecting.

Removed along the way — do not reintroduce references to them: `RunInTerminal`, `GetExecutionHistory`, `ClearExecutionHistory`, `GetAvailableTerminals`, the `TerminalInfo` model, the `terminal` setting, and the `OutputPane`/`HistoryPane` components.

## Schema Migrations

- `migrations/` package contains versioned migration files.
- `db.go` runs migrations via `migrations.Migrations` slice (version order matters — version 4 was intentionally merged into 5).
- Adding a migration: create `migrations/NNNN_description.go`, define a `Migration` struct, append to `Migrations` slice in `migration.go`.
- After changing schema, delete `~/.cmdex/cmdex.db` during dev (or the migration handles the upgrade if you wrote it).

## Gotchas

- `wails3 generate bindings` replaces the old v2 `wails generate module` — if you don't see new methods, make sure you ran this.
- `category_id` in commands is nullable — use `sql.NullString` for Go scanning; `NULL` means uncategorized.
- `frontend/tsconfig.json` has `strict: false`. Don't assume strict-mode enforcement.
- Mixed punctuation in frontend: `App.tsx` and most components use semicolons + single quotes; `ui/` components use double quotes + no semicolons. Match the file you're editing.
- Editor tabs stay mounted when inactive (`CommandDetailTab` toggles `display`, it doesn't unmount), so don't rely on unmount cleanup for tab state.
- The terminal is **one shared bottom panel** with its own session tabs, not per-editor-tab output. Older docs described `tabOutputRef`/`tabPaneStateRef`/`applyPaneState` — all removed.
- Themes use CSS variables in `frontend/src/style.css` — modify variables, not hardcoded colors.
- Terminal event names are per session (`pty-output:<id>`), so subscribe only once you have the session ID and clean up on close.
- `AppSettings.ShellIntegration` changes apply to **newly started sessions only**, never to running ones.

## Tests

- Go: `db_test.go` — three migration tests (`TestFreshDBMigrations`, `TestExistingDBIdempotent`, `TestRollbackTo`). Run with `go test ./...`.
- Go: `execution_service_test.go` (working-dir resolution, `FinalCmd` construction, and `TestBuildCommandLine` — a platform-independent table test covering POSIX/cmd.exe/PowerShell command-line construction by shell base name, runnable on any OS), `terminal_capture_test.go` + `ansi_test.go` + `shell_integration_test.go` (OSC 133 capture, ANSI/ConPTY wrap-artifact stripping, script materialization), `pty_env_test.go`.
- Go: `terminal_service_test.go` + `terminal_service_stress_test.go` + `terminal_service_max_sessions_test.go` — multi-session CRUD, cwd inheritance, 100-cycle stress test, and MaxSessions limit test. Stress and max-sessions tests are `//go:build darwin` (use the mock backend from `pty_backend_mock_test.go`).
- **Windows PTY backend is real**, backed by `github.com/charmbracelet/x/conpty` (`pty_backend_windows.go`; `conptyBackend`/`conptyProcess`/`conptyHandle`). A dedicated `test-windows` CI job (`.github/workflows/ci.yml`) runs the full Go test suite on `windows-latest`, including `pty_backend_windows_test.go` (real-backend integration tests: write→read round-trip, process termination verification, bad-working-dir fallback, dimension validation) and `pty_backend_windows_conpty_spike_test.go` (lower-level tests against the raw conpty library, kept as narrower regression coverage independent of the `TerminalService` integration layer). `TestTerminalShutdown`/`TestTerminalExit` (call the raw `ptyStart` helper with Unix shell syntax) remain Windows-skipped for reasons unrelated to backend support. `shell_integration_test.go`'s `TestPwshIntegration_BuiltCommandLineActuallyExecutes`/`LFAloneDoesNotSubmitCommandLine`/`WorkingDirPrefixChangesDirectory`/`WorkingDirPrefixShortCircuitsOnBadDir` are the Windows-only ConPTY execution tests for issue #63's fix, gated on `pwshRealPTYSkipReason` (skip on non-Windows or when pwsh isn't installed) rather than a build tag, so they still compile and vet on every OS.
- Frontend: Vitest unit tests (`frontend/src/**/*.test.ts`, covering pure logic in `src/utils`/`src/lib`) run with `cd frontend && pnpm test`. Playwright e2e tests under `frontend/e2e/tests/*.spec.ts` (tabs, presets, variables/execution, palette/shortcuts, sidebar, settings, errors, i18n, terminal, commands, categories, themes, smoke test, plus a `mock-contract.spec.ts` guard) run with `cd frontend && pnpm test:e2e`, or `make test` which runs `go test ./...`, then the Vitest suite, then the e2e suite. `frontend/e2e/mocks/runtime.ts` mocks `@wailsio/runtime` by dispatching on each generated binding's numeric `$Call.ByID` method hash (named once in its `METHOD_IDS` table) — if you regenerate bindings and IDs shift, `mock-contract.spec.ts` fails the build instead of the historical silent `[e2e mock] no handler for method ID …` console warning. The mock also supports one-shot/sticky RPC fault injection (`window.__cmdexE2E.failNext`/`setFailure`) and a call log (`window.__cmdexE2E.callLog`) for asserting exactly what was called.
- `.golangci.yml` (golangci-lint v2 config; disables `unused`, `forbidigo` — the latter's default fmt.Println/Printf ban conflicts with this codebase's logging convention — plus several complexity/style linters listed in the config's `disable:` block; `_test.go` files are excluded via `exclusions.paths`). Runs in CI's `typecheck` job via `golangci/golangci-lint-action` and **blocks the run on any finding** (same as `pnpm lint`) — currently 0 findings. `make lint` runs this exact config locally (`golangci-lint run`, no `|| true`, so it also fails on findings). `make fmt` (`golangci-lint fmt`) is a separate formatting-only command that rewrites files in place using the formatters configured under `.golangci.yml`'s `formatters:` block (`goimports`, `golines`).
- CI's `go-test` job and the `test-windows` job both run `go test -race ./...` (not just `go test ./...`) and block the run on failure, but both are gated to manual `workflow_dispatch` runs — they do not run on every push/PR. The `e2e` job (Vitest + Playwright) and the `typecheck` job (which also runs `golangci-lint` and `pnpm lint`, blocking on any finding) do run on every push/PR. `make check`/`make fmt`/`make lint` are local-only conveniences — CI does not invoke the Makefile directly.

## Known Cross-Platform Gaps (Windows)

The Windows PTY backend itself is real and CI-verified (see Tests above), but one adjacent, pre-existing issue remains — found while implementing it, deliberately left unfixed as out of scope for that work:

- **`buildPtyEnv` (`pty_env.go`) is not fully Windows-correct.** Windows' `os.Environ()` includes hidden per-drive cwd entries like `=C:=C:\foo`; `buildPtyEnv`'s `strings.Cut(kv, "=")` collapses all of these into a single malformed `""`-keyed map entry, so only one survives the round-trip. Env var names are also case-insensitive on Windows, but the function's map keys case-sensitively. Low impact (affects relative-path resolution per drive in the child process only) but worth fixing if that ever matters.

Fixed (issue #63): command dispatch previously always terminated a dispatched line with `\n`, which submits under a Unix tty's line discipline but is delivered by ConPTY as a literal Ctrl+J rather than Enter — so a command run from the app used to appear fully typed at a Windows prompt and never execute. `shellQuoteDir`'s POSIX-only `cd '<dir>' && ` prefix was a second, stacked bug: Windows PowerShell 5.1 has no `&&` operator and cmd.exe doesn't parse single-quoted paths. Both are fixed by `shellDialectFor`/`buildCommandLine` (`executor.go`), which choose the line-submit key and cd-prefix syntax per shell dialect (POSIX/cmd.exe/PowerShell) — see `TestBuildCommandLine` and the `TestPwshIntegration_*` Windows tests in `shell_integration_test.go`.

## Local CI Verification (act)

`.github/workflows/ci.yml` and `release.yml` can be exercised locally with [nektos/act](https://github.com/nektos/act) before pushing. A repo-root `.actrc` maps `ubuntu-24.04`/`macos-latest`/`windows-latest` to `catthehacker/ubuntu` images — without it, act warns `Skipping unsupported platform` for `ubuntu-24.04` since it has no built-in mapping for that label.

**Platform caveat:** act only runs Linux containers. `macos-latest`/`windows-latest` jobs run against the Linux stand-in image, so `choco`, `pwsh`, `codesign`, `hdiutil`, `makensis` steps will fail or no-op there — use act for the `ubuntu-24.04` path and Linux-agnostic steps (checkout, Go/pnpm setup, `go build`, `pnpm tsc`); verify real macOS/Windows builds on those OSes directly.

On Apple Silicon, add `--container-architecture linux/amd64` if you hit image-manifest errors.

Both workflows need `-s GITHUB_TOKEN=<token>` (any PAT — used by `arduino/setup-task` to avoid GitHub API rate-limiting).

```bash
# ci.yml — typecheck job (Linux-only, fully runnable)
act push -W .github/workflows/ci.yml -j typecheck -s GITHUB_TOKEN=$GITHUB_TOKEN

# ci.yml — test job (Go tests + Playwright e2e, Linux-only, fully runnable)
act push -W .github/workflows/ci.yml -j test -s GITHUB_TOKEN=$GITHUB_TOKEN

# ci.yml — build-check, per matrix os
act push -W .github/workflows/ci.yml -j build-check --matrix os:ubuntu-24.04 -s GITHUB_TOKEN=$GITHUB_TOKEN
act push -W .github/workflows/ci.yml -j build-check --matrix os:macos-latest -s GITHUB_TOKEN=$GITHUB_TOKEN   # stand-in image only
act push -W .github/workflows/ci.yml -j build-check --matrix os:windows-latest -s GITHUB_TOKEN=$GITHUB_TOKEN # stand-in image only

# release.yml — simulate a tag push (release.yml has no branch trigger, only tags/workflow_dispatch)
cat > /tmp/act-tag-push.json <<'EOF'
{ "ref": "refs/tags/v0.0.0-test" }
EOF

# build job only (safe — no GitHub Release is created)
act push -W .github/workflows/release.yml -j build --matrix os:ubuntu-24.04 \
  -e /tmp/act-tag-push.json -s GITHUB_TOKEN=$GITHUB_TOKEN --artifact-server-path /tmp/act-artifacts
```

Do not run the whole `release.yml` (omitting `-j build`) with a repo-write-scoped `GITHUB_TOKEN` — the trailing `release` job calls `softprops/action-gh-release`, which will publish a real GitHub Release.

## References

- `CLAUDE.md` — The fullest agent-facing reference: architecture, execution flow, terminal/shell-integration internals, migration pattern, conventions
- `docs/` — GETTING-STARTED.md, DEVELOPMENT.md, ARCHITECTURE.md, CONFIGURATION.md, DEPLOYMENT.md, TESTING.md, API.md
