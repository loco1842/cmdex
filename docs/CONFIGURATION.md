<!-- generated-by: gsd-doc-writer -->
# Configuration

Cmdex stores all user preferences and application data locally. This document describes every configurable aspect of the application, where settings are stored, and their default values.

---

## 1. App Settings Overview

User preferences are managed centrally through the **Settings** modal, accessible from the sidebar or via the `Cmd + ,` (macOS) / `Ctrl + ,` (Windows/Linux) keyboard shortcut. Settings are grouped into three tabs:

- **Appearance** — Themes, layout density, and custom theme import
- **Typography** — UI font and editor/monospace font selection
- **General** — Language/locale, terminal emulator, and data reset

Settings are persisted automatically to the local SQLite database when changed.

---

## 2. Appearance Settings

### Themes

Cmdex supports a collection of built-in color themes, plus the ability to import custom themes. The active theme is applied globally across the application.

| Theme ID | Label | Type |
|----------|-------|------|
| `vscode-dark` | VS Code Dark+ | Dark |
| `vscode-light` | VS Code Light+ | Light |
| `monokai` | Monokai | Dark |
| `tokyo-night` | Tokyo Night | Dark |
| `one-dark` | One Dark Pro | Dark |
| `classic` | Classic (Purple) | Dark |
| `catppuccin-mocha` | Catppuccin Mocha | Dark |
| `dracula` | Dracula | Dark |

- **Default:** `vscode-dark`
- The app tracks the last used dark and light theme separately, enabling quick toggling when the OS color scheme changes.
- **Custom themes** can be imported via a JSON file. The expected format includes `name`, `type` (`dark` or `light`), and a `colors` object with CSS variable mappings (e.g., `background`, `foreground`, `primary`, `accent`, `border`).

### Layout Density

Controls the spacing and compactness of the UI.

| Value | Label | Description |
|-------|-------|-------------|
| `compact` | Compact | Reduced padding and tighter spacing |
| `comfortable` | Comfortable | Balanced spacing (default) |
| `spacious` | Spacious | Increased padding and relaxed spacing |

- **Default:** `comfortable`
- Applied via the `data-density` HTML attribute on the document root.

---

## 3. Terminal Settings

The **Terminal** setting determines which terminal emulator is used when running a command via **"Run in Terminal"**.

- **Auto-detect** (default): The application automatically detects available terminal emulators on the system.
- **Manual selection**: Users can choose from a list of detected terminals.

Detected terminals are platform-specific:

- **macOS**: Terminal, iTerm2, Kitty, Hyper, Alacritty, WezTerm, Warp
- **Linux**: GNOME Terminal, Konsole, xterm, urxvt, Kitty, Alacritty, Tilix, Terminator, WezTerm, Hyper
- **Windows**: Windows Terminal, cmd, PowerShell, ConEmu, Cmder, Alacritty, WezTerm, Hyper

The terminal preference is stored as a terminal ID string (e.g., `iterm2`, `kitty`). An empty string means auto-detect.

---

## 4. Language / Locale

Cmdex supports internationalization via `react-i18next`.

- **Current supported languages:** English (`en`)
- **Default:** `en`
- The locale setting drives the UI language and is persisted across sessions.

---

## 5. Database Location

All application data — including commands, categories, variable presets, execution history, and settings — is stored in a local SQLite database.

| Property | Value |
|----------|-------|
| **Directory** | `~/.cmdex/` |
| **Database file** | `~/.cmdex/cmdex.db` |
| **Driver** | SQLite (via `modernc.org/sqlite`) |

The database is created automatically on first launch if it does not exist. It includes schema versioning and automatic migrations (current schema version: `9`).

---

## 6. Settings Storage (SQLite)

Application settings are stored in the `app_settings` table as a single JSON row.

### `app_settings` Table Schema

```sql
CREATE TABLE IF NOT EXISTS app_settings (
    data TEXT NOT NULL DEFAULT '{}'
);
```

