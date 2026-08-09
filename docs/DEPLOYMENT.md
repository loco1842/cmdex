<!-- generated-by: gsd-doc-writer -->

# Deployment Guide

This document describes how CmDex is built, packaged, and released across macOS, Windows, Linux, and (experimentally) iOS.

---

## 1. Release Overview

CmDex is a cross-platform desktop application built with **Wails v3**, **Go**, and **React**. It is distributed as native platform installers rather than a web service:

| Platform | Output Formats |
|----------|----------------|
| macOS    | `.dmg` (universal binary) |
| Windows  | `.exe` (NSIS installer) |
| Linux    | `.AppImage`, `.deb`, `.rpm` |
| iOS      | `.app` (via Xcode, experimental) |

All releases are produced via **GitHub Actions**. There is no traditional server or container deployment for the desktop GUI. A headless **server mode** (HTTP-only, no GUI) can be built separately for containerized environments.

---

## 2. CI/CD Pipeline (GitHub Actions)

Two workflows drive the build and release process:

### CI Workflow (`.github/workflows/ci.yml`)

Triggers on every push or pull request to `main`.

- **Type Check Job** (`ubuntu-latest`)
  - Installs Go (version from `go.mod`), Node.js 24, pnpm, and Linux build dependencies (`libgtk-3-dev`, `libwebkit2gtk-4.1-dev`).
  - Installs Wails v3 CLI (`v3.0.0-beta.5`).
  - Generates Wails bindings and runs `pnpm tsc --noEmit`.
  - Runs `go build ./...` to verify compilation.

- **Build Check Job** (matrix: `ubuntu-24.04`, `macos-latest`, `windows-latest`)
  - Installs platform-specific dependencies (NSIS on Windows via Chocolatey).
  - Caches the Wails CLI, frontend build artifacts (`frontend/dist`, `frontend/bindings`), and APT packages.
  - Runs `task build` to produce a working binary on each OS.

### Release Workflow (`.github/workflows/release.yml`)

Triggers on two events:
- **Tag push**: `v*.*.*` (e.g. `v1.2.0`)
- **Manual dispatch**: `workflow_dispatch` with an optional tag name for artifact naming.

Build matrix:

| OS Runner | Artifact Name | Package Task |
|-----------|---------------|--------------|
| `ubuntu-24.04` | `cmdex-linux-amd64` | `task package` |
| `macos-latest` | `cmdex-darwin-universal` | `task package:dmg:universal` |
| `windows-latest` | `cmdex-windows-amd64` | `task package` |

Steps:
1. Check out source.
2. Install Go, Node.js 24, pnpm, Task, Wails v3 CLI, and platform dependencies.
3. Build and package the application.
4. Upload artifacts (`bin/*.AppImage`, `*.deb`, `*.rpm`, `*.dmg`, `*-installer.exe`).
5. If triggered by a tag, download all artifacts, flatten them, and create a **GitHub Release** using `softprops/action-gh-release` with auto-generated release notes.

> **Note:** For manual workflow dispatches, the workflow runs `task build` instead of `task package`, so only raw binaries are produced.

---

## 3. Building Locally

### Prerequisites

- **Go** `>= 1.26.0`
- **Node.js** `24`
- **pnpm**
- **Wails v3 CLI** `v3.0.0-beta.5`
- **Task** (`taskfile.dev`) `3.x`

Install the Wails CLI:
```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.5
```

### Install Dependencies

```bash
cd frontend && pnpm install && cd ..
```

### Development Mode

```bash
# Run the app with hot-reload for frontend changes
wails3 dev -config ./build/config.yml

# Or using Task
task dev
```

### Production Build

```bash
# Build a native binary for the current platform
task build

# Package into platform-specific installers (macOS: .dmg, Windows: .exe, Linux: .AppImage/.deb/.rpm)
task package
```

Build outputs are written to the `bin/` directory.

### Cross-Compilation (Docker)

For building macOS or Linux binaries from a non-native host, or when no C compiler is available:

```bash
# One-time setup (~800MB download)
task setup:docker

# The Taskfiles automatically fall back to Docker builds when cross-compiling
```

### Server Mode (Headless)

A server-only binary (no GUI, HTTP API) can be built for container deployment:

```bash
# Build server binary
task build:server

# Build and run Docker image for server mode
task build:docker
task run:docker
```

---

## 4. Platform-specific Notes

### macOS

- **Universal Binary**: The release pipeline produces a single universal binary containing both `arm64` (Apple Silicon) and `amd64` (Intel) architectures using `lipo`.
- **Signing**: By default, the app bundle is **ad-hoc signed** (`codesign --force --deep --sign -`) during packaging. For distribution outside the Mac App Store, configure a proper Apple Developer ID:
  - Edit `build/darwin/Taskfile.yml` and set `SIGN_IDENTITY` and `KEYCHAIN_PROFILE`.
  - Run `wails3 signing credentials --apple-id ... --team-id ...` to store notarization credentials.
  - Use `task sign:notarize` to sign and notarize the `.app` bundle.
- **Gatekeeper**: Because CmDex is not distributed with a paid Apple Developer certificate, macOS may quarantine the app. Users can remove the quarantine attribute:
  ```bash
  xattr -d com.apple.quarantine /Applications/cmdex.app
  ```
- **Packaging**: The `.dmg` is created with `hdiutil` and includes a symlink to `/Applications`.

### Windows

- **Installer**: The default output is an **NSIS** installer (`.exe`). NSIS must be installed on the build machine (the CI uses Chocolatey: `choco install nsis`).
- **MSIX**: An alternative MSIX package can be created with:
  ```bash
  task package FORMAT=msix
  ```
