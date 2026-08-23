# API Reference

CmDex exposes its backend through Wails v3 **service method bindings** and **runtime events**. During build, Wails auto-generates TypeScript binding modules from the Go service structs, providing type-safe function calls and model classes to the frontend. There are no HTTP endpoints — all communication happens over Wails' internal IPC bridge.

---

## Wails Service Architecture

The Go backend registers seven services, each scoped to a domain:

| Service | Go Struct | Frontend Module | Purpose |
|---|---|---|---|
| App | `App` | `app.js` | Application lifecycle, native dialogs, OS detection |
| CommandService | `CommandService` | `commandservice.js` | Command & category CRUD, variable presets, search |
| ExecutionService | `ExecutionService` | `executionservice.js` | Variable resolution, dispatching a command to the active terminal |
| SettingsService | `SettingsService` | `settingsservice.js` | User preferences |
| ImportExportService | `ImportExportService` | `importexportservice.js` | Command import/export, theme template export |
| EventService | `EventService` | `eventservice.js` | Event name constants for type-safe event handling |
| TerminalService | `TerminalService` | `terminalservice.js` | Multi-session PTY-backed terminals, output capture |

Every service method returns a `CancellablePromise<T>` (from `@wailsio/runtime`). On error, the promise rejects — there is no explicit error return in the TypeScript signature.

---

## Import Paths

All bindings are generated into `frontend/bindings/cmdex/`. The canonical import from frontend source:

```typescript
// Individual service imports
import { GetCategories, CreateCategory, UpdateCategory, DeleteCategory,
         GetCommands, CreateCommand, UpdateCommand, DeleteCommand,
         GetPresets, SavePreset, UpdatePreset, DeletePreset,
         ReorderCommand, GetScriptBody, ReorderPresets,
         SearchCommands, ResetAllData }
    from '../bindings/cmdex/commandservice';

import { GetVariables, RunCommand }
    from '../bindings/cmdex/executionservice';

import { GetSettings, SetSettings }
    from '../bindings/cmdex/settingsservice';

import { ExportCommands, ImportCommands, SaveThemeTemplate }
    from '../bindings/cmdex/importexportservice';

import { ShowSettingsWindow, GetOS, PickDirectory }
    from '../bindings/cmdex/app';

import { GetEventNames }
    from '../bindings/cmdex/eventservice';

import { CreateSession, ListSessions, CloseSession, RenameSession,
         SetActiveSession, GetActiveSession, Start, Stop,
         Write, Resize, Clear, GetLastOutput }
    from '../bindings/cmdex/terminalservice';

// Barrel import (all services + models)
import { App, CommandService, EventService, ExecutionService,
         ImportExportService, SettingsService, TerminalService,
         AppSettings, Category, Command, EventNames,
         ExecutionRecord, SessionInfo, TerminalLastOutput,
         VariableDefinition, VariablePreset, VariablePrompt }
    from '../bindings/cmdex';
```

Model classes (`AppSettings`, `Category`, `Command`, `ExecutionRecord`, `SessionInfo`, `TerminalLastOutput`, `VariableDefinition`, `VariablePreset`, `VariablePrompt`, `EventNames`) are also exported from `../bindings/cmdex/models.js` and re-exported by the barrel `index.js`.

---

## Data Models

### `Category`

A group that organizes related commands.

| Field | Type | Description |
|---|---|---|
| `id` | `string` | UUID |
| `name` | `string` | Display name |
| `icon` | `string` | Icon identifier (e.g., emoji or icon key) |
| `color` | `string` | Accent color hex or CSS value |
| `createdAt` | `string` (RFC 3339) | Creation timestamp |
| `updatedAt` | `string` (RFC 3339) | Last update timestamp |

### `Command`

A saved CLI command with metadata, template variables, and presets.

