<!-- generated-by: gsd-doc-writer -->

# Architecture

## Overview

CmDex is a cross-platform desktop application for saving, organizing, and executing CLI commands with template variable support. It is built as a single-binary desktop app using **Wails v3**, which binds a **Go** backend to a **React 19.2.7 + TypeScript** frontend. All data is stored locally in an **SQLite** database with no external services or cloud dependencies.

The app follows a service-oriented architecture on the backend, where discrete Wails v3 services expose domain-specific operations (commands, execution, settings, import/export) to the frontend via auto-generated TypeScript bindings. The frontend is a single-page React application with tab-based command editing, a searchable sidebar, streaming output panes, and a separate settings window.

---

## High-level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      CmDex Desktop App                       │
│  ┌───────────────────────────────────────────────────────┐  │
│  │           React 19.2.7 + Vite Frontend                │  │
│  │  ┌─────────────┐  ┌──────────────┐  ┌──────────────┐  │  │
│  │  │   Sidebar   │  │ Command Tabs │  │ Output Panes │  │  │
│  │  │  (Search)   │  │  (Editor)    │  │ (Streaming)  │  │  │
│  │  └─────────────┘  └──────────────┘  └──────────────┘  │  │
│  └──────────────────┬────────────────────────────────────┘  │
│                     │ Wails v3 Runtime                       │
│                     │ (Auto-generated TS bindings)           │
│  ┌──────────────────┴────────────────────────────────────┐  │
│  │                    Go Backend                          │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────────┐  │  │
│  │  │  Command    │ │  Execution  │ │     Settings    │  │  │
│  │  │  Service    │ │  Service    │ │    Service      │  │  │
│  │  └──────┬──────┘ └──────┬──────┘ └────────┬────────┘  │  │
│  │  ┌──────┴──────┐ ┌──────┴──────┐ ┌────────┴────────┐  │  │
│  │  │  Import/    │ │   Event     │ │      App        │  │  │
│  │  │  Export     │ │   Service   │ │   (Lifecycle)   │  │  │
│  │  │  Service    │ │             │ │                 │  │  │
│  │  └──────┬──────┘ └──────┬──────┘ └─────────────────┘  │  │
│  │         └─────────────────┘                            │  │
│  │                    DB Layer (SQLite)                   │  │
│  │              ┌─────────────────────┐                   │  │
│  │              │  ~/.cmdex/cmdex.db  │                   │  │
│  │              └─────────────────────┘                   │  │
│  └────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

**Key architectural boundaries:**

- **Frontend** (`frontend/src/`) — React components, hooks, and state management. Communicates with Go exclusively through Wails-generated bindings and runtime events.
- **Wails Runtime** — Bridges Go methods to TypeScript functions and provides an event bus for streaming data (e.g., command output chunks).
- **Backend Services** (`*.go` at project root) — Domain services registered with Wails; each exposes methods callable from the frontend.
- **Database Layer** (`db.go`) — Pure-Go SQLite via `modernc.org/sqlite`, with FTS5 full-text search and transactional migrations.

---

## Backend Layer

### Application Lifecycle (`app.go`)

The `App` struct manages application lifecycle and the secondary settings window.

- **`ServiceStartup`** — Initializes the global `DB` and `Executor` instances when Wails starts.
- **`ServiceShutdown`** — Closes the database connection on app quit.
- **`ShowSettingsWindow`** — Opens a secondary native window (`/?window=settings`) for app preferences. The window is created lazily and destroyed on close.

### Services

CmDex registers six Wails v3 services in `main.go`:

| Service | File | Responsibility |
|---------|------|----------------|
| `App` | `app.go` | Lifecycle, settings window management |
| `CommandService` | `command_service.go` | CRUD for categories, commands, variable presets, reordering, and search |
| `ExecutionService` | `execution_service.go` | Variable resolution, command execution, streaming output, terminal launching, execution history |
| `SettingsService` | `settings_service.go` | Read/write user preferences and detect available terminal emulators |
| `ImportExportService` | `importexport_service.go` | Export commands to JSON, import commands from JSON, save theme templates |
| `EventService` | `event_service.go` | Exposes event name constants to the frontend so both sides use the same strings |

### Database (`db.go`)

- **Engine:** SQLite via `modernc.org/sqlite` (pure Go, no CGO).
- **Location:** `~/.cmdex/cmdex.db`.
- **Schema version:** 9, with incremental migrations from v1 through v9.
- **Key features:**
  - WAL journal mode and foreign key enforcement.
  - FTS5 virtual table (`commands_fts`) for full-text search across title, description, and script content.
  - SQLite triggers keep the FTS index synchronized on insert, update, and delete.
  - Search falls back to `LIKE` queries if FTS5 fails.

