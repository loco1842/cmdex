# Contributing to CmDex

Thank you for your interest in contributing to CmDex! This document provides guidelines and instructions to help you get started.

## Welcome & Code of Conduct

We welcome contributions of all kinds — bug fixes, new features, documentation improvements, and issue reports. By participating in this project, you agree to act respectfully and constructively toward all community members.

## How to Contribute

### Reporting Bugs

If you find a bug, please open an issue on our [GitHub Issues](https://github.com/locnguyen1842/cmdex/issues) page. Include:

- A clear description of the problem
- Steps to reproduce
- Expected vs. actual behavior
- Your operating system and CmDex version

### Suggesting Features

Feature requests are welcome! Open an issue and describe:

- The use case
- The proposed solution
- Any alternatives you've considered

### Pull Requests

We accept pull requests for bug fixes and features. If you're planning a significant change, please open an issue first to discuss your approach.

## Development Setup

### Prerequisites

- **Go** `>= 1.26.0`
- **Node.js** `>=20.19.0 \|\| >=22.13.0 \|\| >=24`
- **pnpm** (latest)
- **Wails v3 CLI** `v3.0.0-beta.12`
- **Task** (`go-task`) — optional but recommended for builds

### Installing Wails v3 CLI

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.12
```

### Clone & Install

```bash
git clone https://github.com/locnguyen1842/cmdex.git
cd cmdex
cd frontend && pnpm install && cd ..
```

### Running in Development Mode

```bash
wails3 dev
```

Or using Task:

```bash
task dev
```

### Build Commands

| Command | Description |
|---------|-------------|
| `wails3 build` | Production build |
| `wails3 dev` | Development mode with hot-reload |
| `task build` | Build via Task (cross-platform) |
| `task dev` | Dev mode via Task |
| `make check` | `go build ./...` + TypeScript type check |
| `make lint` | `golangci-lint run` + ESLint (both blocking in CI) |
| `make fmt` | `golangci-lint fmt` + `pnpm lint:fix` |
| `make test` | Go tests, then the Playwright e2e suite |
| `make generate` | Regenerate Wails bindings after Go service changes |
| `cd frontend && pnpm lint` | Run ESLint on its own |
| `cd frontend && pnpm tsc --noEmit` | TypeScript type check on its own |

## Pull Request Process

1. Fork the repository and create a branch from `main`.
2. Make your changes.
3. If you changed a Go service signature, run `make generate` and commit the regenerated `frontend/bindings/`.
4. Ensure the project builds, lints, type-checks, and passes tests:
   - `make check` — `go build ./...` + `pnpm tsc --noEmit`
   - `make lint` — `golangci-lint run` + ESLint (**both block CI on any finding**)
   - `make test` — `go test ./...` + the Playwright e2e suite
5. Update documentation if your changes affect user-facing behavior.
6. Open a pull request against the `main` branch with a clear description.

For every pull request, CI runs type checks plus Go and frontend lint on Ubuntu, the Go test suite with `-race` on both Ubuntu and Windows, the Playwright e2e suite on Ubuntu, and build verification across Ubuntu, macOS, and Windows.

## Coding Standards

### Go

- Follow standard Go conventions. Run `make fmt` before committing — it applies `golangci-lint fmt` (goimports + golines) to Go and `pnpm lint:fix` to the frontend.
- Keep the backend code in the project root (e.g. `app.go`, `db.go`, `executor.go`, `models.go`); schema migrations go in the `migrations/` package.
- Read-style bound methods log with `fmt.Println` and return empty slices; mutations return `(T, error)` and propagate. Match that convention rather than introducing a logging library.

### TypeScript / React

- Use TypeScript for all frontend code.
- The frontend uses React 19+, Tailwind CSS v4, and shadcn/ui components (New York style).
- Keep UI state logic centralized in `App.tsx` where possible.
- Update CSS variables in `style.css` rather than hardcoding colors.

### General

- Write clear, descriptive commit messages.
- Keep changes focused — one logical change per pull request.

## Commit Message Guidelines

We recommend following conventional commit format:

```
<type>: <short summary>

[optional body]

[optional footer]
```

Common types:

- `feat:` — new feature
- `fix:` — bug fix
- `docs:` — documentation changes
- `style:` — formatting, missing semicolons, etc.
- `refactor:` — code restructuring without behavior change
- `test:` — adding or updating tests
- `chore:` — build process or auxiliary tool changes

Example:

```
feat: add variable preset support

Adds the ability to save and switch between named variable value sets.
```

## Reporting Issues

Please report issues via [GitHub Issues](https://github.com/locnguyen1842/cmdex/issues).

When reporting bugs, include:

1. **Description** — What went wrong?
2. **Reproduction steps** — How can we reproduce it?
3. **Expected behavior** — What did you expect to happen?
4. **Environment** — OS, CmDex version, and any relevant context.

For feature requests, describe the problem you're trying to solve and your proposed solution.

---

By contributing to CmDex, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