| Field | Type | Description |
|---|---|---|
| `id` | `string` | UUID |
| `title` | `NullString` | Display title (`String` / `Valid`) |
| `description` | `NullString` | Optional description |
| `scriptContent` | `string` | The script body. Stored **without** a shebang; records saved by older versions may still carry a leading `#!` line, which the backend strips on read |
| `tags` | `string[]` | User-defined tags |
| `variables` | `VariableDefinition[]` | Template variable definitions |
| `presets` | `VariablePreset[]` | Named sets of variable values |
| `workingDir` | `OSPathMap` | OS-keyed working directory paths |
| `categoryId` | `string` | Parent category UUID (empty = uncategorized) |
| `position` | `number` | Sort position within category |
| `createdAt` | `string` (RFC 3339) | Creation timestamp |
| `updatedAt` | `string` (RFC 3339) | Last update timestamp |

### `VariableDefinition`

Describes a `{{varName}}` template placeholder in a command script.

| Field | Type | Description |
|---|---|---|
| `name` | `string` | Variable name (must match `{{name}}` in script) |
| `description` | `string` | Human-readable hint shown in prompt |
| `example` | `string` | Example value shown in prompt |
| `default` | `string` | Default value or CEL expression (e.g., `now()`, `env("HOME")`) |
| `sortOrder` | `number` | Display order in variable prompt modal |

### `VariablePreset`

A saved snapshot of variable values for a command.

| Field | Type | Description |
|---|---|---|
| `id` | `string` | UUID |
| `name` | `string` | Preset display name |
| `position` | `number` | Sort order among presets |
| `values` | `Record<string, string>` | Map of variable name → value |

### `VariablePrompt`

Returned by `GetVariables()` to drive the fill-variables modal.

| Field | Type | Description |
|---|---|---|
| `name` | `string` | Variable name (matches definition) |
| `placeholder` | `string` | Input placeholder text |
| `description` | `string` | Hint text |
| `example` | `string` | Example value |
| `defaultExpr` | `string` | Raw CEL expression or literal from definition |
| `defaultValue` | `string` | Evaluated default value (ready to use) |

### `ExecutionRecord`

The return value of `RunCommand`. Despite the name, it is **not** persisted anywhere — see the `RunCommand` notes below.

| Field | Type | Description |
|---|---|---|
| `id` | `string` | UUID, generated per call |
| `commandId` | `string` | ID of the dispatched command |
| `scriptContent` | `string` | Unused on the current path (empty) |
| `finalCmd` | `string` | The exact line written to the PTY, including any shell-dialect cd prefix and its line-submit key (`\n` on POSIX, `\r` on cmd.exe/PowerShell) |
| `output` | `string` | Unused — output streams over `pty-output:<id>` instead (empty) |
| `error` | `string` | Populated only on dispatch failure (no active session, write error, command not found) |
| `exitCode` | `number` | `-1` on dispatch failure; `0` (zero value) on success — it is **not** the command's exit code |
| `workingDir` | `string` | Unused on the current path (empty) |
| `executedAt` | `string` (RFC 3339) | Dispatch timestamp |

> To learn a command's real exit status or output, use `TerminalService.GetLastOutput(sessionId)` (requires shell integration) or watch the `pty-output:<id>` stream.

### `SessionInfo`

A terminal session, as returned by `CreateSession`, `ListSessions`, and `GetActiveSession`.

| Field | Type | Description |
|---|---|---|
| `id` | `string` | Session UUID — also the suffix of that session's `pty-*` event names |
| `name` | `string` | Display name shown in the terminal tab bar |
| `running` | `boolean` | Whether the PTY process is alive |
| `shellPath` | `string` | Resolved shell binary for this session |
| `workingDir` | `string` | Directory the session was started in |

### `TerminalLastOutput`

Returned by `GetLastOutput`. Produced from OSC 133 shell-integration markers.

| Field | Type | Description |
|---|---|---|
| `available` | `boolean` | `false` when no command has completed under shell integration (including sessions whose shell has no integration). The other fields are zero values, and the caller should fall back to scraping the xterm buffer |
| `text` | `string` | Exact output of the last completed command, ANSI-stripped |
| `exitCode` | `number` | That command's real exit code, from the `D;<code>` marker |
| `truncated` | `boolean` | `true` if output exceeded the 1 MiB capture cap (the tail is kept) |

### `AppSettings`

User preferences persisted to the SQLite database.

