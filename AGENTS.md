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

# Go formatting + lint (not wired into `make check`; lint is blocking in CI —
# both this and `pnpm lint` fail the run on any finding)
make fmt                                # golangci-lint fmt (rewrites files: goimports + golines)
make lint                               # golangci-lint run (same config CI uses)

# Frontend lint + fix
cd frontend && pnpm lint                # pnpm lint:fix for auto-fix

# Go tests
go test ./...                           # go test -run TestFreshDBMigrations -v ./...

# Frontend e2e tests (Playwright)
cd frontend && pnpm test:e2e

# Go tests + frontend e2e tests together
make test
```

**Note:** The Makefile targets `make dev/build/generate/check/fmt/lint/test/clean` exist. `make clean` removes `bin/` (the real build output dir) and `frontend/dist/`, then restores the tracked `frontend/dist/.gitkeep` placeholder that `//go:embed all:frontend/dist` in `main.go` requires to exist. The Taskfile (`task dev`, `task build`) provides more options (Docker cross-compile, server mode). Both call `wails3` under the hood.

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

Each service is a struct implementing `ServiceStartup(ctx, options) error`. Wails generates TypeScript bindings from exported methods into `frontend/bindings/cmdex/<servicename>/`. **Never hand-edit `frontend/bindings/`** — it's generated output, but it **is** committed (so a fresh clone type-checks without the Wails CLI installed); regenerate with `wails3 generate bindings` and commit the result alongside the Go change that caused it.

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
2. Update SQL schema (add migration in `migrations/` package if persistent; or bump version + update `db.go` schema DDL).
3. Update `db.go` CRUD queries and scan helpers.
4. Update the service method signatures in the relevant `*_service.go` file.
5. Run `wails3 dev` to regenerate bindings.
6. Update TypeScript types in `frontend/src/types.ts`.
7. Update `CommandDetail.tsx` / `CommandDetailTab.tsx` or `CategoryEditor.tsx`.
8. Update `App.tsx` where the create/update calls are made.

### Event system

Events bridge Go and the frontend. Event names are defined in `event_service.go` (`EventNames` struct) and consumed in the frontend via `@wailsio/runtime`:

```ts
import { Events } from '@wailsio/runtime';
Events.On('cmd-output', handler);
```

Frontend fallback/initialization in `frontend/src/wails/events.ts`. Streaming execution output uses `cmd-output` events batched with `requestAnimationFrame`.

## Key Files & Responsibilities

| File | Purpose |
|---|---|
| `main.go` | Entry point, service registration, native menus, window config |
| `app.go` | App lifecycle (`ServiceStartup`/`Shutdown`), settings window management; `db` and `executor` are package-level vars initialized here |
| `command_service.go` | Category + Command CRUD bound methods |
| `execution_service.go` | `RunCommand`, `RunInTerminal`, `GetVariables`, execution history |
| `settings_service.go` | `GetSettings`, `SetSettings` |
| `importexport_service.go` | Import/export commands, theme templates |
| `event_service.go` | `GetEventNames` — exposes event name strings to frontend |
| `db.go` | SQLite access, schema DDL, migrations runner, FTS5 search, all SQL queries |
| `models.go` | Go domain types for Wails/JSON and SQL scanning |
| `script.go` | `{{var}}` parsing, shebang wrapping, template substitution (pure functions) |
| `executor.go` | Subprocess execution, temp scripts, CEL default evaluation, terminal detection/launch |
| `migrations/` | Versioned migration files (`0001_initial.go` … `0010_working_dir.go`), `migration.go` defines the ordered `Migrations` slice |
| `frontend/src/App.tsx` | Central state: tabs, modals (discriminated union `ModalState`), data loading, event subscriptions |
| `frontend/src/types.ts` | TS mirrors of Go domain types |
| `frontend/src/utils/tabDraft.ts` | Tab draft/baseline state and dirty comparison |
| `frontend/src/utils/templateVars.ts` | Variable detection and merging for UI |
| `frontend/src/hooks/useKeyboardShortcuts.ts` | Global keyboard shortcuts |