### Command Execution (`executor.go`)

- **`Executor`** — Detects the platform shell (`bash` on macOS/Linux, `cmd` on Windows) and executes resolved scripts in temporary files.
- **Streaming** — `ExecuteScript` streams stdout/stderr via a callback (`OutputChunk`), caps persisted output at 8 KB, and enforces a 60-second timeout.
- **Terminal Integration** — `OpenInTerminal` detects and launches a variety of terminal emulators (Terminal, iTerm2, Warp, Alacritty, Kitty, Ghostty, GNOME Terminal, Windows Terminal, etc.) using OS-specific strategies (`osascript` on macOS, direct binary invocation on Linux/Windows).
- **CEL Evaluation** — `EvalDefaults` evaluates CEL expressions in variable defaults, supporting `now()`, `env("KEY")`, and `date("layout")`.

### Script Handling (`script.go`)

- **`GenerateScript`** / **`ParseScriptBody`** — Wraps user-editable script bodies in a `#!/bin/bash` header for execution.
- **Template Variables** — Uses `{{varName}}` syntax. `ExtractTemplateVars` detects variables from script text; `ReplaceTemplateVars` substitutes values before execution.
- **`MergeDetectedVars`** — Merges auto-detected variables with manually defined ones, preserving metadata (descriptions, defaults, examples).

---

## Frontend Layer

### Technology Stack

- **Framework:** React 19.2.7 with TypeScript
- **Build Tool:** Vite
- **UI Components:** shadcn/ui (built on Radix UI primitives + Tailwind CSS)
- **State Management:** React `useState` and `useRef` (no external state library)
- **I18n:** `react-i18next`
- **Notifications:** `sonner` toast system

### Application Entry (`main.tsx`)

The frontend is a Vite-built SPA embedded by Wails. Assets load locally, so the build prefers a **simple bundle** (no vendor code-splitting for size warnings) and only lazy-loads heavy entry points / features (e.g. `App` vs `SettingsPage`, and the xterm `Terminal`). It renders two UIs based on the URL:

- **Main Window** (`/` or no query param) — Lazily loads the primary `<App />` component.
- **Settings Window** (`/?window=settings`) — Lazily loads `<SettingsPage />` inside a dedicated settings window that persists preferences independently, then emits `settingsChanged` events back to the main window.

### Main App Structure (`App.tsx`)

`App.tsx` is the central orchestrator. It manages:

- **Global State:** `categories`, `commands`, `executionHistory`, `theme`, `uiFont`, `monoFont`, `density`.
- **Tab System:** `openTabs`, `activeTabId`, `tabDrafts`, `tabBaselines`. Each command opens in a tab with dirty-state tracking.
- **Modal State:** A discriminated union (`ModalState`) controls which modal is open: category editor, variable prompt, preset manager, discard confirmation, etc.
- **Execution State:** `isExecuting`, `streamLines`, `selectedRecord`, per-tab output persistence (`tabOutputRef`).
- **Settings Sync:** A `settingsRef` holds the latest settings values; `flushSettings` persists them to the Go backend. The main window also listens for `settingsChanged` events from the settings window.

### Key Components

| Component | Responsibility |
|-----------|----------------|
| `Sidebar` | Category tree, command list, drag-and-drop reordering, search |
| `CommandDetail` | Command editor (title, description, tags, script body, variables, presets) |
| `TabBar` | Tab switching and dirty-state indicators |
| `OutputPane` | Streaming stdout/stderr display with ANSI handling |
| `HistoryPane` | Past execution records for the selected command |
| `VariablePrompt` | Modal form for filling template variables before execution |
| `CommandPalette` | Quick-search overlay for commands |
| `SettingsPage` / `SettingsDialog` | Theme, density, font, terminal, and locale preferences |
| `ResizablePanel` | Collapsible/resizeable side panes |

### Type System (`types.ts`)

TypeScript interfaces mirror the Go structs from `models.go`:

- `Category`, `Command`, `VariableDefinition`, `VariablePreset`
- `VariablePrompt`, `ExecutionRecord`, `ExecutionResult`
- `TabDraft`, `SettingsPayload`

Utilities like `getCommandDisplayTitle` and `isNewCommandTabId` keep naming and tab logic consistent.

---

## Data Flow

### Wails Bindings

Wails v3 automatically generates TypeScript bindings for every exported method on registered services. During development, running `wails3 dev` (or `wails generate module`) produces callable functions under `frontend/bindings/`.