| Field | Type | Required | Description |
|---|---|---|---|
| `locale` | `string` | No | Language code (e.g., `"en"`, `"zh"`). Default: `"en"` |
| `theme` | `string` | No | Active theme ID (e.g., `"vscode-dark"`) |
| `lastDarkTheme` | `string` | No | Last used dark theme ID |
| `lastLightTheme` | `string` | No | Last used light theme ID |
| `customThemes` | `string` | No | JSON-encoded custom theme array |
| `uiFont` | `string` | No | UI sans-serif font family |
| `monoFont` | `string` | No | Monospace font family for editor |
| `density` | `string` | No | Layout density: `"compact"`, `"comfortable"`, or `"spacious"` |
| `defaultWorkingDir` | `OSPathMap` | No | Global default working directory per OS |
| `windowX` | `number` | No | Settings window X position |
| `windowY` | `number` | No | Settings window Y position |
| `windowWidth` | `number` | No | Settings window width (min 480) |
| `windowHeight` | `number` | No | Settings window height (min 400) |
| `shellIntegration` | `boolean` | No | OSC 133 shell-integration markers. Unset (`null`) = enabled. Applies to **newly started sessions only** |

There is no `terminal` field — the external-terminal-emulator preference was removed when execution moved into the built-in PTY terminal.

### `OSPathMap`

A map of OS identifier (`"darwin"`, `"linux"`, `"windows"`) to directory path string. Used to store OS-specific working directories for commands and the global default. Exposed to TypeScript as `Record<string, string>`.

### `EventNames`

Returned by `GetEventNames()` with the following fields:

| Field | Event Constant | Event Data |
|---|---|---|
| `openSettings` | `"open-settings"` | none |
| `openShortcuts` | `"open-shortcuts"` | none |
| `settingsChanged` | `"settings-changed"` | `Partial<SettingsPayload>` |
| `settingsWindowClosing` | `"settings-window-closing"` | none |

