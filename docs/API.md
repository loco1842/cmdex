<!-- generated-by: gsd-doc-writer -->

# API Reference

CmDex exposes its backend through Wails v3 **service method bindings** and **runtime events**. During build, Wails auto-generates TypeScript binding modules from the Go service structs, providing type-safe function calls and model classes to the frontend. There are no HTTP endpoints — all communication happens over Wails' internal IPC bridge.

---

## Wails Service Architecture

The Go backend registers seven services, each scoped to a domain:

| Service | Go Struct | Frontend Module | Purpose |
|---|---|---|---|
| App | `App` | `app.js` | Application lifecycle, native dialogs, OS detection |
| CommandService | `CommandService` | `commandservice.js` | Command & category CRUD, variable presets, search |
| ExecutionService | `ExecutionService` | `executionservice.js` | Command execution, output streaming, history |
| SettingsService | `SettingsService` | `settingsservice.js` | User preferences, terminal detection |
| ImportExportService | `ImportExportService` | `importexportservice.js` | Command import/export, theme template export |
| EventService | `EventService` | `eventservice.js` | Event name constants for type-safe event handling |
| TerminalService | `TerminalService` | `terminalservice.js` | Multi-session PTY-backed terminals |

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

import { GetVariables, RunCommand, GetExecutionHistory,
         ClearExecutionHistory, RunInTerminal }
    from '../bindings/cmdex/executionservice';

import { GetSettings, SetSettings, GetAvailableTerminals }
    from '../bindings/cmdex/settingsservice';

import { ExportCommands, ImportCommands, SaveThemeTemplate }
    from '../bindings/cmdex/importexportservice';

import { ShowSettingsWindow, GetOS, PickDirectory }
    from '../bindings/cmdex/app';

import { GetEventNames }
    from '../bindings/cmdex/eventservice';

// Barrel import (all services + models)
import { App, CommandService, EventService, ExecutionService,
         ImportExportService, SettingsService,
         AppSettings, Category, Command, EventNames,
         ExecutionRecord, TerminalInfo,
         VariableDefinition, VariablePreset, VariablePrompt }
    from '../bindings/cmdex';
```

Model classes (`AppSettings`, `Category`, `Command`, `ExecutionRecord`, `TerminalInfo`, `VariableDefinition`, `VariablePreset`, `VariablePrompt`, `EventNames`) are also exported from `../bindings/cmdex/models.js` and re-exported by the barrel `index.js`.

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
| `scriptContent` | `string` | Full script including `#!/bin/bash` header |
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

Captures a single command execution for history.

| Field | Type | Description |
|---|---|---|
| `id` | `string` | UUID |
| `commandId` | `string` | ID of the executed command |
| `scriptContent` | `string` | Full script content at time of execution |
| `finalCmd` | `string` | Resolved command with variable values substituted |
| `output` | `string` | Captured stdout (max 8 KB stored) |
| `error` | `string` | Captured stderr or error message |
| `exitCode` | `number` | Process exit code (0 = success, -1 = error/timeout) |
| `workingDir` | `string` | Resolved working directory used |
| `executedAt` | `string` (RFC 3339) | Execution timestamp |

### `TerminalInfo`

Describes a detected terminal emulator available for `RunInTerminal`.

| Field | Type | Description |
|---|---|---|
| `id` | `string` | Terminal ID (e.g., `"terminal"`, `"iterm2"`, `"alacritty"`) |
| `name` | `string` | Human-readable name (e.g., `"Terminal"`, `"iTerm2"`) |

### `AppSettings`

User preferences persisted to the SQLite database.

| Field | Type | Required | Description |
|---|---|---|---|
| `locale` | `string` | No | Language code (e.g., `"en"`, `"zh"`). Default: `"en"` |
| `terminal` | `string` | No | Preferred terminal ID. Empty = auto-detect |
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

### `OSPathMap`

A map of OS identifier (`"darwin"`, `"linux"`, `"windows"`) to directory path string. Used to store OS-specific working directories for commands and the global default. Exposed to TypeScript as `Record<string, string>`.

### `EventNames`

Returned by `GetEventNames()` with the following fields:

| Field | Event Constant | Event Data |
|---|---|---|
| `cmdOutput` | `"cmd-output"` | `{ stream: "stdout" \| "stderr", data: string }` |
| `openSettings` | `"open-settings"` | none |
| `openShortcuts` | `"open-shortcuts"` | none |
| `settingsChanged` | `"settings-changed"` | `Partial<SettingsPayload>` |
| `settingsWindowClosing` | `"settings-window-closing"` | none |

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