**Example flow:**
1. Go method: `func (s *CommandService) GetCommands() []Command`
2. Wails generates: `GetCommands()` in `frontend/bindings/cmdex/commandservice.ts`
3. Frontend imports: `import { GetCommands } from '../bindings/cmdex/commandservice'`
4. Frontend calls `GetCommands()` — Wails marshals the request, invokes Go, and returns a Promise resolving to the typed result.

### Runtime Events

For streaming and cross-window communication, CmDex uses Wails runtime events:

- **`cmd-output`** — `ExecutionService` emits `OutputChunk` structs while a command runs. The frontend listens via `Events.On(eventNames.cmdOutput, ...)` and appends chunks to a buffered stream renderer.
- **`settings-changed`** — The settings window emits this after any preference change. The main window listens and updates theme, fonts, and density in real time.
- **`open-settings`** / **`open-shortcuts`** — Triggered from the native menu bar to open the settings window or keyboard shortcuts dialog.
- **`settings-window-closing`** — Fired when the settings window closes, allowing the main window to clean up references.

### Command Execution Flow

```
User clicks "Run"
        │
        ▼
┌───────────────┐
│ VariablePrompt│  ← If command has {{vars}}, user fills values
│   (modal)     │
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ ExecutionService.RunCommand(commandID, variables)
│   - Loads command from DB
│   - Replaces {{vars}} with values
│   - Calls Executor.ExecuteScript()
└───────┬───────┘
        │
        ▼
┌───────────────┐
│   Executor    │  ← Spawns bash process, streams stdout/stderr
│               │     via callback → emits "cmd-output" events
└───────┬───────┘
        │
        ▼
┌───────────────┐
│   Frontend    │  ← Listens to "cmd-output", updates OutputPane
│  OutputPane   │
└───────────────┘
```

---

## Database Schema

The SQLite schema (version 9) consists of the following tables:

| Table | Purpose |
|-------|---------|
| `schema_version` | Tracks current schema version for migrations |
| `categories` | Command groups (id, name, icon, color) |
| `commands` | Saved CLI commands (title, description, script, category, position) |
| `tags` | Unique tag names |
| `command_tags` | Many-to-many link between commands and tags |
| `variable_definitions` | Per-command variable metadata (name, description, example, default, sort order) |
| `variable_presets` | Named sets of variable values per command |
| `preset_values` | Individual key/value pairs within a preset |
| `executions` | Execution history (output, error, exit code, working dir, timestamp) |
| `app_settings` | Single-row JSON blob storing all user preferences |
| `commands_fts` | FTS5 virtual table for full-text search |

**Indexes & Constraints:**
- Foreign keys with `ON DELETE CASCADE` for variable definitions, presets, tags, and executions.
- FTS5 triggers (`commands_ai`, `commands_ad`, `commands_au`) keep the search index in sync.

---

## Key Design Decisions

### 1. Wails v3 over Wails v2
The project uses Wails v3 (`github.com/wailsapp/wails/v3`), which introduces a service-based registration model (`application.NewService`) and an improved event system. Services are cleaner than the monolithic `App` struct pattern used in v2.

### 2. Pure-Go SQLite (`modernc.org/sqlite`)
By avoiding CGO, the project can be cross-compiled easily and distributed as a single static binary without system SQLite dependencies.

### 3. No External State Library
All React state lives in `App.tsx` using hooks and refs. This keeps the architecture simple and avoids the complexity of Redux/Zustand for a desktop app with a single user and no server.

### 4. Two-Window Architecture
Settings are rendered in a separate native window (`/?window=settings`) rather than a modal. This keeps the main UI uncluttered and allows independent window management (e.g., closing settings without losing editor context).

### 5. Per-Tab Output Persistence
Execution output and pane visibility are stored in `tabOutputRef` and `tabPaneStateRef`. When switching tabs, the UI restores the previous tab's output and pane state, giving the feeling of multiple independent workspaces.

### 6. Optimistic UI with Rollback
Command reordering (`handleReorderCommand`) updates local state immediately and rolls back if the backend call fails, keeping the UI responsive.

### 7. Settings Migration Strategy
Early settings were stored in `localStorage`. On startup, the app performs a one-time migration: if the database holds default values but `localStorage` has user-customized values, the localStorage values are promoted to the database and then cleared.

### 8. Template Variable Syntax: `{{var}}`
Commands use `{{variableName}}` for placeholders (not `${var}` or shell-style variables). This is distinct from shell syntax, making parsing reliable and substitution unambiguous before the script ever reaches a shell.

