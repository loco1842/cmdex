# Architecture

## Overview

CmDex is a cross-platform desktop application for saving, organizing, and executing CLI commands with template variable support. It is built as a single-binary desktop app using **Wails v3**, which binds a **Go** backend to a **React 19 + TypeScript** frontend. All data is stored locally in a **SQLite** database with no external services or cloud dependencies.

The backend is service-oriented: discrete Wails v3 services expose domain operations (commands, execution, settings, import/export, terminals, and the launcher) to the frontend via auto-generated TypeScript bindings. The frontend is a single-page React application with a tab-based command editor, a searchable sidebar, a **real PTY-backed terminal panel**, and three native-window UIs: the main editor, settings, and global launcher.

The defining architectural fact: **commands are not run by a Go subprocess helper.** They are typed into a live pseudo-terminal, exactly as if the user had typed them. Everything about output handling, interactivity, and signal delivery follows from that.

---

## High-level Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                      CmDex Desktop App                        │
│  ┌────────────────────────────────────────────────────────┐  │
│  │              React 19 + Vite Frontend                  │  │
│  │  ┌───────────┐ ┌──────────────┐ ┌──────────────────┐   │  │
│  │  │  Sidebar  │ │ Command Tabs │ │ Terminal Panel   │   │  │
│  │  │ (Search)  │ │  (Editor)    │ │ (xterm.js, 1..N) │   │  │
│  │  └───────────┘ └──────────────┘ └──────────────────┘   │  │
│  └───────────────────────┬────────────────────────────────┘  │
│                          │ Wails v3 Runtime                   │
│                          │ (generated TS bindings + events)    │
│  ┌───────────────────────┴────────────────────────────────┐  │
│  │                     Go Backend                          │  │
│  │  ┌──────────┐ ┌───────────┐ ┌──────────┐ ┌───────────┐ │  │
│  │  │ Command  │ │ Execution │ │ Settings │ │  Import/  │ │  │
│  │  │ Service  │ │  Service  │ │ Service  │ │  Export   │ │  │
│  │  └────┬─────┘ └─────┬─────┘ └────┬─────┘ └─────┬─────┘ │  │
│  │  ┌────┴─────┐ ┌─────┴──────────────────────────┴─────┐ │  │
│  │  │  Event   │ │        TerminalService               │ │  │
│  │  │ Service  │ │  PTY sessions, capture, integration  │ │  │
│  │  └──────────┘ └───────────────┬──────────────────────┘ │  │
│  │  ┌───────────────┐            │                         │  │
│  │  │  App service  │      ┌─────┴──────┐                  │  │
│  │  │  (lifecycle)  │      │ OS shell   │                  │  │
│  │  └───────────────┘      │  via PTY   │                  │  │
│  │                          └────────────┘                  │  │
│  │                   DB Layer (SQLite)                      │  │
│  │                ┌─────────────────────┐                   │  │
│  │                │  ~/.cmdex/cmdex.db  │                   │  │
│  │                └─────────────────────┘                   │  │
│  └──────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

**Key architectural boundaries:**

- **Frontend** (`frontend/src/`) — React components, hooks, state. Talks to Go exclusively through Wails-generated bindings and runtime events.
- **Wails Runtime** — Bridges Go methods to TypeScript functions and provides the event bus (per-session PTY streams, settings sync, menu actions).
- **Backend Services** (`*.go` at the repo root) — Domain services registered with Wails; each exposes methods callable from the frontend.
- **Database Layer** (`db.go`) — Pure-Go SQLite via `modernc.org/sqlite`, with FTS5 full-text search and transactional migrations.

---

## Backend Layer

### Application Lifecycle (`app.go`)

The `App` struct manages application lifecycle and the secondary settings window.

- **`ServiceStartup`** — Initializes the package-level `db` and `executor` instances when Wails starts.
- **`ServiceShutdown`** — Closes the database connection on quit.
- **`ShowSettingsWindow`** — Opens a secondary native window (`/?window=settings`) lazily; it is a singleton and is destroyed on close, emitting `settings-window-closing` so the backend can nil out its reference.
- **`GetOS`** / **`PickDirectory`** — OS identity for `OSPathMap` keys, and the native directory picker.

`app.go` also owns the package-level `db`, `executor`, `terminalSvc`, and `wailsApp` variables that every other service reads. There is no dependency injection.

### Services

CmDex registers **eight** Wails v3 services in `main.go`:

| Service | File | Responsibility |
|---------|------|----------------|
| `App` | `app.go` | Lifecycle, settings window, `GetOS`, `PickDirectory` |
| `CommandService` | `command_service.go` | CRUD for categories, commands, variable presets, reordering, FTS search, `ResetAllData` |
| `ExecutionService` | `execution_service.go` | Variable resolution (CEL defaults), working-directory resolution, `RunCommand` |
| `SettingsService` | `settings_service.go` | Read/write user preferences (a single JSON blob) |
| `ImportExportService` | `importexport_service.go` | Export/import commands as JSON, save theme templates |
| `EventService` | `event_service.go` | Exposes event name constants so both sides use the same strings |
| `TerminalService` | `terminal_service.go` | Multi-session PTY terminals, output capture, shell integration |
| `LauncherService` | `launcher_service.go` | Global launcher window, shortcut registration, launch-at-login, and its internal terminal session |

### Database (`db.go`)

- **Engine:** SQLite via `modernc.org/sqlite` (pure Go, no CGO).
- **Location:** `~/.cmdex/cmdex.db`, opened with WAL journal mode and foreign keys enabled.
- **Schema version:** 10. There is no `schemaVersion` constant — the applied version lives in the `schema_version` table and is driven by the ordered `migrations.Migrations` slice.
- **Search:** an FTS5 virtual table (`commands_fts`) covers title, description, and script content, kept in sync by SQLite triggers. Search falls back to `LIKE` if FTS5 fails.
- **Rollback:** `RollbackTo(version)` walks migrations' `Down` functions in reverse (used by `TestRollbackTo`).

### Terminal Subsystem

This is the largest and most subtle part of the backend.

| File | Role |
|---|---|
| `terminal_service.go` | Session registry (up to `MaxSessions` = 10 user-visible sessions; internal launcher sessions excluded), per-session PTY + read-loop goroutine, `Write`/`Resize`/`Clear`/`Start`/`Stop` |
| `pty_backend.go` | The `ptyHandle` / `ptyProcess` interfaces the service is written against |
| `pty_backend_unix.go` | `creack/pty` implementation |
| `pty_backend_windows.go` | Real ConPTY implementation via `charmbracelet/x/conpty` |
| `pty_backend_mock.go` | In-memory backend used by stress/limit tests |
| `pty_env.go` | `buildPtyEnv` — injects `TERM`/`COLORTERM`/`LANG` that launchd-started GUI apps never inherit, and merges shell-integration env |
| `shell_integration.go` | Materializes the embedded `shell-integration/` scripts and activates OSC 133 markers |
| `terminal_capture.go` | Scans the raw byte stream for OSC 133 markers; backs `GetLastOutput` |
| `ansi.go` | ANSI/CSI stripping, plus removal of ConPTY's injected line-wrap artifacts |

Each session owns a `sessionState` guarded by its own mutex, and streams bytes to the frontend over **per-session** events:

- `pty-output:<sessionId>` — `{ data: string }`
- `pty-exit:<sessionId>` — `{ exitCode, wasIntentional }`
- `pty-cleared:<sessionId>` — no payload

**Why `buildPtyEnv` exists:** a GUI app launched from Finder or a mounted `.dmg` is started by launchd, which supplies no `TERM`. Without it, zsh's line editor has no terminfo entry and cannot emit cursor-movement sequences, so redraws from plugins like zsh-autosuggestions append instead of overwriting — the visible `llsll` corruption this fixes.

**Why `ansi.go` is more than a regex:** unlike a Unix PTY, where line wrapping is purely a client-rendering concern, ConPTY splits long lines with *real* CRLF bytes plus a cursor-reposition escape, and re-emits the boundary character (VT100 deferred-wrap behavior). Stripping only the escape leaves an injected newline mid-word — enough to turn copied JSON into invalid JSON. `removeWrapArtifacts` recognizes the full pattern and rejoins the text, deduplicating the boundary rune. It needs the session's true configured width to match anything.

`stripANSI` also collapses bare `\r` redraws per line via `collapseCarriageReturns`, keeping the **last non-empty** segment between carriage returns rather than unconditionally "everything after the final one" — a trailing `\r` with nothing written after it (a ConPTY repaint, or how PowerShell renders an error record) means nothing overwrote the line, so the content before it must survive. Naively keeping only the text after the last `\r` discarded the whole line in that case, which used to make "copy last output" return blank lines for a failed command on Windows instead of the actual error text.