## Data & Storage

- SQLite at `~/.cmdex/cmdex.db`, opened with WAL + foreign keys enabled.
- Commands use `{{variableName}}` template syntax (double braces).
- Variables auto-detected from `{{var}}` patterns; can also be added manually.
- Variable defaults support CEL expressions: `now()`, `env("KEY")`, `date("2006-01-02")`.
- Scripts stored with `#!/bin/bash` shebang; editor shows the body without it.
- `commands.category_id` is nullable + `ON DELETE SET NULL` (deleting a category uncategorizes its commands).

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
- Per-tab execution output is stored in refs (`tabOutputRef`, `tabPaneStateRef`) not React state — using state causes re-render loops. Restore with `applyPaneState(tabId)` on tab switch.
- Themes use CSS variables in `frontend/src/style.css` — modify variables, not hardcoded colors.
- The output pane does not support interactive shells or full ANSI rendering.

## Tests

- Go: `db_test.go` — three migration tests (`TestFreshDBMigrations`, `TestExistingDBIdempotent`, `TestRollbackTo`). Run with `go test ./...`.
- Go: `terminal_service_test.go` + `terminal_service_stress_test.go` + `terminal_service_max_sessions_test.go` — multi-session CRUD, cwd inheritance, 100-cycle stress test, and MaxSessions limit test. Stress and max-sessions tests are `//go:build darwin` (use the mock backend from `pty_backend_mock_test.go`).
- **Windows conpty verification gap:** The conpty backend in `pty_backend_windows.go` is a stub that returns "not implemented" errors. Runtime conpty testing is not done in this milestone — see `.planning/phases/25-polish-integration/CHECKPOINT.md` for the full gap documentation.
- Frontend: Playwright e2e tests under `frontend/e2e/tests/*.spec.ts` (commands, categories, themes, smoke test). Run with `cd frontend && pnpm test:e2e`, or `make test` which runs `go test ./...` first and then the e2e suite. `frontend/e2e/mocks/runtime.ts` mocks `@wailsio/runtime` by hardcoding each generated binding's numeric `$Call.ByID` method hash — if you regenerate bindings and IDs shift, update this mock's handler table or tests fail silently with `[e2e mock] no handler for method ID …` console warnings.
- `.golangci.yml` (golangci-lint v2 config; disables `unused`, `forbidigo` — the latter's default fmt.Println/Printf ban conflicts with this codebase's logging convention — plus several complexity/style linters listed in the config's `disable:` block; `_test.go` files are excluded via `exclusions.paths`). Runs in CI's `typecheck` job via `golangci/golangci-lint-action` and **blocks the run on any finding** (same as `pnpm lint`) — currently 0 findings. `make lint` runs this exact config locally (`golangci-lint run`, no `|| true`, so it also fails on findings). `make fmt` (`golangci-lint fmt`) is a separate formatting-only command that rewrites files in place using the formatters configured under `.golangci.yml`'s `formatters:` block (`goimports`, `golines`).
- CI's `test` job runs `go test ./...` and the Playwright e2e suite on every push/PR and blocks the run on failure. `golangci-lint` and `pnpm lint` also block the `typecheck` job on any finding. `make check`/`make fmt`/`make lint` are local-only conveniences — CI does not invoke the Makefile directly.

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

- `docs/` — Human-authored docs: GETTING-STARTED.md, DEVELOPMENT.md, ARCHITECTURE.md, CONFIGURATION.md, DEPLOYMENT.md, TESTING.md, API.md
- `.planning/codebase/` — Auto-generated analysis: ARCHITECTURE.md, STACK.md, CONVENTIONS.md, CONCERNS.md, INTEGRATIONS.md, STRUCTURE.md, TESTING.md
- `CLAUDE.md` — Additional agent context (some sections may lag behind Wails v3 migration)