Per-session terminal events (`pty-output:<id>`, `pty-exit:<id>`, `pty-cleared:<id>`) are **not** in `EventNames` — they are built by string concatenation from a session ID. See [Wails Runtime Events](#wails-runtime-events).

---

## CommandService API

All methods return `CancellablePromise<T>`. The promise rejects with an error string on failure.

### Category Operations

#### `GetCategories()`

Returns all categories.

```typescript
import { GetCategories } from '../bindings/cmdex/commandservice';
const cats: Category[] = await GetCategories();
```

#### `CreateCategory(name: string, icon: string, color: string)`

Creates a new category and returns it.

```typescript
const cat: Category = await CreateCategory('Deployment', '🚀', '#007acc');
```

#### `UpdateCategory(id: string, name: string, icon: string, color: string)`

Updates a category's name, icon, and color. Returns the updated category.

```typescript
const updated: Category = await UpdateCategory(cat.id, 'CI/CD', '🔄', '#4ec9b0');
```

#### `DeleteCategory(id: string)`

Deletes a category by ID. Commands in the category become uncategorized.

```typescript
await DeleteCategory(cat.id);
```

### Command Operations

#### `GetCommands()`

Returns all commands across all categories.

```typescript
const cmds: Command[] = await GetCommands();
```

#### `GetCommandsByCategory(categoryID: string)`

Returns commands belonging to a specific category.

```typescript
const deployCmds: Command[] = await GetCommandsByCategory(deployCatId);
```

#### `CreateCommand(title, description, scriptBody, categoryID, tags, variables, workingDir)`

Creates a new command. The `scriptBody` is the raw command body; `GenerateScript` only trims it and adds a trailing newline — **no shebang is added**. `tags` and `variables` default to empty arrays if `null`, and each variable's `sortOrder` is reassigned to its array index.

```typescript
import type { VariableDefinition, OSPathMap } from '../bindings/cmdex';

const workingDir: OSPathMap = { darwin: '/Users/me/projects' };
const vars: VariableDefinition[] = [
  { name: 'message', description: 'The message to echo', example: 'hello', default: '', sortOrder: 0 },
];
const cmd: Command = await CreateCommand(
  'Greet',
  'Print a greeting',
  'echo "{{message}}"',
  myCategoryId,
  ['greeting', 'demo'],
  vars,
  workingDir,
);
```

#### `UpdateCommand(id, title, description, scriptBody, categoryID, tags, variables, workingDir)`

Updates all fields of a command by ID. Returns the updated command.

```typescript
const updated: Command = await UpdateCommand(
  cmd.id, 'New Title', 'New desc', 'echo "{{msg}}"',
  newCatId, ['updated'], newVars, newWorkingDir,
);
```

#### `RenameCommand(id: string, newTitle: string)`

Sets a new title for a command. Lighter-weight than `UpdateCommand`.

```typescript
const renamed: Command = await RenameCommand(cmd.id, 'Better Greeting');
```

#### `DeleteCommand(id: string)`

Deletes a command by ID.

```typescript
await DeleteCommand(cmd.id);
```

#### `ReorderCommand(id: string, newPosition: number, newCategoryId: string)`

Moves a command to a new position within a category (or to a different category). `newCategoryId` may be empty for uncategorized. `newPosition` is 0-based. Returns the full command list.

```typescript
const cmds: Command[] = await ReorderCommand(cmd.id, 0, targetCategoryId);
```

#### `GetScriptContent(commandID: string)`

Returns the stored script content verbatim — including a legacy shebang line, if this record was saved by an older version.

```typescript
const fullScript: string = await GetScriptContent(cmd.id);
```

#### `GetScriptBody(commandID: string)`

Returns the script body with any leading shebang stripped (via `ParseScriptBody`). This is what the editor displays.

```typescript
const body: string = await GetScriptBody(cmd.id);
```

### Search

#### `SearchCommands(query: string)`

Returns commands matching a search query. An empty query returns all commands.

```typescript
const results: Command[] = await SearchCommands('deploy');
```

### Variable Presets

#### `GetPresets(commandID: string)`

Returns all variable presets for a command.

```typescript
const presets: VariablePreset[] = await GetPresets(cmd.id);
```

#### `SavePreset(commandID: string, name: string, values: Record<string, string>)`

Creates a new variable preset for a command.

```typescript
const preset: VariablePreset = await SavePreset(cmd.id, 'Staging', {
  message: 'Deploying to staging',
});
```

#### `UpdatePreset(commandID: string, presetID: string, name: string, values: Record<string, string>)`

Updates an existing preset. Validates that the preset belongs to the command.

```typescript
const updated: VariablePreset = await UpdatePreset(cmd.id, preset.id, 'Production', {
  message: 'Deploying to production',
});
```

#### `DeletePreset(commandID: string, presetID: string)`

Deletes a preset after validating it belongs to the command.

```typescript
await DeletePreset(cmd.id, preset.id);
```

#### `ReorderPresets(commandID: string, presetIDs: string[])`

Reorders presets to match the given ID slice. Must contain exactly all preset IDs for the command.

```typescript
await ReorderPresets(cmd.id, [presetB.id, presetA.id]);
```

### Reset

#### `ResetAllData()`

Deletes all categories, commands, presets, and execution history from the database. **Irreversible.**

```typescript
await ResetAllData();
```

---

## ExecutionService API

#### `GetVariables(commandID: string)`

Returns variable prompts for a command's fill-variables modal. Evaluates CEL default expressions (e.g., `now()`, `env("HOME")`) server-side.

```typescript
import { GetVariables } from '../bindings/cmdex/executionservice';
const prompts: VariablePrompt[] = await GetVariables(cmd.id);
```

#### `RunCommand(commandID: string, variables: Record<string, string>)`

Resolves the command's template variables and **writes the resulting line to the active terminal session's PTY**. It does not spawn a subprocess, capture output, or wait for the command to finish.

```typescript
const record: ExecutionRecord = await RunCommand(cmd.id, { message: 'hello world' });
// record.finalCmd is the exact line written to the PTY
// record.exitCode is -1 only if DISPATCH failed (no active session, write error)
// record.output is always empty — watch pty-output:<id> instead
```

**What it does, in order:**

1. Loads the command and substitutes `{{vars}}`.
2. Strips any shebang and trims trailing newlines.
3. Resolves the active session's shell (falling back to `detectShell()` if the session hasn't started yet) and the working directory (empty if none is configured), then calls `buildCommandLine(shellPath, script, workingDir)` (`executor.go`).
4. `buildCommandLine` prefixes a shell-dialect-correct `cd` (only if a working directory was resolved) — POSIX `cd '<dir>' && `, cmd.exe `cd /d "<dir>" && `, or PowerShell `Set-Location -LiteralPath '<dir>' -ErrorAction Stop; ` — and terminates every line with the dialect's submit key (`\n` for POSIX, `\r` for cmd.exe/PowerShell — ConPTY has no tty line discipline and treats a bare LF as a literal keystroke rather than Enter). The result is passed to `TerminalService.Write` on the active session.

