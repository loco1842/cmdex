# Development Guide

This guide covers how to set up, build, and develop the **Cmdex** application locally.

---

## 1. Development Environment Setup

### Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | `>= 1.26.0` | Backend services and Wails runtime |
| Node.js | `>= 20.19.0 || >=22.13.0 || >=24` | Frontend build tooling (Vite, TypeScript) |
| pnpm | latest | Frontend package manager |
| Wails CLI | `v3.0.0-beta.12` | Desktop app framework and binding generator |

### Installing Wails v3

Install the pinned version — it must match `WAILS_VERSION` in `.github/workflows/ci.yml`/`release.yml`; a mismatched version can cause binding generation failures (see `docs/GETTING-STARTED.md`).

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.12
```

Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is on your `PATH` so the `wails3` command is available.

### Clone and Install Dependencies

```bash
git clone <repo-url>
cd cmdex
pnpm install          # Installs frontend dependencies
```

The frontend lives in `./frontend` and is managed with `pnpm`. The Go module is at the repository root.

---

## 2. Project Structure

```
cmdex/
├── main.go                    # Application entry point; window & menu setup
├── app.go                     # App lifecycle, settings window management
├── models.go                  # Go structs (Command, Category, VariableDefinition, etc.)
├── command_service.go         # CRUD for commands, categories, presets; FTS search
├── execution_service.go       # Resolves variables, dispatches the command to the active PTY
├── settings_service.go        # User preferences persistence
├── importexport_service.go    # Data import / export
├── event_service.go           # Wails event name constants
├── terminal_service.go        # Multi-session PTY terminals (MaxSessions = 10)
├── pty_backend*.go            # PTY abstraction: creack/pty (Unix), ConPTY (Windows), mock
├── pty_env.go                 # TERM/COLORTERM/LANG for launchd-started GUI processes
├── shell_integration.go       # OSC 133 activation via embedded shell startup scripts
├── terminal_capture.go        # OSC 133 marker scanner; GetLastOutput
├── ansi.go                    # ANSI stripping + ConPTY wrap-artifact removal
├── executor.go                # Shell selection, shebang/quoting helpers, CEL eval
├── db.go                      # SQLite database layer
├── script.go                  # Script parsing and variable substitution
├── migrations/                # Versioned schema migrations (0001…0010)
├── shell-integration/         # Embedded bash / zsh / fish / pwsh startup scripts
├── wails.json                 # Stale Wails v2 leftover — not read by the v3 toolchain
├── frontend/
│   ├── package.json           # Frontend manifest (React 19, Vite 8, Tailwind v4)
│   ├── vite.config.ts         # Vite configuration with Wails plugin
│   ├── src/
│   │   ├── main.tsx           # Entry point (routes between main app and settings window)
│   │   ├── App.tsx            # Main application shell, tabs, modals, state
│   │   ├── types.ts           # TypeScript interfaces matching Go models
│   │   ├── i18n.ts            # i18next setup
│   │   ├── style.css          # Global styles, CSS variables, theming
│   │   ├── components/        # UI components (Sidebar, CommandDetail, etc.)
│   │   ├── hooks/             # Custom React hooks
│   │   ├── utils/             # Utility functions (tab drafts, template vars)
│   │   └── wails/             # Event name constants and Wails helpers
│   └── bindings/              # Auto-generated Wails TS bindings (do not edit manually)
└── build/                     # Platform-specific build configs (Taskfile, packaging)
```

**Key files to know:**

- `main.go` — Defines the main window dimensions, menu bar, and registers all seven backend services.
- `app.go` — Holds `ServiceStartup` / `ServiceShutdown`, creates the settings window, and owns the package-level `db` / `executor` / `terminalSvc` / `wailsApp` variables every other service reads.
- `execution_service.go` — Where "run a command" actually lives. Note that it dispatches into the terminal rather than executing anything itself; `executor.go`, despite the name, executes nothing.
- `terminal_service.go` — The largest and most subtle file: session lifecycle, PTY read loops, and the concurrency rules around `sessionState`.
- `frontend/src/App.tsx` — Central hub for application state, tab management, terminal sessions, and modal routing.
- `frontend/src/types.ts` — Single source of truth for TypeScript shapes that mirror Go structs.

---

## 3. Frontend Development

### Vite Dev Server

The frontend is built with **Vite 8** and uses the official `@wailsio/runtime` plugin.

```bash
cd frontend
pnpm dev
```

The dev server runs on the port defined by `VITE_PORT` (default `9245`). When launched via `wails3 dev`, Wails bridges the Vite dev server into the desktop window so changes hot-reload instantly.

### HMR & Fast Refresh

- React components support Fast Refresh out of the box.
- CSS changes in `style.css` or component-scoped styles apply immediately.
- **Go changes are NOT hot-reloaded** — you must restart the `wails3 dev` process after editing backend code.

### Key Frontend Scripts

| Command | Description |
|---------|-------------|
| `pnpm dev` | Start Vite dev server |
| `pnpm build` | Production build (TypeScript compile + Vite bundle) |
| `pnpm build:dev` | Development build (unminified) |
| `pnpm preview` | Preview the production build locally |
| `pnpm lint` | Run ESLint on all frontend source files |
| `pnpm lint:fix` | Run ESLint and auto-fix issues where possible |

### Path Aliases

`vite.config.ts` registers `@/` as an alias to `frontend/src/`:

```ts
import { Button } from '@/components/ui/button';
```

### Production Bundle (Desktop)

Wails loads frontend assets from the local embed/asset server, not the network. Prefer a **simple bundle** over web-style vendor chunking:

- Do **not** add Vite `codeSplitting` / vendor `manualChunks` just to silence size warnings.
- `chunkSizeWarningLimit` is raised to `1500` in `frontend/vite.config.ts` — a ~1MB main chunk is fine for desktop.
- Lazy-load only heavy features that are not needed at first paint (e.g. `Terminal` / xterm in `App.tsx`; `App` vs `SettingsPage` entry points in `main.tsx`).

---

## 4. Backend Development

### Go Services Architecture

Cmdex uses Wails v3 **Services**. Each service is a struct registered in `main.go`:

| Service | File | Responsibility |
|---------|------|----------------|
| `App` | `app.go` | Lifecycle, settings window, `GetOS`, `PickDirectory` |
| `CommandService` | `command_service.go` | CRUD for categories, commands, presets; FTS search |
| `ExecutionService` | `execution_service.go` | Variable resolution, working dir, dispatching to the active PTY |
| `SettingsService` | `settings_service.go` | Read/write user preferences |
| `ImportExportService` | `importexport_service.go` | JSON import / export |
| `EventService` | `event_service.go` | Event name constants |
| `TerminalService` | `terminal_service.go` | Multi-session PTY terminals, output capture |

All services receive a `ServiceStartup` context for initialization and `ServiceShutdown` for cleanup.

### Rebuilding Wails Bindings

Whenever you add or change a **public method** on a service struct, regenerate the TypeScript bindings:

```bash
wails3 generate bindings
```

This updates `frontend/bindings/` so the frontend can call the new Go methods with full type safety.

> **Tip:** `wails3 dev` automatically regenerates bindings on startup, but running the command above is useful when you want to update types without launching the full app.

### Database

- SQLite via `modernc.org/sqlite` (pure Go, no CGO).
- Data is stored in the user's home directory at `~/.cmdex/cmdex.db`.
- See `db.go` for query logic and the migration runner; the migrations themselves live in the `migrations/` package.
- Schema changes need a new `migrations/NNNN_description.go` appended to the ordered `Migrations` slice. SQLite has no `ALTER COLUMN`, so most migrations recreate the table and then rebuild the FTS index and triggers. During local development you can instead delete `~/.cmdex/cmdex.db`.

---

## 5. Adding Features

### The Wails Bindings Pattern

To expose a new backend capability to the frontend:

1. **Add a method to the appropriate service** in Go:

   ```go
   // command_service.go
   func (s *CommandService) DuplicateCommand(id string) (Command, error) {
       // ... implementation
   }
   ```

2. **Regenerate bindings**:

   ```bash
   wails3 generate bindings
   ```

3. **Import and call from React**:

   ```ts
   import { DuplicateCommand } from '../bindings/cmdex/commandservice';

   const copy = await DuplicateCommand(originalId);
   ```

### Data Flow (Frontend → Backend → Frontend)

```
React Component
      │
      ▼
  Wails Binding (auto-generated TS)
      │
      ▼
  Go Service Method
      │
      ▼
  SQLite (db.go)
      │
      ▼
  Response → React State Update