The `data` column contains a JSON object with the following fields:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `locale` | string | `"en"` | UI language code |
| `terminal` | string | `""` | Preferred terminal ID; empty = auto-detect |
| `theme` | string | `"vscode-dark"` | Active theme ID |
| `lastDarkTheme` | string | `"vscode-dark"` | Last selected dark theme |
| `lastLightTheme` | string | `"vscode-light"` | Last selected light theme |
| `customThemes` | string | `"[]"` | JSON-encoded array of custom theme objects |
| `uiFont` | string | `"Inter"` | Sans-serif UI font family |
| `monoFont` | string | `"JetBrains Mono"` | Monospace font for script editor |
| `density` | string | `"comfortable"` | Layout density (`compact`, `comfortable`, `spacious`) |
| `windowX` | int | `-1` | Settings window X position; `-1` = center |
| `windowY` | int | `-1` | Settings window Y position; `-1` = center |
| `windowWidth` | int | `640` | Settings window width (min: 480) |
| `windowHeight` | int | `520` | Settings window height (min: 400) |

### Persistence Behavior

- Settings are **auto-saved** to the database whenever a value changes in the UI.
- On startup, settings are loaded from the database and applied to the frontend state.
- A one-time migration from legacy `localStorage` keys (used in earlier versions) occurs automatically if present, after which the localStorage keys are cleared.
- Settings changes emit a `settingsChanged` Wails event so that all open windows stay synchronized.

### Resetting Data

Users can reset all application data from the **Danger Zone** section in Settings. This:

1. Deletes all commands, categories, tags, variable definitions, presets, and execution history
2. Resets `app_settings` to factory defaults
3. Runs `VACUUM` on the SQLite database to reclaim space

**This action is irreversible.**

---

## 7. Environment Variables

Cmdex uses a small set of environment variables for build-time and runtime configuration. `.env` files (at the project root and `frontend/` subdirectory) are optional, loaded via each Taskfile's `dotenv:` directive — every variable below has a built-in default, so you don't need to create these files for standard development.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `APP_NAME` | N/A | `cmdex` | Hardcoded in `Taskfile.yml` — not environment-configurable. Binary output name. |
| `VITE_PORT` | No | `9245` | Port for the Vite development server. The default is baked into `Taskfile.yml`/`build/Taskfile.yml` as a shell fallback (`${VITE_PORT:-9245}`) — override via `.env`/`frontend/.env`, or ad hoc: `VITE_PORT=9246 task dev`. `frontend/vite.config.ts`'s `server.host` is pinned to `127.0.0.1` (not the ambiguous `localhost`) so Vite's bind address always matches the IPv4 address Wails' asset-server proxy dials. |
| `PRODUCTION` | No | (varies) | Set automatically by the build task (`build/Taskfile.yml`) to `true` for production builds or `false` for dev builds. Controls the Vite build mode. Not user-configurable. |
| `SHELL` | No | `/bin/sh` | Read at runtime on macOS/Linux to select the shell used for script execution (see executor in `executor.go`). If unset, `/bin/sh` is used. On Windows, `cmd` is always used regardless of this variable. |

All variables are optional — the application will function with factory defaults if none are set.

---

## 8. Config File Format

Cmdex uses two project-level configuration files beyond environment variables.

### `wails.json` (Project Root)

The Wails project descriptor. Defines application metadata, frontend directory, and build commands.

```json
{
  "name": "CmDex",
  "outputfilename": "cmdex",
  "author": {
    "name": "Loco",
    "email": "locnguyen1842@gmail.com"
  },
  "frontend": {
    "dir": "./frontend",
    "install": "pnpm install",
    "build": "pnpm run build",
    "dev": "pnpm run dev",
    "devServerUrl": "auto"
  }
}
```

| Key | Description |
|-----|-------------|
| `name` | Application display name. Used in window title, menus, and platform metadata. |
| `outputfilename` | Name of the compiled binary (without extension). |
| `author.name` / `author.email` | Author metadata embedded in the binary. |
| `frontend.dir` | Path to the frontend project directory. |
| `frontend.install` | Command to install frontend dependencies. |
| `frontend.build` | Command to build the frontend for production. |
| `frontend.dev` | Command to start the frontend dev server. |
| `frontend.devServerUrl` | Set to `"auto"` to let Wails auto-detect the Vite dev server URL from `VITE_PORT`. |

### `build/config.yml`

Wails build configuration controlling platform metadata and dev-mode behavior.

