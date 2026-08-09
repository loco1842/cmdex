# CLAUDE.md

## Project Overview

Cmdex is a cross-platform desktop app for saving, organizing, and executing CLI commands as bash scripts with dynamic variable arguments. Built with Go + Wails v3 (backend/desktop) and React + TypeScript + Vite (frontend).

Data is stored locally in a SQLite database at `~/.cmdex/cmdex.db` using `modernc.org/sqlite` (pure Go, no CGo).

## Prerequisites

- Go 1.26+ (see `go.mod`)
- Wails v3 CLI, pinned to `v3.0.0-beta.5` (see `.github/workflows/ci.yml`'s `WAILS_VERSION`): `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.5`
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
make check                # runs: go build ./... && cd frontend && pnpm tsc --noEmit

# Go formatting/lint (not wired into `make check`; lint is blocking in CI)
make fmt                  # golangci-lint fmt (rewrites files: goimports + golines)
make lint                 # golangci-lint run (same config as CI; fails on findings)

# Clean build artifacts
make clean                # removes bin/ and frontend/dist/, then restores the
                           # tracked frontend/dist/.gitkeep placeholder that
                           # //go:embed all:frontend/dist in main.go requires

# Frontend dependencies (pnpm required)
cd frontend && pnpm install
```

**Tests exist:** Go tests (`db_test.go`, `terminal_service_test.go` + stress/max-sessions variants) run via `go test ./...`; frontend Playwright e2e tests run via `cd frontend && pnpm test:e2e`. `make test` runs both (Go tests, then the e2e suite), and CI's `test` job runs the same two steps directly and gates the run on failure. `make check` and CI's `typecheck` job run `go build ./...` + `pnpm tsc --noEmit` as a separate, faster gate.

## Architecture

### Wails Bindings (Go <-> Frontend)

Seven services are registered as `application.Service` in `main.go` — not a single monolithic `App` struct. Wails auto-generates TypeScript bindings per service under `frontend/bindings/cmdex/<servicename>.js` (**not** `frontend/wailsjs/`, which doesn't exist in this v3 project). To add a backend feature:
1. Add a method to the relevant service struct (or create a new service and register it in `main.go`'s `Services` slice)
2. Run `wails3 generate bindings` (or `wails3 dev`, which regenerates automatically) to regenerate bindings
3. Import the generated function in React: `import { SomeMethod } from '../bindings/cmdex/servicename'` (path is depth-relative to the importing file)

### Backend (Go)

- **`main.go`** - Entry point: `application.New(...)`, service registration, native menu setup, main window config
- **`app.go`** - `App` service: lifecycle (`ServiceStartup`/`ServiceShutdown`), settings window management, `GetOS`, `PickDirectory`; holds package-level `db`/`executor`/`wailsApp` vars
- **`command_service.go`** - `CommandService`: CRUD for categories/commands/presets, search
- **`execution_service.go`** - `ExecutionService`: `RunCommand`, `GetVariables`, execution history
- **`settings_service.go`** - `SettingsService`: `GetSettings`/`SetSettings`
- **`importexport_service.go`** - `ImportExportService`: import/export commands, theme templates
- **`event_service.go`** - `EventService`: exposes event name constants to the frontend
- **`terminal_service.go`** + **`pty_backend*.go`** - `TerminalService`: multi-session PTY-backed terminals
- **`models.go`** - Data types: `Category`, `Command`, `VariableDefinition`, `VariablePreset`, `ExecutionRecord`, `AppSettings`
- **`db.go`** - SQLite database layer with FTS5 full-text search, normalized tables (categories, commands, tags, variable_definitions, variable_presets, executions, app_settings); migrations live in the `migrations/` package
- **`script.go`** - Script generation (`GenerateScript`), body extraction (`ParseScriptBody`), `{{var}}` template extraction (`ExtractTemplateVars`), replacement (`ReplaceTemplateVars`), and variable merging (`MergeDetectedVars`)
- **`executor.go`** - Script execution via temp files (`os/exec`), streaming output via `wailsApp.Event.Emit` (e.g. `pty-output:<id>`, `pty-exit:<id>`), terminal emulator detection/launch, CEL expression evaluation for variable defaults (supports `now()`, `env()`, `date()`)

Scripts are stored with `{{variableName}}` template placeholders. At execution time, `{{var}}` placeholders are replaced with user-provided values, written to a temp file, and executed with `bash <tmpfile>`. Variables are auto-detected from `{{var}}` patterns in the script body and can also be added manually.

Platform-aware shell: uses `$SHELL -lc` on Unix (falls back to `/bin/sh`), `cmd /C` on Windows.

### Frontend (React + TypeScript)

- **`App.tsx`** - Central state management; all modals controlled via discriminated union `ModalState` type
- **`types.ts`** - TypeScript interfaces mirroring Go models in `models.go`
- **`i18n.ts`** - i18next setup; translations in `src/locales/en.json`
- **UI components** in `src/components/`: `Sidebar`, `CommandDetail`, `TabBar`, `CategoryEditor`, `VariablePrompt`, `HistoryPane`, `OutputPane`, `SettingsDialog`, plus inline-edit helpers (`InlineEditField`, `HoverActionButton`, `FloatingSaveBar`)
- **Tab system**: Editor uses tab-based interface (not modals). Each command opens in a tab with dirty state tracking. See **Tab-Based Editor Architecture** below
- **shadcn/ui components** in `src/components/ui/` (Radix UI + Tailwind CSS + CVA)
- Styling: Tailwind CSS v4 with custom CSS variables in `style.css` for the dark theme (`--bg-primary`, `--accent-primary`, etc.)

Streaming output: Go emits `cmd-output` Wails events -> frontend buffers with `requestAnimationFrame` -> renders in `OutputPane`.

### Adding a New Field to Command or Category

1. Update struct in `models.go`
2. Update schema and queries in `db.go`
3. Update method signatures in the relevant `*_service.go` file
4. Run `wails3 generate bindings` to regenerate bindings
5. Update TypeScript interfaces in `frontend/src/types.ts`
6. Update the relevant UI (`CommandDetail.tsx` or `CategoryEditor.tsx`)
7. Update calls in `App.tsx`

### Tab-Based Editor Architecture

The editor uses a tab-based interface (replaced modal `CommandEditor`):
- **Tab identification**: Welcome tab uses `'__welcome__'` literal ID; new-command tabs use a generated prefix checked by `isNewCommandTabId()` from `types.ts`; saved commands use their DB ID
- **Dirty state**: Each tab tracks `isDirty` for unsaved changes; visual indicator shows dot on tab
- **State management**: Tabs stored in array; `activeTabId` controls focus; each command tab holds draft + baseline in `App` for inline editing and batch save
- **Per-tab output + pane state**: `tabOutputRef` and `tabPaneStateRef` are refs (not state) that persist each tab's output record, stream lines, and pane visibility across tab switches — use `applyPaneState(tabId)` to restore them; never replace with React state or re-renders will loop
- **Adding a new tab type**: Add an ID pattern/constant, handle it in `handleSelectTab`, `openTab`, and tab rendering logic; initialize its `tabPaneStateRef` entry on open; clean up in `finalizeCloseTab`

### Preset & Variable UX Patterns

**Preset Management** (inline editing in `CommandDetail`):
- Presets display as chips with context menu (rename, delete)
- "+" chip creates empty preset with immediate name edit mode
- Variables render as card rows with name label and value input
- **Keyboard navigation**: Tab/Shift+Tab between variable inputs; Enter saves; Escape cancels
- **Visual feedback**: Focused variable highlighted in command preview; TEMPLATE badge on placeholder view

**Variable Preview System**:
- Dual-preview: Template (with `{{var}}` placeholders) + Resolved (with actual values)
- Focus highlight follows editing variable; `var-focused` CSS class for outline
- Preset save: Check icon button in Preview header; auto-save on Enter

## Key Design Decisions

- Commands use `{{variableName}}` template syntax for variables (e.g., `echo "Hello {{name}}"`)
- Variables are auto-detected from `{{var}}` patterns in the script body, and can also be added manually
- At execution time, `{{var}}` placeholders are replaced with user values via simple string substitution
- Variable defaults support CEL expressions (Google Common Expression Language) with custom functions: `now()`, `env("KEY")`, `date("2006-01-02")`
- Scripts are stored with a `#!/bin/bash` shebang; the editor shows/edits the body without the shebang
- Commands can have named presets (saved sets of variable values)
- SQLite with FTS5 for full-text search on title, description, and script content
- Foreign keys with ON DELETE CASCADE for referential integrity
- Exception: `commands.category_id` uses `ON DELETE SET NULL` (deleting a category uncategorizes its commands rather than deleting them)
- Theme colors use CSS variables - modify those rather than hardcoding colors
- The output panel does not support interactive shells or full ANSI color rendering
- `time.Time` fields (e.g. `CreatedAt`, `ExecutedAt`) generate as plain `string`-typed fields in the bindings — no separate `time` package output, no warnings expected

## Gotchas

- `category_id` in `commands` table is nullable (`NULL` = uncategorized). Use `nullableString()` helper when inserting/updating, and `sql.NullString` when scanning.
- After changing Go structs or method signatures, delete `~/.cmdex/cmdex.db` if schema changed, or add a new migration in the `migrations/` package (see Schema Migration Pattern below) — there is no `schemaVersion` constant in `db.go`; the applied version lives in the `schema_version` table and is driven by the ordered `Migrations` slice in `migrations/migration.go`
- Schema migrations must recreate tables (SQLite doesn't support `ALTER COLUMN`) — see the migration pattern below
- Schema migrations must be wrapped in transactions to prevent partial failures leaving DB inconsistent
- `//go:embed all:frontend/dist` in `main.go` requires `frontend/dist` to exist and be non-empty, or `go build`/`go vet`/`go test`/`wails3 build` all fail with `pattern all:frontend/dist: no matching files found`. A tracked `frontend/dist/.gitkeep` placeholder keeps this working on a fresh checkout; `make clean` deletes and restores it. `wails3 build`/`wails3 dev` build the real frontend first so this only bites bare `go build`/`go test` invocations.
- When changing script storage format, delete `~/.cmdex/cmdex.db` to reset
- `RenameCommand` is a separate metadata-only DB method — don't re-process scripts through `UpdateCommand` just to change the title

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
- Handle data transformation (e.g., empty string → NULL) in the migration.

<!-- GSD:project-start source:PROJECT.md -->
## Project

**Cmdex v2 — Premium Release**

Cmdex is a cross-platform desktop app for saving, organizing, and executing CLI commands as bash scripts with dynamic variable arguments. Built with Go + Wails v3 (backend) and React + TypeScript + Vite (frontend), with SQLite local storage. This milestone adds workspace management, cloud sync/sharing, user-defined themes, and a comprehensive UI/UX overhaul to transform Cmdex into a premium command management tool.

**Core Value:** Users can organize commands by project context, sync them across devices, and share them with the community — all in a clean, customizable interface.

### Constraints

- **Tech stack**: Must use Cloudflare services for cloud backend — user preference for Cloudflare ecosystem
- **Auth**: OAuth-only (Google/GitHub) — no email/password flows
- **Desktop framework**: Wails v3 — existing investment, not migrating away
- **Frontend**: React + TypeScript + Tailwind — existing stack, not changing
- **Data**: SQLite remains local data store — cloud sync is additive, not replacement
<!-- GSD:project-end -->

<!-- GSD:stack-start source:codebase/STACK.md -->
## Technology Stack

## Languages
- **Go** (toolchain `1.26.0` per `go.mod`) — Wails backend, SQLite access, script execution, CEL evaluation, OS terminal integration (`main.go`, `app.go`, `db.go`, `executor.go`, `script.go`, `models.go`, plus `*_service.go` service files and the `migrations/` package).
- **TypeScript** (`typescript` `^5.9.3` in `frontend/package.json`) — React UI and Wails-generated bindings under `frontend/bindings/`.
- **JSON** — i18n resources (`frontend/src/locales/en.json`).
- **CSS** — Tailwind v4–driven styling via `frontend/src/style.css` and `@tailwindcss/vite`.
## Runtime
- **Go** `1.26.0` — declared in `go.mod`; CI (`.github/workflows/ci.yml`, `release.yml`) resolves the toolchain dynamically via `go-version-file: go.mod`, not a hardcoded version.
- **Node.js** — CI uses `NODE_VERSION: '24'` (see `.github/workflows/ci.yml`, `.github/workflows/release.yml`); no `.nvmrc` in repo.
- **pnpm** — frontend installs (`frontend/package.json`, `frontend/pnpm-lock.yaml`); driven by `build/Taskfile.yml`'s `PACKAGE_MANAGER` var, not `wails.json` (a stale, unused Wails v2 config file).
- Lockfile: `frontend/pnpm-lock.yaml` (present).
## Frameworks
- **Wails v3** (`github.com/wailsapp/wails/v3` `v3.0.0-beta.5` in `go.mod`) — desktop shell via `application.New(...)`, asset server embedding `frontend/dist` (`//go:embed all:frontend/dist` in `main.go`), Go↔JS bindings (`wails3 generate bindings` → `frontend/bindings/cmdex/<servicename>.js`).
- **React** `^19.2.7` + **React DOM** `^19.2.7` — UI in `frontend/src/` (entry `frontend/src/main.tsx`, root `frontend/src/App.tsx`).
- **Vite** `^8.0.16` with `@vitejs/plugin-react` `^5.2.0` — dev server and production bundle (`frontend/vite.config.ts`), plus the `@wailsio/runtime/plugins/vite` bindings plugin.
- Go: `db_test.go`, `terminal_service_test.go` (+ stress/max-sessions variants, `//go:build darwin`) via `go test ./...`. Frontend: Playwright e2e under `frontend/e2e/` via `pnpm test:e2e`. `Makefile` `check` runs only `go build ./...` + `pnpm tsc --noEmit` — tests are a separate step.
- **Wails CLI** — `wails3 dev`, `wails3 build`, `wails3 generate bindings` (documented in `Makefile`, `AGENTS.md`).
- **TypeScript compiler** — `frontend` build script: `tsc && vite build` (`frontend/package.json`).
- **GitHub Actions** — plain `actions/checkout`/`actions/setup-go`/`actions/setup-node`/`pnpm/action-setup` steps plus a manually `go install`ed `wails3` CLI and `arduino/setup-task` for Taskfile-driven builds; matrix over `ubuntu-24.04`/`macos-latest`/`windows-latest` (`.github/workflows/ci.yml`, `.github/workflows/release.yml`). No third-party "wails" GitHub Action is used.
## Key Dependencies
- `github.com/wailsapp/wails/v3` `v3.0.0-beta.5` — application framework and desktop integration (webview2 is folded into this module as of beta.5, no longer a separate dependency).
- `modernc.org/sqlite` `v1.47.0` — pure-Go SQLite driver (imported in `db.go` as `_ "modernc.org/sqlite"`).
- `github.com/google/cel-go` `v0.27.0` — CEL for variable default expressions in `executor.go`.
- `github.com/google/uuid` `v1.6.0` — ID generation for domain entities.
- `github.com/creack/pty` — PTY backend for `TerminalService` (`terminal_service.go`, `pty_backend_unix.go`).
- `radix-ui` / `@radix-ui/react-slot` — accessible primitives (shadcn-style components under `frontend/src/components/ui/`).
- `@tailwindcss/vite` + `tailwindcss` `^4.3.1` — styling pipeline.
- `i18next` + `react-i18next` — localization wired in `frontend/src/i18n.ts`.
- `@dnd-kit/core`, `@dnd-kit/sortable`, `@dnd-kit/utilities` — drag-and-drop in the UI.
- `@xterm/xterm` + addons — terminal emulator rendering for `TerminalService` sessions.
- `cmdk` — command palette patterns.
- `sonner` — toasts.
- `lucide-react` — icons (aligned with `frontend/components.json` `iconLibrary`).
- Wails v3 pulls platform helpers (webview2 internally on Windows, `godbus` on Linux, `coder/websocket`) transitively via `go.mod`, not referenced directly from app packages.
## Configuration
- No `VITE_*` or `process.env` usage detected in `frontend/src` TypeScript/TSX.
- Runtime data path is derived from `os.UserHomeDir()` in `db.go` (not environment variables).
- **Wails:** `build/config.yml` — app metadata, dev-mode file watching; `wails.json` is a stale, unread Wails v2 leftover.
- **Vite:** `frontend/vite.config.ts` — React + Tailwind plugins, path alias `@` → `frontend/src`, Wails bindings plugin.
- **TypeScript:** `frontend/tsconfig.json` — `strict: false`, `paths` `@/*` → `./src/*`; `frontend/tsconfig.node.json` for tooling.
- **UI generator metadata:** `frontend/components.json` — shadcn schema, New York style, aliases for `@/components`, `@/lib`.
- **Go lint:** `.golangci.yml` (golangci-lint v2 config, `forbidigo` disabled — conflicts with this codebase's `fmt.Println` logging convention). Runs in CI's `typecheck` job via `golangci/golangci-lint-action` and blocks the run on any finding (same as `pnpm lint`); not wired into `make check`. Run the same config locally with `make lint`.
## Platform Requirements
- Go matching `go.mod` (1.26.x).
- Wails v3 CLI, pinned to `v3.0.0-beta.5` (see `docs/GETTING-STARTED.md`).
- Node.js 24.x (matches CI) and pnpm.
- macOS / Linux / Windows for full parity with terminal-launch paths in `executor.go` (Windows `conpty` backend is currently a stub — see `AGENTS.md` Tests section).
- Desktop targets built via Wails; artifacts produced by local `wails3 build`/`task package` or the GitHub Actions release workflow (`.github/workflows/release.yml`).
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

## Naming Patterns
- Application code lives in package `main` at the repo root: `app.go`, `db.go`, `executor.go`, `main.go`, `models.go`, `script.go`.
- Exported types and methods use **PascalCase** (Wails-bound API surface): e.g. `Category`, `Command`, `GetCategories`, `CreateCommand` in `app.go`.
- Unexported helpers use **camelCase**: e.g. `writeTempScript` in `executor.go`.
- JSON field names for Wails/frontend use **camelCase** struct tags on models in `models.go` (e.g. `json:"categoryId"`, `json:"scriptContent"`).
- Root entry: `frontend/src/main.tsx`, app shell `frontend/src/App.tsx`.
- Feature components: **PascalCase** filenames under `frontend/src/components/` (e.g. `CommandDetail.tsx`, `TabBar.tsx`).
- shadcn-style UI primitives: **kebab-case** under `frontend/src/components/ui/` (e.g. `alert-dialog.tsx`, `toggle-group.tsx`).
- Hooks: **camelCase** with `use` prefix in `frontend/src/hooks/` (e.g. `useKeyboardShortcuts.ts`).
- Shared utilities: `frontend/src/utils/` (`tabDraft.ts`, `templateVars.ts`).
- Library helpers: `frontend/src/lib/` (`utils.ts`, `shortcuts.ts`).
- Global types: `frontend/src/types.ts`; i18n setup: `frontend/src/i18n.ts`.
- Interfaces and component names: **PascalCase** (`Command`, `Category`, `ModalState` in `App.tsx`).
- Functions and variables: **camelCase** (`getCommandDisplayTitle`, `emptyDraft`, `isNewCommandTabId`).
- Constants: **SCREAMING_SNAKE_CASE** where used for shortcut registry keys in `frontend/src/lib/shortcuts.ts` (re-exported via `useKeyboardShortcuts`).
## Code Style
- Standard **gofmt** layout (tabs for indentation). `.golangci.yml` (golangci-lint v2 config) blocks CI's `typecheck` job on findings and locally via `make lint` (`golangci-lint run`, no `|| true`). Not wired into `make check`. `make fmt` (`golangci-lint fmt`) is a separate formatting command that rewrites files using the formatters configured under `.golangci.yml`'s `formatters:` block (`goimports`, `golines`). No `.editorconfig` detected.
- Module path: `cmdex` in `go.mod`; Go **1.26.0** as declared there.
- **TypeScript** with `frontend/tsconfig.json`: `strict` is **false**; `forceConsistentCasingInFileNames` is true.
- **Mixed punctuation style**: root `App.tsx` and many components use semicolons and often single quotes; `frontend/src/components/ui/button.tsx` and `frontend/src/lib/utils.ts` use double quotes and omit semicolons. New code should match the file you are editing.
- **Build**: `frontend/package.json` runs `tsc && vite build` for production; type-check only via `pnpm tsc --noEmit` (see `Makefile` `check` target).
- Tailwind CSS v4 via `@tailwindcss/vite` in `frontend/vite.config.ts`; theme tokens live in `frontend/src/style.css` (CSS variables for dark theme).
## Import Organization
- `@/*` maps to `frontend/src/*` in both `frontend/tsconfig.json` and `frontend/vite.config.ts`.
- Standard library first, then blank line, then third-party (e.g. `github.com/google/uuid`, `github.com/wailsapp/wails/v3/...`, `modernc.org/sqlite` side-effect in `db.go`), then a final blank line before local `cmdex/...` imports (e.g. `cmdex/migrations` in `db.go`).
## Error Handling
- Wails startup failures: `wailsruntime.LogFatal` in `app.go` `startup` when DB init fails.
- Many read-style bound methods **log with `fmt.Println` and return empty slices** on DB error (e.g. `GetCategories`, `GetCommands` in `app.go`) instead of surfacing `error` to the frontend.
- Mutations and creates typically **return `(T, error)` or `error`** and propagate DB errors (e.g. `CreateCategory`, `DeleteCategory`).
- Executor paths: return `ExecutionResult` with `Error` string and exit code (e.g. `ExecuteScript` in `executor.go`).
- Async Wails calls use `.then` / `.catch` or try/catch patterns in UI code; user feedback often via `toast` from `sonner` (see imports in `App.tsx`).
## Logging
## Comments
- Section banners in `app.go` (`// ========== Category Operations ==========`) organize large binding files.
- Doc comments on exported Go symbols where relevant (e.g. `// App struct holds application state`).
- JSDoc-style blocks on non-obvious hooks (e.g. `useKeyboardShortcuts` behavior in `frontend/src/hooks/useKeyboardShortcuts.ts`).
## Function Design
- `App` methods are thin orchestration over `DB` and `Executor`; SQL and migrations concentrated in `db.go`.
- Script/template logic in `script.go`; CEL evaluation in `executor.go`.
- Large central state and tab/modal logic in `App.tsx`; presentational and focused logic in components under `frontend/src/components/`.
- Custom hooks encapsulate keyboard and shortcut concerns (`frontend/src/hooks/useKeyboardShortcuts.ts`).
## Module Design
- React components: default export for page-level components (`Sidebar`, `CommandDetail`, etc.); named exports where multiple symbols are shared (e.g. `Tab` from `TabBar.tsx`).
- `frontend/src/lib/utils.ts` exports `cn` for class merging (shadcn pattern).
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

## Pattern Overview
- **Service-per-concern backend:** Seven structs (`App`, `CommandService`, `ExecutionService`, `SettingsService`, `ImportExportService`, `EventService`, `TerminalService`) are each registered as an `application.Service` in `main.go` — no single monolithic `App` orchestrates everything.
- **Flat `package main` at the repo root**, plus a separate `migrations` package: domain logic, persistence, execution, and script helpers live across the `*_service.go` files, `db.go`, `executor.go`, `script.go`, `models.go`, `pty_backend*.go`.
- **Centralized React state:** `frontend/src/App.tsx` owns lists, tabs, modals, execution/output state, and coordinates child components.
- **Push streaming for stdout/stderr:** Execution uses Go callbacks that emit Wails events (`wailsApp.Event.Emit`, e.g. `pty-output:<id>`); the frontend subscribes via `@wailsio/runtime`'s `Events.On` and batches lines for rendering.
## Layers
- Purpose: Create the app via `application.New(...)`, register all 7 services, configure native menus, embed `frontend/dist`, run with `app.Run()`.
- Location: `main.go`
- Contains: `//go:embed all:frontend/dist`, `application.New(application.Options{Services: [...]})`, menu callbacks that emit `open-settings`/`open-shortcuts`.
- Depends on: `application`/`events` packages (`github.com/wailsapp/wails/v3/pkg/...`), all 7 service structs
- Used by: OS / Wails runtime only
- Purpose: App-level lifecycle service — settings window management, `GetOS`, `PickDirectory`; holds package-level `db`/`executor`/`wailsApp` vars used by other services.
- Location: `app.go`
- Contains: `ServiceStartup`/`ServiceShutdown` (DB + executor init/teardown), `ShowSettingsWindow`, `GetOS`, `PickDirectory`
- Depends on: `DB` (`db.go`), `Executor` (`executor.go`), Wails `application` package for the settings `WebviewWindow`
- Used by: React via generated bindings (`frontend/bindings/cmdex/app.js`); `main.go` holds a reference for menu events
- Purpose: Category/command CRUD, presets, search — split out of the old monolithic `App` into `CommandService`, `ExecutionService`, `SettingsService`, `ImportExportService`, `EventService`.
- Location: `command_service.go`, `execution_service.go`, `settings_service.go`, `importexport_service.go`, `event_service.go`
- Contains: `GetScriptBody`/`GetScriptContent`, `GetVariables`/`RunCommand`, `GetSettings`/`SetSettings`, import/export, `GetEventNames`
- Depends on: `DB` (`db.go`), `Executor` (`executor.go`), `models.go`, `script.go` (`ParseScriptBody`, `ReplaceTemplateVars`, etc.), `wailsApp.Event.Emit` for events
- Used by: React via generated bindings under `frontend/bindings/cmdex/<servicename>.js`
- Purpose: SQLite access, schema, migrations, FTS5 search, CRUD for all entities.
- Location: `db.go`
- Contains: `DB` struct, `runMigrations()`/`RollbackTo()` (running the ordered `migrations.Migrations` slice), queries, `SearchCommands` against `commands_fts`.
- Depends on: `database/sql`, `modernc.org/sqlite`, `models.go` types
- Used by: `app.go` exclusively
- Purpose: Shebang wrapping/stripping, `{{var}}` detection, merge with manual variable definitions.
- Location: `script.go`
- Contains: `GenerateScript`, `ParseScriptBody`, `ExtractTemplateVars`, `ReplaceTemplateVars`, `MergeDetectedVars`
- Depends on: `models.go` (`VariableDefinition`)
- Used by: `app.go`, `db.go` (indirect via command handling), `executor.go` (`ReplaceTemplateVars` in display helpers), `models.go` (`Command.DisplayTitle`)
- Purpose: Temp-file script execution, streaming output, CEL default evaluation, optional external terminal launch.
- Location: `executor.go`
- Contains: `Executor`, `ExecuteScript` with chunk callback, `EvalDefaults` (CEL), `GetAvailableTerminals`, `OpenInTerminal`
- Depends on: `script.go`, `github.com/google/cel-go`, `os/exec`, platform `runtime`
- Used by: `app.go`
- Purpose: JSON-serializable structs for Wails and SQL scanning helpers (e.g. `sql.NullString` on `Command.Title`).
- Location: `models.go`
- Contains: `Category`, `Command`, `VariableDefinition`, `VariablePreset`, `VariablePrompt`, `ExecutionRecord`, `AppSettings`, `ExecutionResult`, `TerminalInfo`
- Depends on: `database/sql`, `time`, `script.go` for `DisplayTitle`
- Used by: `app.go`, `db.go`, `executor.go`, `script.go`
- Purpose: UI, tabbed editor, modals, keyboard shortcuts, i18n, calls to Go and event subscriptions.
- Location: `frontend/src/` (root component `App.tsx`)
- Contains: Feature components under `components/`, shared hooks (`hooks/useKeyboardShortcuts.ts`), utilities (`utils/tabDraft.ts`, `utils/templateVars.ts`), path alias `@/` → `src/`
- Depends on: Generated `frontend/bindings/cmdex/<servicename>.js`, `@wailsio/runtime` (`Events`, etc.), `i18next`, Radix/shadcn-style UI under `components/ui/`
- Used by: End user
- Purpose: JavaScript stubs (with JSDoc types) for Go service methods and Wails runtime models.
- Location: `frontend/bindings/cmdex/*.js`
- Regenerated by: `wails3 generate bindings` or `wails3 dev` (also happens automatically as part of `wails3 build`/`task build`)
## Data Flow
- **Global UI state:** React `useState` / `useRef` / `useCallback` in `App.tsx` — categories, commands, active tab, modal union `ModalState`, execution flags, stream buffers.
- **Per-tab output and pane layout:** Refs such as `tabOutputRef` and `tabPaneStateRef` in `App.tsx` (documented in `CLAUDE.md`) so tab switches do not lose output or pane visibility without forcing full re-render loops.
- **Theme:** `THEME_STORAGE_KEY` and theme list in `App.tsx`; persisted in browser storage for the webview.
- **i18n:** `frontend/src/i18n.ts` and locale JSON (e.g. `frontend/src/locales/en.json`).
## Key Abstractions
- Purpose: Sole Wails-bound surface for the frontend; holds `ctx`, `*DB`, `*Executor`.
- Examples: `app.go`
- Pattern: Facade over store + executor; no interface indirection in-repo.
- Purpose: Encapsulates connection string (WAL, foreign keys), migrations, and all SQL.
- Examples: `db.go`
- Pattern: Struct + methods; the ordered `migrations.Migrations` slice drives incremental migrations against the `schema_version` table.
- Purpose: Shell selection (Windows vs Unix), script file lifecycle, subprocess I/O, CEL evaluation for defaults, terminal detection.
- Examples: `executor.go`
- Pattern: Stateless executor type constructed once in `startup`.
- Purpose: `Command` is the persisted aggregate (script, tags, variables, presets). `TabDraft` in `utils/tabDraft.ts` represents editor draft + dirty comparison for tabs.
- Examples: `models.go`, `frontend/src/types.ts`, `frontend/src/utils/tabDraft.ts`
- Pattern: Baseline vs draft for unsaved-indicator and batch save.
- Purpose: Single discriminated union for dialogs (category editor, variable fill, confirm delete, settings, etc.).
- Examples: `ModalState` type in `frontend/src/App.tsx`
- Pattern: Type-safe modal switching without many boolean flags.
## Entry Points
- Location: `main.go`
- Triggers: Executable launch
- Responsibilities: `embed` frontend assets, `application.New(application.Options{Services: [...]})`, build native menu, `app.Run()`
- Location: `frontend/src/main.tsx`
- Triggers: Wails loading `index.html` / Vite dev server
- Responsibilities: `createRoot`, import `i18n` and `style.css`, render `<App />`
- Location: `build/config.yml`
- Triggers: Wails CLI (`wails3 dev`, `wails3 build`, `wails3 generate bindings`)
- Responsibilities: App metadata (name, identifier, version), dev-mode file watching config. (`wails.json` at the repo root is a stale Wails v2 file, not read by v3.)
## Error Handling
- `CreateCommand` / `UpdateCommand` / preset APIs return `(T, error)` to the frontend.
- `GetCategories`, `GetCommands`, `GetExecutionHistory`, `SearchCommands` swallow errors internally and return `[]` on failure (`app.go`).
- `RunCommand` always returns an `ExecutionRecord`; load failures set `Error` and negative `ExitCode` on the record.
## Cross-Cutting Concerns
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

| Skill | Description | Path |
|-------|-------------|------|
| add-shadcn-component | Add a new shadcn/ui component using the project's configured style and path aliases | `.claude/skills/add-shadcn-component/SKILL.md` |
| todo-scanner | "Scans the todos/ folder for *.pending.md files and creates implementation plans from their content using the writing-plans workflow. Use this skill whenever the user mentions checking todos, scanning pending tasks, reviewing todo files, processing pending items, or wants to plan work from markdown todo files. Also triggers on phrases like 'what's pending', 'any todos', 'plan my tasks', or 'work on todos'." | `.claude/skills/todo-scanner/SKILL.md` |
| wails-feature | Guide for adding new Wails-bound features across Go backend and React frontend, following the project's established data flow pattern | `.claude/skills/wails-feature/SKILL.md` |
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->

<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