- **Build Flags**: Production builds use `-ldflags="-w -s -H windowsgui"` to strip debug info and hide the console window.
- **CGO**: By default, Windows builds use `CGO_ENABLED=0`. When building **from a non-Windows host** (Linux/macOS) and CGO is enabled, the build uses Docker cross-compilation with Zig. On Windows hosts, setting `CGO_ENABLED=1` builds natively.
- **Code Signing**: To sign the executable or installer, configure `SIGN_CERTIFICATE` or `SIGN_THUMBPRINT` in `build/windows/Taskfile.yml`, then run:
  ```bash
  task sign
  task sign:installer
  ```

### Linux

- **Build Dependencies**: On Debian/Ubuntu, install:
  ```bash
  sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev
  ```
- **Package Formats**: The release produces:
  - `.AppImage` — portable, distribution-agnostic
  - `.deb` — Debian/Ubuntu package
  - `.rpm` — Fedora/RHEL package
  - AUR files — Arch Linux package metadata
- **Native vs Docker**: Linux builds require CGO. If a C compiler (gcc/clang) is present and the target architecture matches the host, the build runs natively. Otherwise, it falls back to the Docker cross-compilation image.
- **Code Signing**: DEB and RPM packages can be signed with a PGP key. Configure `PGP_KEY` in `build/linux/Taskfile.yml` and run:
  ```bash
  task sign:packages
  ```

---

## 5. Release Process

### Automated Release (Recommended)

1. Ensure all changes are merged into `main` and CI is green.
2. Create and push a semver tag:
   ```bash
   git tag v1.2.0
   git push origin v1.2.0
   ```
3. The **Build & Release** workflow triggers automatically.
4. Wait for all three platform jobs to complete.
5. A GitHub Release is created with auto-generated notes and all platform artifacts attached.

### Manual / Pre-release Build

1. Go to **Actions > Build & Release > Run workflow** in the GitHub UI.
2. Optionally provide a tag name (used only for artifact naming).
3. The workflow produces artifacts that can be downloaded from the workflow summary page. No GitHub Release is created for manual runs.

### Artifact Checklist

A successful release should contain the following files in the GitHub Release:

- `cmdex-darwin-universal-vX.Y.Z.dmg`
- `cmdex-linux-amd64-vX.Y.Z.AppImage`
- `cmdex-linux-amd64-vX.Y.Z.deb`
- `cmdex-linux-amd64-vX.Y.Z.rpm`
- `cmdex-windows-amd64-vX.Y.Z-installer.exe`

> **Note:** Exact artifact names may vary slightly based on the tag or dispatch input.

---

## 6. Environment Setup (Production)

For production deployment, the following environment variables and configuration settings are relevant:

- **Server Mode**: When running `task build:server` for container deployment, the Docker image exposes port `8080` and listens on `0.0.0.0` by default (set via `WAILS_SERVER_HOST` in `Dockerfile.server`). Override at runtime with `docker run -e WAILS_SERVER_HOST=...`.
- **Application Settings**: All user-facing configuration (themes, typography, locale, terminal preferences) is stored in the local SQLite database and managed through the in-app Settings modal. See [CONFIGURATION.md](CONFIGURATION.md) for the full list of configurable options.
- **Secrets**: No API keys or external service credentials are required by the application itself. Signing credentials for platform distribution (Apple Developer ID, Windows Authenticode certificate, PGP key) should be stored as CI secrets or in the system keychain via `wails3 setup signing`.

<!-- VERIFY: Server mode port and host defaults are from Dockerfile.server — confirm these match production deployment expectations -->

---

## 7. Rollback Procedure

No automated rollback pipeline is configured. If a release introduces a regression:

1. **GitHub Releases**: Locate the previous stable release on the GitHub Releases page and direct users to download it.
2. **Docker (server mode)**: Redeploy the previous Docker image tag:
   ```bash
   docker pull cmdex:<previous-tag>
   docker run --rm -p 8080:8080 cmdex:<previous-tag>
   ```
3. **Local installs**: Reinstall from the previous release's platform package (`.dmg`, `.exe`, `.AppImage`, `.deb`, `.rpm`).

<!-- VERIFY: GitHub Releases page URL is specific to the repository and not discoverable from source -->

---

## 8. Monitoring

No application-level monitoring (Sentry, Datadog, OpenTelemetry, etc.) is currently integrated. The application is a local-first desktop app and does not phone home or report telemetry.

For server mode deployments, standard container monitoring tools (Docker health checks, log aggregation) can be used — configure health checks via the hosting platform.

<!-- VERIFY: Confirm no monitoring/telemetry has been added since last audit -->

---

## 9. iOS (Experimental)

Basic iOS device tasks are available in the build system (see `ios:device:list` and `ios:run:device` in `build/Taskfile.yml`). This target requires a configured Xcode project at `build/ios/xcode/` and appropriate signing profiles. iOS is not part of the automated CI pipeline and is currently **experimental**.

---

## Related Files

- `.github/workflows/ci.yml` — Continuous integration
- `.github/workflows/release.yml` — Release automation
- `Taskfile.yml` — Top-level build orchestration
- `build/Taskfile.yml` — Shared build tasks (frontend, server, Docker, iOS)
- `build/darwin/Taskfile.yml` — macOS build, sign, and package tasks
- `build/windows/Taskfile.yml` — Windows build, sign, and package tasks
- `build/linux/Taskfile.yml` — Linux build, sign, and package tasks
- `build/docker/Dockerfile.server` — Server mode Docker image
- `build/docker/Dockerfile.cross` — Cross-compilation Docker image
- `build/config.yml` — Wails dev/build configuration
- `wails.json` — Wails application configuration
- `docs/CONFIGURATION.md` — Application settings reference