```

For streaming data, the backend emits events via `wailsApp.Event.Emit(...)` and the frontend listens with `Events.On(...)`. Command output does not follow the request/response path above at all: `RunCommand` writes the resolved line into the active terminal session's PTY, and the output arrives asynchronously on that session's `pty-output:<sessionId>` event, straight into xterm.js.

### Adding a New Field to a Model

When adding a field to `Command`, `Category`, or any shared model:

1. Update the Go struct in `models.go`.
2. Add a migration in `migrations/` and update the CRUD logic and queries in `db.go`.
3. Update `Create` and `Update` method signatures in the relevant service (e.g., `command_service.go`).
4. Run `wails3 generate bindings` and commit the regenerated `frontend/bindings/`.
5. Update the corresponding TypeScript interface in `frontend/src/types.ts`.
6. If the field is editable per tab, extend `TabDraft` and the helpers in `frontend/src/utils/tabDraft.ts` — otherwise dirty-state detection will ignore it.
7. Update the UI components (`CommandDetail.tsx`, `CategoryEditor.tsx`) to display/edit the field.
8. Update `App.tsx` where Create/Update calls are invoked.

### Window Configuration

Main window dimensions, title, background color, and macOS-specific options are set in `main.go` inside the `application.New(...)` block. The settings window is created programmatically in `app.go`.

---

## 6. Code Style & Conventions

### Go

- Standard Go formatting. Run `make fmt` before committing: it applies `golangci-lint fmt` (the formatters under `.golangci.yml`'s `formatters:` block — `goimports`, `golines`) to Go **and** `pnpm lint:fix` to the frontend.
- `make lint` runs `golangci-lint run` **and** `pnpm lint` — the same configs CI runs, and both fail the run on any finding. Not wired into `make check`. Use `make lint-fix` to auto-fix what each tool can.
- Services are named `XxxService` with exported methods in PascalCase.
- Errors are wrapped with `fmt.Errorf("...: %w", err)`.
- Database access is centralized in `db.go`; services call `db.*` rather than issuing SQL directly.

### TypeScript / React

- ESLint is configured in `frontend/eslint.config.mjs` using `@eslint/js`, `typescript-eslint`, and `eslint-plugin-react-hooks`. Run `pnpm lint` before committing frontend changes.
- Use functional components and hooks.
- Custom hooks live in `frontend/src/hooks/`.
- Utility functions live in `frontend/src/utils/`.
- Components use the `@/` alias for cross-imports.
- Theming is driven by CSS variables in `style.css`. Always update variables rather than hard-coding colors.

### Tailwind CSS v4

The project uses Tailwind v4 with the new `@tailwindcss/vite` plugin. Styles are configured via CSS in `style.css` rather than a traditional `tailwind.config.js`.

---

## 7. Useful Commands

### Daily Development

| Command | Description |
|---------|-------------|
| `wails3 dev` | Run the desktop app in development mode (auto-starts Vite, enables HMR) |
| `wails3 generate bindings` | Regenerate TypeScript bindings from Go services |
| `wails3 build` | Build a production binary |

### Makefile Shortcuts

| Command | Description |
|---------|-------------|
| `make dev` | Alias for `wails3 dev` |
| `make build` | Alias for `wails3 build` |
| `make generate` | Alias for `wails3 generate bindings` |
| `make check` | Compile Go + type-check TypeScript |
| `make fmt` | `golangci-lint fmt` (`goimports` + `golines`) **+** `pnpm lint:fix` |
| `make lint` | `golangci-lint run` **+** `pnpm lint` (blocking — same configs CI uses) |
| `make lint-fix` | `golangci-lint run --fix` **+** `pnpm lint:fix` |
| `make test` | Run Go tests (`go test ./...`), then the frontend Playwright e2e suite |
| `make clean` | Remove `bin/` and `frontend/dist`, then restore the tracked `frontend/dist/.gitkeep` placeholder |

### Taskfile (Cross-platform builds)

| Command | Description |
|---------|-------------|
| `task dev` | Dev mode with explicit config and port |
| `task build` | Platform-specific production build |
| `task package` | Package the app for distribution |
| `task run` | Run the compiled binary |

### Renaming the App

The binary/package name (`cmdex`) exists as independent, manually-synced copies in four places: `Taskfile.yml` (`APP_NAME`), `build/darwin/Info.plist` (`CFBundleExecutable`), `build/linux/nfpm/nfpm.yaml` (binary path), and `build/windows/nsis/wails_tools.nsh` (`INFO_PROJECTNAME`). Editing `Taskfile.yml`'s `APP_NAME` alone will silently break Linux/Windows packaging and the macOS `.app` bundle launch, since those three files won't match the new binary name.

To rename the app, do all of this together, in one commit:

1. Update `APP_NAME` in `Taskfile.yml`.
2. Update `info.productName` (and `info.productIdentifier` if needed) in `build/config.yml`.
3. Regenerate the platform-specific files from the new name: `task common:update:build-assets`.
4. Verify the regenerated `Info.plist`, `nfpm.yaml`, and `wails_tools.nsh` all reflect the new name, then commit everything together.

### Frontend-only Checks

```bash
cd frontend
pnpm lint           # Run ESLint
pnpm lint:fix       # Auto-fix ESLint issues where possible
pnpm tsc --noEmit   # Type-check without emitting
```

---

## Known Limitations

- The terminal output panel renders plain text stdout. It does **not** support interactive shells (e.g., `vim`, `htop`, REPLs) or advanced ANSI color sequences.
- Go code changes require a full restart of `wails3 dev`; only the frontend benefits from HMR.

---

## 8. Branch Conventions

The default branch is `main`. All development work happens on feature branches created from `main`.

Branch names follow a conventional-commit-style prefix:

| Prefix | Usage |
|--------|-------|
| `feat/` | New features (e.g., `feat/execution-working-dir`) |
| `fix/` | Bug fixes (e.g., `fix/parse-variables-duplicated`) |
| `refactor/` | Code restructuring without behavior change |
| `chore/` | Maintenance, tooling, dependency updates |
| `ci/` | CI/CD configuration changes |
| `docs/` | Documentation-only changes |
| `lint/` | Linting and style enforcement changes |

Use lowercase kebab-case for the description: `feat/add-command-preset-support`.

---

## 9. Pull Request Process

1. **Fork the repo** and create a feature branch from `main`.
2. **Make your changes**, following the code style conventions in [Section 6](#6-code-style--conventions).
3. **Verify before pushing:**
   ```bash
   go build ./...                     # Go compiles
   cd frontend && pnpm lint           # ESLint passes
   cd frontend && pnpm tsc --noEmit   # TypeScript type-checks
   ```
   Or use the `make check` shortcut to run both language checks at once:
   ```bash
   make check
   ```
4. **Update documentation** if your changes affect user-facing behavior or public APIs.
5. **Open a pull request against `main`** with a clear description of the change and the motivation behind it.

### CI Checks

Every pull request triggers the [CI workflow](.github/workflows/ci.yml), but only two of its jobs actually run automatically:

| Job | What it checks | Platform | Trigger |
|-----|---------------|----------|---------|
| **Type check** | Go compilation, ESLint, TypeScript `tsc --noEmit`, Wails bindings generation | Ubuntu | every push/PR |
| **e2e** | Frontend Vitest unit tests + Playwright e2e suite (Chromium) | Ubuntu | every push/PR |
| **go-test** | `go test -race ./...` | Ubuntu | manual (`workflow_dispatch`) |
| **test-windows** | `go test -race ./...`, including the real ConPTY backend | Windows | manual (`workflow_dispatch`) |
| **Build check** | `task build` cross-platform build verification | Ubuntu, macOS, Windows | manual (`workflow_dispatch`) |

The two automatic checks must pass before a PR can be merged; the manual jobs are triggered explicitly (e.g. before a release) rather than gating every PR. The CI caches Go modules, pnpm dependencies, Wails CLI, and platform-specific build tools (GTK, NSIS) to keep run times fast.