```yaml
version: '3'
info:
  companyName: "fenv"
  productName: "CmDex"
  productIdentifier: "com.fenv.cmdex"
  description: "CLI command manager with variable placeholders"
  copyright: "(c) 2026"
  version: "0.1.0"
dev_mode:
  root_path: .
  log_level: warn
  debounce: 1000
  ignore:
    dir: [.git, node_modules, frontend, bin]
    watched_extension: ["*.go", "*.js", "*.ts"]
    git_ignore: true
  executes:
    - cmd: wails3 build DEV=true
      type: blocking
    - cmd: wails3 task common:dev:frontend
      type: background
    - cmd: wails3 task run
      type: primary
```

| Key | Description |
|-----|-------------|
| `info.companyName` | Company/publisher name for platform metadata. |
| `info.productName` | Product display name (matches `wails.json` `name`). |
| `info.productIdentifier` | Reverse-domain bundle identifier (`com.fenv.cmdex`). |
| `info.version` | Application version string. |
| `dev_mode.log_level` | Wails log verbosity during development (`warn`, `info`, `debug`, `error`). |
| `dev_mode.debounce` | File-watch debounce delay in milliseconds before triggering rebuild. |
| `dev_mode.ignore` | Files and directories excluded from the dev-mode file watcher. |
| `dev_mode.executes` | Ordered list of commands run by `wails dev`. The `blocking` build step compiles the Go backend; the `background` step starts the Vite dev server; the `primary` step launches the compiled binary. |

---

## 9. Required vs Optional Settings

### Required for Execution

The following settings have no functional defaults and must be configured or auto-detected for certain features to work:

| Setting | Required For | Behavior When Unset |
|---------|-------------|---------------------|
| `terminal` (terminal emulator ID) | **"Run in Terminal"** feature | Falls back to **auto-detect**: the application scans the system for installed terminal emulators and uses the first one found. If no terminal is detected, `OpenInTerminal` returns an error. |
| Per-command `workingDir` (OSPathMap) | Working directory for command execution | Falls back through a **5-step resolution chain** (see "Default Working Directory" below). The final fallback is the user's home directory, so execution always has a valid working directory. |

### Optional (Factory Defaults Apply)