### Shell Integration & Output Capture

Shell integration makes "copy last output" exact instead of scraping the terminal buffer.

1. `shell_integration.go` writes the embedded `shell-integration/` scripts (bash, zsh, fish, pwsh) into a subdirectory of `~/.cmdex` — shells read config from real paths, not memory. It **never edits the user's own dotfiles**; it points the shell at extra startup files (e.g. via `ZDOTDIR` for zsh).
2. Those scripts emit OSC 133 semantic-prompt markers: `C` immediately before a command's output, `D;<exit code>` immediately after.
3. `terminal_capture.go`'s `captureScan` watches the raw PTY stream for those markers and records the bytes between the most recent `C` and `D` as that session's last output, bounded by `maxCaptureBytes` (1 MiB, keeping the tail on overflow).
4. `TerminalService.GetLastOutput` returns `TerminalLastOutput{Available, Text, ExitCode, Truncated}`. `Available: false` means the session's shell has no integration, and the frontend falls back to scraping the xterm buffer.

**Marker forgery is prevented by a per-session nonce.** Each marker embeds a random token so a command that merely *prints* those bytes cannot fake a boundary. The token is passed as the *path to* a mode-0600 file (`oscNonceFileEnvVar`), never as an env var value: `unset`ing an exported variable only edits the shell's live view, and on Linux `/proc/<pid>/environ` keeps exposing the original block forever. The scripts read the file and delete it before sourcing any user profile or running any command, so no process the shell spawns can read it back. Code running *in-process* in the same shell (a sourced plugin, a shell function) can still see it — an inherent, accepted limit.

`//go:embed all:shell-integration` requires the `all:` prefix. A plain `//go:embed shell-integration` silently excludes files beginning with `.` or `_` — precisely zsh's `.zshenv`/`.zprofile`/`.zshrc` — which would hand zsh sessions an empty `ZDOTDIR` and silently disable integration.

Integration is toggled by `AppSettings.ShellIntegration` (nil = enabled). Changes affect **newly started sessions only**.

### Command Execution (`execution_service.go`)

`executor.go` is *not* an execution engine despite the name. It holds shell selection (`$SHELL -lc` on Unix falling back to `/bin/sh`; `cmd /C` on Windows), `stripShebang`, `shellQuoteDir`, `shellDialectFor`/`buildCommandLine`, and `EvalDefaults` (CEL). Nothing in the codebase writes temp scripts or spawns subprocesses.

`RunCommand(commandID, variables)` does the following:

1. Load the command, evaluate CEL defaults, and merge supplied variable values. A referenced required variable with neither a supplied value nor a default stops here; `RunCommand` returns an in-band `ExecutionRecord{ExitCode: -1, Error: ...}` before writing to the PTY. Unused variable definitions do not block execution.
2. Substitute `{{vars}}` via `ReplaceTemplateVars`, then `stripShebang` and trim trailing newlines. Placeholders with no definition remain unchanged because that low-level helper does not validate them.
3. Resolve the active session's shell (`SessionInfo.ShellPath`, falling back to `detectShell()` for a not-yet-started session) and the working directory (`""` if none is configured), then call `buildCommandLine(shellPath, script, workingDir)`.
4. `buildCommandLine` classifies the shell by base name into a dialect (POSIX / cmd.exe / PowerShell — see `shellDialectFor`) and composes the final line: an optional cd prefix syntactically correct for that dialect — POSIX `cd '<dir>' && ` (`shellQuoteDir`), cmd.exe `cd /d "<dir>" && `, or PowerShell `Set-Location -LiteralPath '<dir>' -ErrorAction Stop; ` (Windows PowerShell 5.1 has no `&&` operator; `-ErrorAction Stop` preserves short-circuit-on-failure) — followed by the script, with every line terminated by the dialect's actual submit key: `\n` for POSIX (a Unix pty's line discipline accepts it), `\r` for cmd.exe/PowerShell (ConPTY has no line discipline and delivers a bare LF as a literal Ctrl+J, not Enter — see issue #63). Write the finished line to the **active session's PTY** via `terminalSvc.Write`.

Consequences worth internalizing:

- Output is **not** captured by `RunCommand`; it streams over `pty-output:<id>` to xterm.js. The returned `ExecutionRecord` carries `FinalCmd` and errors only.
- **No execution history is persisted.** `db.go` still defines `GetExecutions`/`AddExecution`/`ClearExecutions` and the `executions` table still exists, but nothing in production calls them — they are referenced only from `execution_service_test.go`. Vestigial.
- Ctrl+C works naturally: the PTY has a real foreground process group.
- Interactive prompts, colors, and TUIs work, because it is a real terminal.
- With no active session, a nil `terminalSvc`, or a missing referenced required variable, `RunCommand` returns `ExecutionRecord{ExitCode: -1, Error: ...}` rather than writing to the PTY or rejecting the promise.

### Script Handling (`script.go`)

Pure functions, no I/O:

- **`GenerateScript`** — trims the body and adds a trailing newline. **It does not add a shebang.**
- **`ParseScriptBody`** — strips a leading `#!` line, so scripts saved by older versions (which did store `#!/bin/bash`) still edit cleanly.
- **`ExtractTemplateVars`** / **`ReplaceTemplateVars`** — `{{varName}}` detection (unique, in order of first appearance) and verbatim substitution. The helper leaves placeholders without values unchanged; `ExecutionService.resolveScript` evaluates defaults, rejects referenced required variables without a value before PTY dispatch, and ignores unused variable definitions. Command authors add shell quoting when a value must remain one word.
- **`MergeDetectedVars`** — merges auto-detected variables with manually defined ones: detected first in detection order, then manual-only variables with their relative order preserved, with metadata (description, default, example) carried over.

### Working Directory Resolution

`ExecutionService.resolveWorkingDir` walks a fallback chain and never returns empty:

1. Per-command `WorkingDir` for the current OS
2. Global `AppSettings.DefaultWorkingDir` for the current OS
3. `os.UserHomeDir()`
4. `os.Getwd()`
5. `os.TempDir()`

Note the deliberate asymmetry: `hasExplicitWorkingDir` decides whether to emit a `cd` prefix at all, so when neither the command nor settings specify a directory, the shell simply stays where it is instead of being pinned to `$HOME`.

---

## Frontend Layer

### Technology Stack

- **Framework:** React 19 with TypeScript (`strict: false`)
- **Build Tool:** Vite 8
- **UI Components:** shadcn/ui (Radix UI primitives + Tailwind CSS v4, New York style)
- **Terminal:** `@xterm/xterm` with the fit, web-links, and WebGL addons
- **State Management:** React `useState`/`useRef` — no external state library
- **I18n:** `react-i18next`
- **Notifications:** `sonner`
- **Drag & drop:** `@dnd-kit`

### Application Entry (`main.tsx`)

The frontend is a Vite-built SPA embedded by Wails. Assets load locally, so the build prefers a simple bundle (no vendor code-splitting) and only lazy-loads heavy entry points — `App` vs `SettingsPage`, and the xterm `Terminal`. Wails creates three native windows, all served by this same bundle, and the frontend renders one of three UIs based on the URL:

- **Main Window** (`/`, no query param) — lazily loads `<App />`
- **Settings Window** (`/?window=settings`) — lazily loads `<SettingsPage />`, which persists preferences independently and emits `settings-changed` back to the main window
- **Launcher Window** (`/?window=launcher`) — renders `<Launcher />` in a persistent, hidden-until-summoned window.

### Main App Structure (`App.tsx`)

`App.tsx` is the central orchestrator. It manages:

- **Data:** `categories`, `commands`, `selectedCommand`
- **Tabs:** `openTabs`, `activeTabId`, `tabDrafts`, `tabBaselines` (each mirrored into a ref via `useSyncedRef` so handlers read current values without re-subscribing)
- **Modals:** a discriminated union (`ModalState`) selects which dialog is open — category editor, variable prompt, discard confirmation, and so on
- **Terminal panel:** `sessions`, `activeSessionId`, `terminalOrderRef`, `terminalRefs`, plus resize/collapse state persisted to `localStorage` under `TERM_STORAGE_KEY`
- **Execution:** `executingTabIdRef`/`executingTabIdState` record which editor tab triggered the current run
- **Settings sync:** a `settingsRef` holds the latest values; the main window also listens for `settings-changed` from the settings window

The terminal panel is **shared, not per-tab**. It has its own session tabs, independent of which editor tab is focused.

### Key Components

| Component | Responsibility |
|-----------|----------------|
| `Sidebar` | Category tree, command list, drag-and-drop reordering, search |
| `CommandDetail` | Command editor (title, description, tags, script body, variables, presets) |
| `CommandDetailTab` | `React.memo` wrapper binding a `TabDraft` to `CommandDetail`; toggles `display` instead of unmounting |
| `TabBar` | Editor tab switching and dirty-state indicators |
| `Terminal` | xterm.js host; subscribes to that session's `pty-*` events |
| `TerminalTabBar` | Terminal session tabs (create, rename, close, activate) |
| `VariablePrompt` | Modal form for filling template variables before running |
| `CommandPalette` | Quick-search overlay |
| `Launcher` | Global quick-launcher UI rendered in its own window |
| `LauncherSettings` | Launcher enable, shortcut, and launch-at-login settings |
| `SettingsPage` / `SettingsDialog` | Theme, density, font, locale, working directory, shell integration |
| `ResizablePanel` | Collapsible/resizable panes |
| `WelcomeTab` | Empty-state tab (`'__welcome__'`) |
| `KeyboardShortcutsDialog` | Shortcut reference (`Cmd+?`) |

There is no `OutputPane` or `HistoryPane` — both were removed when execution moved into the PTY terminal.

### Hooks & Utilities

| Module | Purpose |
|---|---|
| `hooks/useKeyboardShortcuts.ts` | Single `keydown` capture listener over a `ShortcutMap`. Modifier combos fire regardless of focus; bare keys only outside editable elements. Uses a render-phase ref to avoid re-registering |
| `hooks/useResizable.ts` | Drag-to-resize with axis/min/max and `localStorage` persistence |
| `hooks/useSyncedRef.ts` | Returns a ref whose `.current` tracks the latest value during render — removes `useEffect`-to-ref plumbing |
| `hooks/useCopyToClipboard.ts` | `{ copied, copy }`, auto-resetting after a configurable delay |
| `utils/tab.ts` | `isNewCommandTabId()` (`__new_` prefix), `createNewTabId()`, `getCommandDisplayTitle()` |
| `utils/tabDraft.ts` | `draftFromCommand()`, `draftsEqual()` (dirty detection), `cloneDraft()`, `makePlaceholderCommand()` |
| `utils/templateVars.ts` | Variable detection/merging for the UI |
| `utils/path.ts` | `normalizeOS()`, `getOSPath()`/`setOSPath()`, `shortenPath()` |
| `utils/clipboard.ts` | Clipboard write helper |
| `lib/theme-apply.ts` | `applyTheme`/`applyDensity`/`applyFonts` write CSS custom properties and `data-*` attributes on `<html>` |
| `lib/shortcuts.ts` | Central `SHORTCUTS` registry plus platform-aware labels (`⌘ Enter` vs `Ctrl+Enter`) |

---

## Data Flow

### Wails Bindings

Wails v3 generates bindings for every exported method on a registered service. `wails3 generate bindings` (also run by `wails3 dev` and `wails3 build`) writes them to `frontend/bindings/cmdex/`.

**Example flow:**

1. Go method: `func (s *CommandService) GetCommands() []Command`
2. Wails generates `GetCommands()` in `frontend/bindings/cmdex/commandservice.js`
3. Frontend imports: `import { GetCommands } from '../bindings/cmdex/commandservice'`
4. Calling it marshals the request, invokes Go, and returns a `CancellablePromise` resolving to the typed result

`frontend/bindings/` is generated but **committed**, so a fresh clone type-checks without the Wails CLI. Never hand-edit it; regenerate and commit alongside the Go change.

### Runtime Events

**Named events** (from `EventNames`, fetched once via `GetEventNames()`):

- **`settings-changed`** — emitted by the frontend after a preference write; the main window updates theme, fonts, density, and locale live.
- **`open-settings`** / **`open-shortcuts`** — from the native menu bar.
- **`settings-window-closing`** — emitted by `app.go` when the settings window closes, so the backend can drop its reference.

**Per-session terminal events** — `pty-output:<id>`, `pty-exit:<id>`, `pty-cleared:<id>`. These are built by string concatenation in `terminal_service.go` and are *not* part of `EventNames`, so subscription happens per session ID.

> Earlier revisions of this document described a `cmd-output` event carrying `{ stream, data }` chunks. It no longer exists; output flows through the PTY session events above.

### Command Execution Flow

```
User presses Run (⌘/Ctrl+Enter)
        │
        ▼
┌────────────────────┐
│   VariablePrompt   │  ← if the command has {{vars}}, user fills values
│     (modal)        │     (defaults pre-evaluated by GetVariables → CEL)
└─────────┬──────────┘
          │
          ▼
┌──────────────────────────────────────────────┐
│ ExecutionService.RunCommand(id, variables)   │
│   1. load command from DB                     │
│   2. resolveScript: defaults, validation,      │
│      ReplaceTemplateVars → stripShebang       │
│   3. buildCommandLine(shellPath, script, wd)  │
│      → shell-dialect cd prefix + submit key   │
│   4. terminalSvc.Write(activeSession, line)   │
└─────────┬────────────────────────────────────┘
          │
          ▼
┌────────────────────┐
│  PTY (real shell)  │  ← shell executes it as if typed
└─────────┬──────────┘
          │  raw bytes, read loop
          ▼
┌────────────────────┐        ┌──────────────────────────┐
│ pty-output:<id>    │───────▶│ Terminal.tsx → xterm.js  │
└─────────┬──────────┘        └──────────────────────────┘
          │
          │  (in parallel) OSC 133 C/D markers
          ▼
┌────────────────────┐
│  captureScan       │  → sessionState.lastOutput
│ (terminal_capture) │     → GetLastOutput() for exact copy
└────────────────────┘
```

---

## Database Schema

The SQLite schema (version 10) consists of:

| Table | Purpose |
|-------|---------|
| `schema_version` | Tracks the applied schema version |
| `categories` | Command groups (id, name, icon, color) |
| `commands` | Saved commands (title, description, script, category, position, working dir) |
| `tags` | Unique tag names |
| `command_tags` | Many-to-many link between commands and tags |
| `variable_definitions` | Per-command variable metadata (name, description, example, default, sort order) |
| `variable_presets` | Named sets of variable values per command |
| `preset_values` | Individual key/value pairs within a preset |
| `executions` | Legacy execution history — **no longer written or read in production** |
| `app_settings` | Single-row JSON blob holding all user preferences |
| `commands_fts` | FTS5 virtual table for full-text search |

**Indexes & constraints:**

- Foreign keys with `ON DELETE CASCADE` for variable definitions, presets, tags, and executions.
- `commands.category_id` is the exception: nullable with `ON DELETE SET NULL`, so deleting a category uncategorizes its commands rather than deleting them.
- FTS5 triggers (`commands_ai`, `commands_ad`, `commands_au`) keep the search index in sync.

### Migrations

Migrations live in the `migrations/` package as `NNNN_description.go` files, each defining a `Migration{Version, Description, DisableFKDuringMigration, Up, Down}`, appended to the ordered `Migrations` slice in `migration.go`. The runner in `db.go` wraps each `Up` in a transaction.

Because SQLite has no `ALTER COLUMN`, most migrations recreate the table: create `_new`, copy, drop, rename, then re-create triggers, indexes, and the FTS table. After recreating `commands`, rebuild the FTS index with `INSERT INTO commands_fts(commands_fts) VALUES('rebuild')`.

Versions **skip 4** — it was intentionally folded into 5 — so the runner compares `Migration.Version` rather than assuming `+1` increments. `DisableFKDuringMigration` is set only by `migration0005`, which needs `PRAGMA foreign_keys = OFF` before its transaction begins.

---

## Key Design Decisions

### 1. Execution through a PTY, not a subprocess

Running commands by writing to a live PTY (rather than `exec`ing a temp script and streaming captured pipes) is the decision that shapes the rest of the app. It buys interactive prompts, full ANSI/TUI support, correct Ctrl+C via the foreground process group, and a shell that keeps its state between commands. The costs are equally real: there is no exit code or captured output on the `RunCommand` return path, no execution history, and knowing what a command printed requires the OSC 133 machinery described above.

### 2. Wails v3 over v2

Wails v3 introduces service-based registration (`application.NewService`) and an improved event system. Eight focused services replace v2's monolithic `App` struct.

### 3. Pure-Go SQLite (`modernc.org/sqlite`)

Avoiding CGO keeps cross-compilation simple and ships a single static binary with no system SQLite dependency.

### 4. No external state library

All React state lives in `App.tsx` with hooks and refs. For a single-user desktop app with no server, Redux/Zustand would add ceremony without benefit.

### 5. Three-window architecture

Settings render in a separate native window (`/?window=settings`) rather than a modal, keeping the main UI uncluttered and letting the user close settings without disturbing editor context. The global launcher renders in its own persistent window (`/?window=launcher`) so its command palette and PTY remain available while the main editor stays hidden or focused elsewhere.

### 6. Inactive editor tabs stay mounted

`CommandDetailTab` toggles `display: none/flex` instead of unmounting, so scroll position, cursor, and uncommitted edits survive tab switches.

### 7. Settings as one JSON blob

Since migration 0009, all preferences live in a single `app_settings` row as JSON. Adding a preference is a Go struct field, not a migration.

### 8. Template variable syntax `{{var}}`

Placeholders deliberately avoid shell syntax (`$var`, `${var}`), so substitution is unambiguous and cannot collide with what the shell itself will expand.

### 9. Per-OS paths (`OSPathMap`)

Working directories are stored per OS (`darwin`/`linux`/`windows`), so a command exported from one machine and imported on another still resolves sensibly.

### 10. Nonce-protected shell integration

Capturing output via OSC 133 markers is only trustworthy if a command cannot print those markers itself. The per-session nonce, delivered by file rather than environment value, is what makes the capture path safe to trust.

---

## Known Gaps

Documented deliberately; see `AGENTS.md` for the full discussion.

- **`buildPtyEnv` is not fully Windows-correct.** Windows' `os.Environ()` includes per-drive cwd entries like `=C:=C:\foo`, which `strings.Cut(kv, "=")` collapses into one malformed entry; Windows env names are also case-insensitive while the function's map keys are not.
- **The `executions` table and its DB methods are vestigial.** Nothing in production writes or reads history.

Fixed (issue #63): dispatch previously always terminated a line with `\n` and always used `shellQuoteDir`'s POSIX single-quote escaping for the cd prefix — neither of which cmd.exe or PowerShell can execute (ConPTY has no tty line discipline, so LF never submits; Windows PowerShell 5.1 has no `&&` operator). `buildCommandLine`/`shellDialectFor` (`executor.go`) now pick the submit key and cd-prefix syntax per shell dialect.

---

## File Organization

```
cmdex/
├── main.go                     # Entry point, service registration, native menu, window config
├── app.go                      # App lifecycle, settings window, GetOS, PickDirectory
├── command_service.go          # Command & category CRUD, search, presets
├── execution_service.go        # Variable resolution, working dir, RunCommand → PTY
├── settings_service.go         # Read/write app settings
├── importexport_service.go     # JSON import/export for commands & themes
├── event_service.go            # Event name constants exposed to frontend
├── terminal_service.go         # Multi-session PTY terminals (MaxSessions = 10)
├── pty_backend.go              # ptyHandle / ptyProcess interfaces
├── pty_backend_unix.go         # creack/pty backend
├── pty_backend_windows.go      # ConPTY backend (charmbracelet/x/conpty)
├── pty_backend_mock.go         # Mock backend for tests
├── pty_env.go                  # buildPtyEnv — TERM/COLORTERM/LANG for GUI launches
├── shell_integration.go        # Materialize scripts, activate OSC 133, per-session nonce
├── terminal_capture.go         # OSC 133 marker scanner, GetLastOutput
├── ansi.go                     # ANSI stripping + ConPTY wrap-artifact removal
├── db.go                       # SQLite schema, migration runner, queries
├── executor.go                 # Shell selection, shebang/quoting helpers, CEL eval
├── script.go                   # {{var}} parsing & substitution, shebang stripping
├── models.go                   # Go structs mirroring DB entities
├── migrations/                 # 0001…0010 + migration.go (ordered Migrations slice)
├── shell-integration/          # Embedded bash / zsh / fish / pwsh startup scripts
│
├── frontend/
│   ├── src/
│   │   ├── main.tsx            # Routes between main, settings & launcher windows
│   │   ├── App.tsx             # Central state, tabs, modals, terminal sessions
│   │   ├── types.ts            # TypeScript interfaces (mirror of Go models)
│   │   ├── i18n.ts             # i18n configuration
│   │   ├── style.css           # Global CSS variables & themes
│   │   ├── components/         # Feature components + ui/ (shadcn primitives)
│   │   ├── hooks/              # useKeyboardShortcuts, useResizable, useSyncedRef, …
│   │   ├── utils/              # tab, tabDraft, templateVars, path, clipboard
│   │   ├── lib/                # utils (cn), shortcuts, theme-apply
│   │   └── wails/events.ts     # Event name initialization from backend
│   ├── bindings/               # Generated by Wails (committed, never hand-edited)
│   ├── e2e/                    # Playwright specs + @wailsio/runtime mock
│   ├── vite.config.ts
│   └── package.json
│
├── build/                      # Wails config (config.yml), Taskfile, platform assets
└── docs/                       # This file and its siblings
```

---

## Key Abstractions

### Go Backend

| Abstraction | File | Description |
|---|---|---|
| `DB` struct | `db.go` | Wrapper over `database/sql` with the `modernc.org/sqlite` driver. Owns migrations (version 10), FTS5 triggers, WAL mode, and every CRUD query the services use. |
| `TerminalService` + `sessionState` | `terminal_service.go` | Session registry keyed by ID, capped at `MaxSessions` user-visible sessions (internal launcher sessions excluded). Each `sessionState` holds its PTY handle, process, shell path, last size, capture buffers, and nonce, guarded by a per-session mutex. |
| `LauncherService` | `launcher_service.go` | Persistent always-on-top launcher window, global shortcut, launch-at-login, and dedicated internal terminal session. |
| `ptyHandle` / `ptyProcess` | `pty_backend.go` | The only interfaces the terminal service is written against, so Unix, Windows, and mock backends are interchangeable. |
| `Executor` struct | `executor.go` | Shell selection (`$SHELL -lc` / `cmd /C`) plus CEL default evaluation. Constructed once at startup; executes nothing. |
| `Command` struct | `models.go` | Central domain entity: nullable title/description (`sql.NullString`), embedded `VariableDefinition[]`/`VariablePreset[]`, an `OSPathMap` working directory, and a `DisplayTitle()` that falls back to script content. |
| `OSPathMap` type | `models.go` | JSON-marshalable map of OS key → path, with `GetCurrentOS()`, `GetForOS()`, `SetForOS()`. Mirrored on the frontend as `Record<string, string>`. |
| `AppSettings` struct | `models.go` | Every user preference, stored as one JSON blob in `app_settings`. Pointer fields (`DefaultWorkingDir`, `WindowX`, `ShellIntegration`) distinguish "unset" from "explicitly cleared". |
| `TerminalLastOutput` | `terminal_capture.go` | `{Available, Text, ExitCode, Truncated}` — the result of OSC 133 capture, with `Available: false` signaling the frontend to fall back to buffer scraping. |
| `Migration` struct | `migrations/migration.go` | `{Version, Description, DisableFKDuringMigration, Up, Down}`, ordered in the `Migrations` slice and compared by `Version` (values skip 4). |
| Service registration | `main.go` | Each concern is a struct registered via `application.NewService(...)`; all exported methods become callable TypeScript functions. |

### React Frontend

| Abstraction | File | Description |
|---|---|---|
| `TabDraft` interface | `frontend/src/types.ts` | Per-tab edit buffer (`title`, `description`, `tags`, `categoryId`, `scriptBody`, `variables`, `workingDir`) plus a `revealed` sub-object tracking expanded optional fields. Compared against a baseline snapshot for dirty state. |
| `CommandDetailTab` | `frontend/src/components/CommandDetailTab.tsx` | `React.memo` wrapper connecting a `TabDraft` + `Command` to `CommandDetail`, binding tab-local callbacks and rendering `FloatingSaveBar`. Uses `display: none/flex` so inactive tabs are never unmounted. |
| `Terminal` + `TerminalHandle` | `frontend/src/components/Terminal.tsx` | xterm.js host. Subscribes to `pty-output/exit/cleared:<id>`, and exposes an imperative handle (kept in `terminalRefs`) so `App` can fit, focus, or scrape a session. |
| `ModalState` union | `frontend/src/App.tsx` | One discriminated union for every dialog, replacing a pile of booleans. |
| `useSyncedRef` | `frontend/src/hooks/useSyncedRef.ts` | Returns a ref whose `.current` tracks the latest value during render, so event handlers and callbacks read current state without re-subscribing. |
| `useKeyboardShortcuts` | `frontend/src/hooks/useKeyboardShortcuts.ts` | One `keydown` capture listener over a `ShortcutMap`. Modifier combos fire regardless of focus; bare keys only outside editable elements. |
| `applyTheme` / `applyDensity` / `applyFonts` | `frontend/src/lib/theme-apply.ts` | Write CSS custom properties and `data-theme`/`data-density` on `document.documentElement`. Used by both windows at startup and after any preference change. |
| `tabDraft.ts` helpers | `frontend/src/utils/tabDraft.ts` | `draftFromCommand()` (merges detected template vars), `draftsEqual()` (dirty detection), `cloneDraft()` (baseline snapshot), `makePlaceholderCommand()`. |