**Because it goes through a real PTY:**

- There is **no timeout** and no output cap on execution — the command runs until it finishes, exactly as if typed.
- Interactive prompts, colors, and TUIs work. `Ctrl+C` reaches the process via the PTY's foreground process group.
- Shell state (cwd, exported variables, activated virtualenvs) persists between runs in the same session.
- Nothing is written to execution history.

**Failure modes** return an `ExecutionRecord` with `exitCode: -1` and a populated `error`, rather than rejecting:

```typescript
const record = await RunCommand(cmd.id, vars);
if (record.exitCode === -1) {
  // "no active terminal session" | "terminal service not initialized" | write/lookup error
  console.error(record.error);
}
```

**Reading the output:**

```typescript
import { Events } from '@wailsio/runtime';

// Live stream — subscribe once per session, not per run
const cleanup = Events.On(`pty-output:${sessionId}`, (event) => {
  const { data } = event.data as { data: string };
  term.write(data); // raw PTY bytes, feed straight to xterm.js
});

// Or, after the command completes, get exactly what it printed:
const last = await GetLastOutput(sessionId);
if (last.available) {
  console.log(last.text, last.exitCode);
}
```

---

## TerminalService API

Manages PTY-backed terminal sessions. At most `MaxSessions` (**10**) may exist concurrently; `CreateSession` rejects beyond that.

Every session ID doubles as an event namespace: a session's output arrives on `pty-output:<id>`. See [Wails Runtime Events](#wails-runtime-events).

### Session Lifecycle

#### `CreateSession()`

Creates a session and returns its `SessionInfo`. The PTY process is not started until `Start` is called with the terminal's dimensions.

```typescript
const session: SessionInfo = await CreateSession();
```

#### `Start(sessionId: string, cols: number, rows: number)`

Spawns the shell for the session at the given dimensions. Call this once the xterm instance has been measured, so the PTY's initial size matches what the user sees.

```typescript
await Start(session.id, term.cols, term.rows);
```

#### `Stop(sessionId: string)`

Terminates the session's process but keeps the session in the list, so it can be restarted with `Start`.

```typescript
await Stop(session.id);
```

#### `CloseSession(id: string)`

Terminates the process and removes the session entirely. Unsubscribe from its `pty-*` events afterwards.

```typescript
await CloseSession(session.id);
```

#### `ListSessions()`

Returns all sessions.

```typescript
const sessions: SessionInfo[] = await ListSessions();
```

#### `RenameSession(id: string, name: string)`

Sets a session's display name.

```typescript
await RenameSession(session.id, 'build');
```

#### `SetActiveSession(id: string)` / `GetActiveSession()`

Sets or reads the active session. **`ExecutionService.RunCommand` dispatches to whatever `GetActiveSession` returns**, so keep this in sync with the focused terminal tab.

```typescript
await SetActiveSession(session.id);
const active: SessionInfo | null = await GetActiveSession();
```

### Session I/O

#### `Write(sessionId: string, data: string)`

Writes raw bytes to the PTY. This is how keystrokes from xterm.js reach the shell, and how `RunCommand` dispatches a command. Include a trailing `\n` to submit a line.

```typescript
await Write(session.id, 'echo hello\n');
```

#### `Resize(sessionId: string, cols: number, rows: number)`

Informs the PTY of a new window size so the shell can reflow. Call on every xterm fit.

```typescript
await Resize(session.id, term.cols, term.rows);
```

#### `Clear(sessionId: string)`