Creates a new command. The `scriptBody` is the raw command body (without shebang) — the backend wraps it in `#!/bin/bash`. `tags` and `variables` default to empty arrays if `null`.

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

Returns the full script content (including shebang header) for editing.

```typescript
const fullScript: string = await GetScriptContent(cmd.id);
```

#### `GetScriptBody(commandID: string)`

Returns just the script body (shebang stripped) for simple-mode editing.

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

Executes a command with template variables resolved. Streams stdout/stderr chunks via the `cmd-output` Wails event **during execution**, then returns the final `ExecutionRecord` once the process completes. The execution is persisted to history automatically.

```typescript
const record: ExecutionRecord = await RunCommand(cmd.id, { message: 'hello world' });
// record.output contains captured stdout (max 8KB)
// record.error contains captured stderr or error message
// record.exitCode is 0 for success, -1 for error/timeout
```

**Streaming output pattern:**

```typescript
import { Events } from '@wailsio/runtime';
import { eventNames } from './wails/events';

// Subscribe BEFORE calling RunCommand
const cleanup = Events.On(eventNames.cmdOutput, (event) => {
  const chunk = event.data as { stream: 'stdout' | 'stderr'; data: string };
  if (chunk.stream === 'stderr') {
    console.error(chunk.data);
  } else {
    console.log(chunk.data);
  }
});

// Execute
const record = await RunCommand(cmd.id, vars);

// Unsubscribe when done
cleanup();
```

- **Execution timeout:** 60 seconds. Timed-out commands return `exitCode: -1` with an error message.
- **Output cap:** Persisted output is truncated to 8 KB. The full stream is delivered via events but only the first 8 KB is stored in history.

#### `RunInTerminal(commandID: string, variables: Record<string, string>)`

Opens the resolved command in the system's terminal emulator. Uses the user's preferred terminal from settings, or auto-detects an available one. The terminal remains open after the command completes.

```typescript
await RunInTerminal(cmd.id, { message: 'hello' });
```

**Supported terminals by OS:**

| OS | Supported Terminals |
|---|---|
| macOS | Terminal.app, iTerm2, Warp, Alacritty, Kitty, Ghostty, Hyper |
| Linux | GNOME Terminal, GNOME Console (kgx), Konsole, XFCE Terminal, Alacritty, Kitty, Ghostty, XTerm |
| Windows | Windows Terminal, Command Prompt (cmd), PowerShell / pwsh |

#### `GetExecutionHistory()`

Returns all past execution records.

```typescript
const history: ExecutionRecord[] = await GetExecutionHistory();
```

#### `ClearExecutionHistory()`

Deletes all execution history records.

```typescript
await ClearExecutionHistory();
```

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

#### `GetAvailableTerminals()`

Returns all terminal emulators detected on the current system.

```typescript
const terminals: TerminalInfo[] = await GetAvailableTerminals();
// Example: [{ id: 'terminal', name: 'Terminal' }, { id: 'iterm2', name: 'iTerm2' }]
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
      "scriptContent": "#!/bin/bash\n\necho \"{{message}}\"\n",
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
// names.cmdOutput === "cmd-output"
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

### `cmd-output`

Emitted by the backend during `RunCommand()` execution. Each chunk is one line of stdout or stderr output.

**Emitter:** `ExecutionService.RunCommand` (Go backend, via `wailsApp.Event.Emit`)

**Data:**

```typescript
{ stream: "stdout" | "stderr"; data: string }
```

**Usage:**

```typescript
import { Events } from '@wailsio/runtime';
import { eventNames } from './wails/events';

const cleanup = Events.On(eventNames.cmdOutput, (event) => {
  const chunk = event.data as { stream: string; data: string };
  // chunk.stream is "stdout" or "stderr"
  // chunk.data is a line of output (with trailing newline)
});
```

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
  terminal?: string;
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
3. **System errors** — File I/O errors, execution failures (returned as `ExecutionRecord` with `exitCode: -1` and an `error` field, not as promise rejections).

**Error handling pattern:**

```typescript
try {
  const cmd = await CreateCommand('Title', '', 'echo hello', catId, [], [], {});
} catch (err) {
  console.error('Failed to create command:', err);
  // err is a string
}
```

For `RunCommand`, non-zero exit codes and execution errors are returned in the `ExecutionRecord`, not as rejections:

```typescript
const record = await RunCommand(cmd.id, vars);
if (record.exitCode !== 0) {
  console.error('Command failed:', record.error);
}
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

This resolution happens in `ExecutionService.resolveWorkingDir()` (Go backend) and is transparent to the frontend. The resolved path is included in `ExecutionRecord.workingDir`.