All other settings have factory defaults (see [Section 10](#10-defaults)). The application never fails to start due to a missing or corrupt settings value — the `GetSettings` function merges persisted JSON over a complete set of defaults, and `SetSettings` merges partial updates over existing values.

### Validation Constraints

| Setting | Constraint | Enforced In |
|---------|-----------|-------------|
| `density` | Must be one of `compact`, `comfortable`, or `spacious` | Frontend UI (dropdown selection) |
| `theme` | Must match a known theme ID or custom theme name | Frontend applies CSS variables; unknown IDs produce unstyled UI |
| `windowWidth` | Minimum 480px | Main window config in `main.go` (`MinWidth: 900`) and settings window re-centering logic |
| `windowHeight` | Minimum 400px | Main window config in `main.go` (`MinHeight: 600`) and settings window re-centering logic |
| `locale` | Must be a supported locale code (currently only `en`) | `react-i18next` will fall back to the default namespace for unknown locales |

---

## 10. Defaults

This section consolidates all default values in one place. These originate from the `GetSettings` function in `db.go` and the `Executor` struct in `executor.go`.

### Application Settings Defaults

| Setting | Default Value | Source |
|---------|--------------|--------|
| `locale` | `"en"` | `db.go` `GetSettings()` |
| `terminal` | `""` (auto-detect) | `db.go` `GetSettings()` |
| `theme` | `"vscode-dark"` | `db.go` `GetSettings()` |
| `lastDarkTheme` | `"vscode-dark"` | `db.go` `GetSettings()` |
| `lastLightTheme` | `"vscode-light"` | `db.go` `GetSettings()` |
| `customThemes` | `"[]"` (empty JSON array) | `db.go` `GetSettings()` |
| `uiFont` | `"Inter"` | `db.go` `GetSettings()` |
| `monoFont` | `"JetBrains Mono"` | `db.go` `GetSettings()` |
| `density` | `"comfortable"` | `db.go` `GetSettings()` |
| `defaultWorkingDir` | `{}` (empty OSPathMap; no OS-keyed paths) | `db.go` `GetSettings()` |
| `windowX` | `-1` (center on screen) | `db.go` `GetSettings()` |
| `windowY` | `-1` (center on screen) | `db.go` `GetSettings()` |
| `windowWidth` | `640` | `db.go` `GetSettings()` |
| `windowHeight` | `520` | `db.go` `GetSettings()` |

### Execution Defaults

| Parameter | Default Value | Source |
|-----------|--------------|--------|
| Execution timeout | `60s` | `executor.go` `defaultExecTimeout` |
| Max stored output per execution | `8 KB` (8192 bytes) | `executor.go` `maxStoredOutputBytes` |
| Shell (macOS/Linux) | `$SHELL` or `/bin/sh` | `executor.go` `NewExecutor()` |
| Shell (Windows) | `cmd` | `executor.go` `NewExecutor()` |
| Script extension (macOS/Linux) | `.sh` | `executor.go` `writeTempScript()` |
| Script extension (Windows) | `.bat` | `executor.go` `writeTempScript()` |

### Default Working Directory (Fallback Chain)

When a command is executed, the working directory is resolved via this fallback chain (see `execution_service.go` `resolveWorkingDir`):

1. **Per-command `workingDir`** for the current OS (from the command's `OSPathMap`)
2. **Global `defaultWorkingDir`** from app settings (Settings → General → Default Working Directory)
3. **User home directory** (via `os.UserHomeDir()`)
4. **Current working directory** (via `os.Getwd()`, fallback if home dir unavailable)
5. **OS temporary directory** (via `os.TempDir()`, last resort)

The chain guarantees a non-empty working directory for every execution.

---

## 11. Per-Environment Overrides

Cmdex does not use separate `.env.development`, `.env.production`, or `.env.test` files. Instead, configuration differences between environments are handled through the following mechanisms:

### Development vs Production

| Aspect | Development (`wails3 dev`) | Production (`wails3 build`) |
|--------|--------------------------|---------------------------|
| Frontend build | Vite dev server on `VITE_PORT` (default `9245`) | Static `dist/` output served from embedded filesystem |
| Go build flags | `DEV=true` injected by `build/config.yml` dev executes | No `DEV` flag |
| `PRODUCTION` env var | `false` (set in `build/Taskfile.yml`) | `true` (set in `build/Taskfile.yml`) |
| Wails devServerUrl | Auto-detected from running Vite process | N/A (embedded assets used) |
| File watching | Enabled — Go and frontend changes trigger rebuild | N/A |
| Log level | `warn` (configurable in `build/config.yml`) | Standard Go logging |

### Platform-Specific Behavior

The following behaviors vary by operating system at runtime (not configurable by the user):

| Behavior | macOS | Linux | Windows |
|----------|-------|-------|---------|
| Shell for script execution | `$SHELL` or `/bin/sh` with `-lc` flag | `$SHELL` or `/bin/sh` with `-lc` flag | `cmd` with `/C` flag |
| Temp script extension | `.sh` | `.sh` | `.bat` |
| Terminal emulator detection | macOS-specific app bundle paths (Terminal.app, iTerm.app, Warp.app, etc.) plus CLI terminals | Linux desktop terminals (GNOME Terminal, Konsole, xterm, etc.) | Windows Terminal, cmd, PowerShell |
| Terminal launch method | `osascript` for `.app` bundles; direct binary launch for CLI terminals | Direct binary execution | `wt`, `cmd /c start`, or `powershell -NoExit` |
| Settings shortcut | `Cmd + ,` | `Ctrl + ,` | `Ctrl + ,` |

### Build Configuration Overrides

The `Taskfile.yml` supports the following task-level variable overrides for build customization:

| Variable | Default | Description |
|----------|---------|-------------|
| `BUILD_FLAGS` | `""` | Additional Go build flags passed to `wails generate bindings` and `go build` (e.g., `-tags server`). |
| `TAG` | `cmdex:latest` | Docker image tag for `build:docker` and `run:docker` tasks. |
| `PORT` | `8080` | Host port mapping for `run:docker` task. |
| `DEV` | `""` | When set to `"true"`, activates development-optimized frontend builds with HMR support. |