Clears the session and emits `pty-cleared:<id>` so the frontend can reset its xterm buffer.

```typescript
await Clear(session.id);
```

#### `GetLastOutput(sessionId: string)`

Returns the exact output of the most recently completed command in the session, recorded from OSC 133 shell-integration markers — no prompt text, no echoed command, no reflow artifacts.

```typescript
const last: TerminalLastOutput = await GetLastOutput(session.id);
if (last.available && last.text.trim() !== '') {
  await navigator.clipboard.writeText(last.text);
} else {
  // Shell has no integration, none has completed yet, or the capture came
  // back blank — fall back to scraping the xterm buffer. `available: true`
  // with empty/whitespace-only `text` is a real, reachable state (e.g. a
  // stale "D" marker with no preceding "C"), not just the no-integration
  // case — treating `available` alone as "trust this" was what let "Copy
  // last output" silently copy nothing/blank lines for a failed command.
}
```

Requires `AppSettings.shellIntegration` to have been enabled **when the session started**. Capture is bounded at 1 MiB; on overflow the tail is kept and `truncated` is `true`.

---

## SettingsService API

#### `GetSettings()`

Returns the current application settings.

```typescript
import { GetSettings } from '../bindings/cmdex/settingsservice';
const settings: AppSettings = await GetSettings();
```

#### `SetSettings(jsonStr: string)`

Updates application settings from a JSON string. The JSON must deserialize to an `AppSettings` struct — partial updates are supported (omitted fields are left unchanged).

```typescript
await SetSettings(JSON.stringify({
  locale: 'zh',
  theme: 'tokyo-night',
  density: 'compact',
}));
```

After calling `SetSettings`, emit the `settings-changed` event so other windows (e.g., the main window if settings was changed from the settings window) pick up the new values:

```typescript
import { Events } from '@wailsio/runtime';
Events.Emit(eventNames.settingsChanged, newSettingsPayload);
```

---

## ImportExportService API

#### `ExportCommands(commandIDs: string[])`

Opens a native Save File dialog, then exports the selected commands to a JSON file. The export format includes version info, timestamps, commands with their variables, presets, and category names.

```typescript
import { ExportCommands } from '../bindings/cmdex/importexportservice';
await ExportCommands([cmd1.id, cmd2.id]);
```

**Export file format:**

```json
{
  "version": "1.0",
  "exportedAt": "2026-04-27T12:00:00Z",
  "commands": [
    {
      "title": "Greet",
      "description": "Print a greeting",
      "scriptContent": "echo \"{{message}}\"\n",
      "tags": ["greeting"],
      "variables": [{ "name": "message", "description": "...", "example": "...", "default": "", "sortOrder": 0 }],
      "presets": [{ "name": "Default", "values": { "message": "hello" } }],
      "workingDir": { "darwin": "/Users/me" },
      "categoryName": "My Category"
    }
  ]
}
```

#### `ImportCommands()`

Opens a native Open File dialog, parses a JSON export file (version `"1.0"`), and imports all commands. Categories are created if they don't already exist by name. Returns the full command list after import.

```typescript
const allCmds: Command[] = await ImportCommands();
```

#### `SaveThemeTemplate()`

Opens a native Save File dialog and writes a JSON template for custom theme creation. The template includes all required color fields with placeholder values.

```typescript
await SaveThemeTemplate();
```

---

## App Service API

#### `GetOS()`

Returns the current operating system identifier. Used by the frontend to read/write the correct OS key in `OSPathMap`.

```typescript
import { GetOS } from '../bindings/cmdex/app';
const os: string = await GetOS(); // "darwin" | "linux" | "windows"
```

#### `PickDirectory(currentPath: string)`

Opens a native directory picker dialog starting from `currentPath`. Returns the selected path, or an empty string if the user cancels.

```typescript
const selectedPath: string = await PickDirectory('/Users/me/projects');
```

#### `ShowSettingsWindow()`

Opens the settings window, creating it on first call. The window is a singleton — subsequent calls focus the existing window. The main menu `Cmd+,` shortcut calls this automatically.

```typescript
import { ShowSettingsWindow } from '../bindings/cmdex/app';
await ShowSettingsWindow();
```