---

## File Organization

```
cmdex/
├── main.go                     # Application entry point, Wails config, native menu
├── app.go                      # App lifecycle & settings window management
├── command_service.go          # Command & category CRUD, search, presets
├── execution_service.go        # Run commands, stream output, terminal launch, history
├── settings_service.go         # Read/write app settings, terminal detection
├── importexport_service.go     # JSON import/export for commands & themes
├── event_service.go            # Event name constants exposed to frontend
├── db.go                       # SQLite schema, migrations, queries
├── executor.go                 # Subprocess execution, terminal integration, CEL eval
├── script.go                   # Shebang wrapping, {{var}} parsing & substitution
├── models.go                   # Go structs mirroring DB entities
├── wails.json                  # Wails project configuration
│
├── frontend/
│   ├── src/
│   │   ├── main.tsx            # React entry: routes between main & settings window
│   │   ├── App.tsx             # Central state, tabs, modals, execution orchestration
│   │   ├── types.ts            # TypeScript interfaces (mirror of Go models)
│   │   ├── i18n.ts             # i18n configuration
│   │   ├── style.css           # Global CSS variables & dark theme
│   │   ├── components/
│   │   │   ├── Sidebar.tsx
│   │   │   ├── CommandDetail.tsx
│   │   │   ├── TabBar.tsx
│   │   │   ├── OutputPane.tsx
│   │   │   ├── HistoryPane.tsx
│   │   │   ├── VariablePrompt.tsx
│   │   │   ├── CommandPalette.tsx
│   │   │   ├── SettingsPage.tsx
│   │   │   ├── SettingsDialog.tsx
│   │   │   ├── CategoryEditor.tsx
│   │   │   ├── ResizablePanel.tsx
│   │   │   ├── FloatingSaveBar.tsx
│   │   │   ├── KeyboardShortcutsDialog.tsx
│   │   │   ├── WelcomeTab.tsx
│   │   │   ├── InlineEditField.tsx
│   │   │   ├── HoverActionButton.tsx
│   │   │   └── ui/             # shadcn/ui primitives (button, dialog, tooltip, etc.)
│   │   ├── hooks/
│   │   │   └── useKeyboardShortcuts.ts
│   │   ├── utils/
│   │   │   ├── tabDraft.ts     # Tab draft creation, comparison, cloning
│   │   │   └── templateVars.ts # Variable detection & prompt generation
│   │   ├── wails/
│   │   │   └── events.ts       # Event name initialization from backend
│   │   ├── assets/
│   │   │   └── images/
│   │   │       └── main-logo.tsx
│   │   └── lib/
│   │       ├── utils.ts        # cn() and general helpers
│   │       └── shortcuts.ts    # Shortcut definitions
│   ├── bindings/               # Auto-generated by Wails (TypeScript service bindings)
│   ├── index.html
│   ├── vite.config.ts
│   ├── tsconfig.json
│   └── package.json            # pnpm-based; React 19.2.7, Vite, Tailwind, shadcn/ui
│
└── .planning/
    └── docs/
        └── ARCHITECTURE.md     # This file
```

---

## Key Abstractions

The most significant interfaces, design patterns, and utility abstractions that shape the codebase.

### Go Backend

| Abstraction | File | Description |
|---|---|---|
| `DB` struct | `db.go` | Database wrapper around `database/sql` with `modernc.org/sqlite` driver. Manages migrations (currently version 10), FTS5 full-text search triggers, WAL journal mode, and provides all CRUD query methods used by services. |
| `Executor` struct | `executor.go` | Subprocess execution engine. Detects the platform shell (`bash` on macOS/Linux, `cmd` on Windows), executes resolved scripts in temporary files, streams stdout/stderr via the `OutputChunk` callback, and supports opening commands in native terminal emulators. |
| `Command` struct | `models.go` | Central domain entity representing a saved CLI command. Includes nullable title/description fields, embedded `VariableDefinition[]` and `VariablePreset[]` slices, an `OSPathMap` for per-OS working directories, and a `DisplayTitle()` method that falls back to script content. |
| `OSPathMap` type | `models.go` | Custom JSON marshalable map that stores OS-keyed file paths (e.g., `"darwin": "/path/on/mac"`, `"linux": "/path/on/linux"`). Provides `GetCurrentOS()`, `GetForOS()`, and `SetForOS()` methods. Mirrored on the frontend as a plain `Record<string, string>`. |
| `AppSettings` struct | `models.go` | Single struct holding all user preferences (theme, fonts, density, locale, default working directory, window position). Stored as a single JSON blob in the `app_settings` database row. |
| Service Registration Pattern | `main.go` | Each backend concern is a discrete struct registered with Wails via `application.NewService(...)`. All exported methods on the struct are automatically exposed as callable TypeScript functions. This replaces the monolithic `App` struct used in Wails v2. |

