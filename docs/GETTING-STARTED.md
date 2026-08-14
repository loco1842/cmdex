<!-- generated-by: gsd-doc-writer -->

# Getting Started

This guide will walk you through setting up the Cmdex project for local development and building it from source.

## Prerequisites

Before you begin, make sure you have the following installed:

- **Go** `>= 1.26.0`
- **Node.js** `>= 20.19.0 || >=22.13.0 || >=24`
- **pnpm** (required for frontend dependencies)
- **Wails v3 CLI** (`v3.0.0-beta.8`)
- **Task** (optional, used by build scripts — install from [taskfile.dev](https://taskfile.dev))

### Platform-specific dependencies

- **Linux**: Install system libraries required for the WebKit renderer:
  ```bash
  sudo apt-get update
  sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev
  ```

- **Windows**: NSIS is required to build the installer. Install it with:
  ```powershell
  choco install nsis
  ```

- **macOS**: No extra system dependencies are required.

## Installation steps

1. **Clone the repository**
   ```bash
   git clone https://github.com/locnguyen1842/cmdex.git
   cd cmdex
   ```

2. **Install the Wails v3 CLI**
   ```bash
   go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8
   ```

3. **Install frontend dependencies**
   ```bash
   cd frontend && pnpm install
   ```

## Running in development mode

Start the application with hot-reload for the frontend:

```bash
wails3 dev
```

Or use Task (respects the project `.env` configuration):

```bash
task dev
```

> **Note:** Frontend changes are reloaded automatically. Changes to Go code require restarting the dev server.

## Building for production

### Quick build

Build a native binary for your current platform:

```bash
wails3 build
```

### Using Task (recommended)

The project includes a `Taskfile.yml` that orchestrates platform-specific builds:

| Command | Description |
|---------|-------------|
| `task build` | Build the application for the current OS |
| `task package` | Build and package a production release (installer/disk image) |
| `task package:universal` | Build a universal macOS binary (arm64 + amd64) |

The packaged artifacts are placed in the `bin/` directory.

## First-time setup

No additional configuration is required to run the app locally. On first launch, Cmdex automatically creates its local data directory and database at:

```
~/.cmdex/cmdex.db
```

All data is stored locally — no accounts, cloud services, or API keys are needed.

`.env` and `frontend/.env` are gitignored, optional, local-only files for overriding the development server port (`VITE_PORT`) — they do not exist on a fresh clone, and `task dev`/`make dev` fall back to sane built-in defaults (port `9245`) without them. The application name comes from `build/config.yml`'s `productName`, not these files. You should not need to create them for standard development.

## Common setup issues

### Port conflict (VITE_PORT=9245)

The Vite dev server runs on port **9245** by default (baked into `Taskfile.yml`/`build/Taskfile.yml` as a shell fallback: `${VITE_PORT:-9245}`), overridable via `VITE_PORT` in `.env`/`frontend/.env` or the shell environment. If that port is already in use, `wails3 dev` will fail and `task dev` will exit with an error because the frontend dev server uses `--strictPort`. To resolve this:

- Free up port 9245, or
- Set `VITE_PORT` to an available port — either in `.env`/`frontend/.env`, or ad hoc: `VITE_PORT=9246 task dev`

### Wails CLI version mismatch

This project pins Wails to **`v3.0.0-beta.8`** (as specified in `go.mod`). Installing a different version of `wails3` can cause binding generation failures or unexpected behavior at runtime. Verify your installed version:

```bash
wails3 version
```

If it does not match `v3.0.0-beta.8`, reinstall the exact version:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8
```

### Go toolchain version

The `go.mod` file declares `go 1.26.0`. An older Go toolchain may produce compilation errors or Go module resolution failures. Verify your Go version matches:

```bash
go version  # should show go1.26.x or newer
```

### Wails bindings not generated

If you encounter TypeScript errors about missing imports from `../wailsjs/go/main/App`, the Wails-generated bindings are out of date. Regenerate them manually:

```bash
wails3 generate bindings
```

This generates TypeScript bindings in `frontend/bindings/` based on the `App` and service structs in `app.go` and `*_service.go` files.

### Linux: wrong WebKitGTK version

The application requires **`libwebkit2gtk-4.1-dev`**, not `libwebkit2gtk-4.0-dev` (the older API version). Installing the wrong package will cause the application to fail on launch with a missing shared library error. The correct installation command is:

```bash
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev
```

### C compiler not found on Linux

Linux builds require CGO (the CGo bridge for the SQLite dependency). If you see errors like `CGO_ENABLED=0` or `gcc not found`, install a C compiler:

```bash
sudo apt-get install -y build-essential
```

On Docker-based cross-compilation (`task setup:docker`), the Docker image provides the toolchain automatically.

## Next steps

Once you have the project running, refer to these guides to go deeper:

- **[Development Guide](./DEVELOPMENT.md)** — Code style, build commands, branch conventions, and the PR process
- **[Architecture Overview](./ARCHITECTURE.md)** — System components, data flow, and directory structure
- **[Configuration](./CONFIGURATION.md)** — Environment variables, settings, and per-environment overrides
- **[Testing Guide](./TESTING.md)** — Manual testing workflows and the roadmap for automated tests
- **[Contributing](./CONTRIBUTING.md)** — Coding standards, PR guidelines, and how to report issues