---

## EventService API

#### `GetEventNames()`

Returns the canonical Wails event name constants. The frontend uses an `initEventNames()` helper (in `frontend/src/wails/events.ts`) that calls this on startup and populates a shared `eventNames` object. This enables type-safe event emission and listening without hardcoded strings.

```typescript
import { GetEventNames } from '../bindings/cmdex/eventservice';
const names = await GetEventNames();
// names.openSettings === "open-settings"
// names.settingsChanged === "settings-changed"
// names.settingsWindowClosing === "settings-window-closing"
```

---

## Wails Runtime Events

Events flow over the Wails v3 event bus. The frontend subscribes using `Events.On()` from `@wailsio/runtime` and unsubscribes by calling the returned cleanup function.

**Event wrapper format (Wails v3):**

All `Events.On` callbacks receive a `WailsEvent` object:

```typescript
interface WailsEvent {
  name: string;   // Event name string
  data: unknown;  // Payload — always unwrap `.data` to access the actual payload
  sender: string; // Window ID that emitted the event
}
```

### `pty-output:<sessionId>`

Raw bytes read from that session's PTY. Emitted continuously by the session's read-loop goroutine — this is the only channel command output travels on.

**Emitter:** `TerminalService` read loop (`terminal_service.go`, via `wailsApp.Event.Emit`)

**Data:** `{ data: string }`

**Usage:**

```typescript
import { Events } from '@wailsio/runtime';

const cleanup = Events.On(`pty-output:${sessionId}`, (event) => {
  const { data } = event.data as { data: string };
  term.write(data); // raw bytes, including ANSI escapes — feed straight to xterm.js
});
```

Do not line-split or trim this data: it is a byte stream, and escape sequences can straddle chunk boundaries.

### `pty-exit:<sessionId>`

Emitted once when the session's shell process exits.

**Data:** `{ exitCode: number; wasIntentional: boolean }`

`wasIntentional` is `true` when the exit followed a `Stop`/`CloseSession` call, letting the UI distinguish a deliberate close from a crash or a user typing `exit`.

```typescript
Events.On(`pty-exit:${sessionId}`, (event) => {
  const { exitCode, wasIntentional } = event.data as {
    exitCode: number; wasIntentional: boolean;
  };
  if (!wasIntentional) markSessionDead(sessionId, exitCode);
});
```

### `pty-cleared:<sessionId>`

Emitted after a successful `Clear(sessionId)` so the frontend can reset its xterm buffer.

**Data:** none

```typescript
Events.On(`pty-cleared:${sessionId}`, () => term.clear());
```

> **Note:** these three are the only output-bearing events. There is no `cmd-output` event — earlier revisions of this document described one, but command output has never flowed through a Go-side streaming callback since execution moved into the PTY terminal.

### `open-settings`

Emitted when the user selects **Settings...** from the CmDex application menu (`Cmd+,`). The frontend opens the settings window in response.

**Emitter:** Application menu handler (Go backend, `main.go`)

**Data:** none

**Usage:**

```typescript
Events.On(eventNames.openSettings, async () => {
  await ShowSettingsWindow();
});
```

### `open-shortcuts`

Emitted when the user selects **Keyboard Shortcuts...** from the Help menu (`Cmd+?`). The frontend opens the shortcuts dialog in response.

**Emitter:** Application menu handler (Go backend, `main.go`)

**Data:** none

**Usage:**

```typescript
Events.On(eventNames.openShortcuts, () => {
  setShortcutsDialogOpen(true);
});
```

### `settings-changed`

Emitted by the frontend after settings are saved, so other windows (e.g., the main window) can react to changes from the settings window. This is a frontend-to-frontend event (emitted via `Events.Emit`).

**Emitter:** Frontend (`SettingsPage.tsx`, `main.tsx`)

**Data:** `Partial<SettingsPayload>`

```typescript
interface SettingsPayload {
  locale?: string;
  shellIntegration?: boolean;
  theme?: string;
  lastDarkTheme?: string;
  lastLightTheme?: string;
  customThemes?: string;
  uiFont?: string;
  monoFont?: string;
  density?: string;
  defaultWorkingDir?: OSPathMap;
  windowX?: number;
  windowY?: number;
  windowWidth?: number;
  windowHeight?: number;
}
```