### React Frontend

| Abstraction | File | Description |
|---|---|---|
| `TabDraft` interface | `frontend/src/types.ts` | In-memory edit buffer per open command tab. Contains independently editable fields (`title`, `description`, `tags`, `categoryId`, `scriptBody`, `variables`, `workingDir`) plus a `revealed` sub-object tracking which optional fields are expanded. Compared against a `baseline` snapshot to determine dirty state. |
| `CommandDetailTab` component | `frontend/src/components/CommandDetailTab.tsx` | Thin `React.memo` wrapper that connects a `TabDraft` and `Command` to the `CommandDetail` editor component. Binds tab-local callbacks (`onExecute`, `onSave`, `onDiscard`, preset operations) and renders a `FloatingSaveBar` for dirty-state save/discard actions. Rendered with `display: none/flex` to avoid unmounting inactive tabs. |
| `useResizable` hook | `frontend/src/hooks/useResizable.ts` | Generic drag-to-resize hook for side panels. Accepts axis, direction, min/max/default size, and a `localStorage` key. Reads initial size from `localStorage`, supports live mouse-drag resizing with clamping, and persists the final size on mouse-up. |
| `useKeyboardShortcuts` hook | `frontend/src/hooks/useKeyboardShortcuts.ts` | Global keyboard shortcut handler. Accepts a `ShortcutMap` (key combo → handler) and registers a single `keydown` capture listener. Modifier combos (`Cmd+Enter`) fire regardless of focus; bare keys (`?`) only fire when focus is outside editable elements. Uses a render-phase ref to avoid re-registering on prop changes. |
| `useSyncedRef` hook | `frontend/src/hooks/useSyncedRef.ts` | Lightweight utility that returns a `MutableRefObject` whose `.current` stays in sync with the latest value during render. Eliminates the need for `useEffect` to propagate value changes into refs used by event handlers and callbacks. |
| `useCopyToClipboard` hook | `frontend/src/hooks/useCopyToClipboard.ts` | Clipboard interaction hook. Returns a `{ copied, copy }` API where `copy(text)` writes to the system clipboard and sets `copied = true` for a configurable duration (default 1500ms). Cleans up timers on unmount. |
| `applyTheme` / `applyDensity` / `applyFonts` | `frontend/src/lib/theme-apply.ts` | Theme application utilities that write CSS custom properties and data attributes on `document.documentElement`. `applyTheme` sets `data-theme` and optionally sets 26 `--*` CSS variables for custom themes; `applyDensity` sets `data-density`; `applyFonts` sets `--font-sans` and `--font-mono`. Used by both the main window and settings window on startup and after any preference change. |
| `shortcuts.ts` registry | `frontend/src/lib/shortcuts.ts` | Central keyboard shortcut definition file. Exports `SHORTCUTS` constant with 19 named shortcuts (execute, save, palette, search, tab operations, etc.). Provides `isMac`, `cmdOrCtrl`, `shortcutLabelParts`, and `shortcutLabel` for platform-aware rendering (e.g., `⌘ Enter` on macOS, `Ctrl+Enter` on Windows/Linux). |
| `tab.ts` utilities | `frontend/src/utils/tab.ts` | Tab ID helpers: `isNewCommandTabId()` detects unsaved tabs by their `__new_` prefix, `createNewTabId()` generates UUID-based temporary tab IDs, and `getCommandDisplayTitle()` returns a display title with shebang stripping and 50-character truncation. |
| `path.ts` utilities | `frontend/src/utils/path.ts` | OS-aware path helpers: `normalizeOS()` maps runtime `GOOS` strings to `OSKey`, `getOSPath()`/`setOSPath()` read/write `OSPathMap` entries, and `shortenPath()` truncates file paths to the last N segments. |
| `tabDraft.ts` utilities | `frontend/src/utils/tabDraft.ts` | Tab draft lifecycle functions: `draftFromCommand()` builds an editable `TabDraft` from a `Command` (merging detected template variables), `draftsEqual()` performs deep comparison for dirty-state detection, `cloneDraft()` creates a JSON-based snapshot for baselines, and `makePlaceholderCommand()` provides a blank command shell for new tabs. |

---

*Last updated: generated by gsd-doc-writer on 2026-04-22.*