**Usage:**

```typescript
// Emit (in SettingsPage)
Events.Emit(eventNames.settingsChanged, {
  theme: 'tokyo-night',
  density: 'compact',
});

// Listen (in App.tsx)
Events.On(eventNames.settingsChanged, (event) => {
  const payload = event.data as Partial<SettingsPayload>;
  if (payload.theme) applyTheme(payload.theme);
  if (payload.locale) i18n.changeLanguage(payload.locale);
});
```

### `settings-window-closing`

Emitted by the backend when the settings window's close button is clicked. The backend clears the settings window reference so a new window is created on the next `ShowSettingsWindow()` call.

**Emitter:** Go backend (`app.go`, settings window `WindowClosing` hook)

**Data:** none

**Usage (backend only):** The frontend does not typically listen for this event; the backend uses it to nil out its window reference.

---

## Error Handling

All service methods are promise-based. On error, the promise rejects with a string message. There are no HTTP status codes — errors come from three sources:

1. **Database errors** — Returned as plain error strings from the DB layer (e.g., unique constraint violations, not-found errors).
2. **Validation errors** — Returned from service methods before touching the DB (e.g., preset ownership validation, import version mismatch).
3. **System errors** — File I/O errors, PTY/session errors (e.g. `CreateSession` past `MaxSessions`, writing to an unknown session ID).

Note that read-style methods do **not** reject. `GetCategories`, `GetCommands`, `GetVariables`, `SearchCommands`, and friends log the error on the Go side and return an empty slice, so an empty result does not distinguish "nothing found" from "query failed".

**Error handling pattern:**

```typescript
try {
  const cmd = await CreateCommand('Title', '', 'echo hello', catId, [], [], {});
} catch (err) {
  console.error('Failed to create command:', err);
  // err is a string
}
```

For `RunCommand`, **dispatch** failures are returned in the `ExecutionRecord` rather than as rejections. The command's own success or failure is not reported here at all — it happens asynchronously in the terminal:

```typescript
const record = await RunCommand(cmd.id, vars);
if (record.exitCode === -1) {
  console.error('Could not dispatch:', record.error);
}
// The command itself may still fail later. To observe that, read
// GetLastOutput(sessionId).exitCode once it has completed.
```

---

## Template Variables and CEL Expressions

Command scripts use `{{varName}}` double-brace syntax for template variables. The Go backend supports **CEL (Common Expression Language)** expressions in variable default values, enabling dynamic defaults:

| Expression | Description |
|---|---|
| `"literal value"` | A plain string value |
| `now()` | Current timestamp in RFC 3339 format |
| `date("2006-01-02")` | Current date formatted with Go's reference time layout |
| `env("HOME")` | Value of the named environment variable |

CEL expressions are evaluated server-side in `GetVariables()` (`ExecutionService`), producing a `defaultValue` that the frontend can display directly. If an expression fails to compile or evaluate, the raw expression string is used as the default value.

---

## Working Directory Resolution

When executing a command, the working directory is resolved using a fallback chain:

1. **Per-command working dir** — `Command.WorkingDir` for the current OS
2. **Global default** — `AppSettings.DefaultWorkingDir` for the current OS
3. **User home directory** — `os.UserHomeDir()`
4. **Current working directory** — `os.Getwd()`
5. **System temp directory** — `os.TempDir()`

This resolution happens in `ExecutionService.resolveWorkingDir()` (Go backend) and never returns an empty string.

A separate check, `hasExplicitWorkingDir`, decides whether a `cd` prefix is emitted at all: only steps 1 and 2 count as "explicit". When neither the command nor global settings specify a directory, no `cd` is prepended and the shell simply stays where it is — the later fallbacks exist for callers that need a concrete path, not to pin every command to `$HOME`.

The resolved path appears in `ExecutionRecord.finalCmd` (inside the cd prefix `buildCommandLine` emits) when a prefix was emitted. `ExecutionRecord.workingDir` is **not** populated on this path.
