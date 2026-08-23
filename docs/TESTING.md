# Testing Guide

This document covers the current testing status, manual testing workflows, and a roadmap for introducing automated tests in the Cmdex project.

## 1. Current Testing Status

**Cmdex has automated tests, but coverage is uneven across the stack.**

- **Go backend:** `*_test.go` files exist across the root package (migrations, terminal sessions, PTY backends, execution, output capture/ANSI, shell integration — see [Section 7](#7-running-tests) for the full list).
- **Frontend:** No `.test.ts`/`.test.tsx` unit tests exist yet in `frontend/src/`, but Playwright e2e specs exist under `frontend/e2e/tests/*.spec.ts`, run via the `test:e2e` script in `frontend/package.json`.
- **CI:** `.github/workflows/ci.yml`'s `typecheck` job (build check, lint, type check) runs on every push/PR. The `test`, `test-windows`, and `build-check` jobs run the Go and Playwright suites but are gated to `workflow_dispatch` — they do **not** run automatically on push/PR.

So automated push/PR verification is build + lint + type check only; the Go and Playwright suites must be triggered manually via `workflow_dispatch`, or run locally with `make test` / `go test ./...` / `cd frontend && pnpm test:e2e`.

## 2. Manual Testing Guide

### Running the Application

Start the development server with hot-reload for the frontend:

```bash
wails3 dev
# or
make dev
# or
task dev
```

Build a production binary:

```bash
wails3 build
# or
make build
# or
task build
```

### Static Checks

Before committing, run the static checks that mirror CI:

```bash
# Go compile check
go build ./...

# TypeScript type check
cd frontend && pnpm tsc --noEmit
```

### Manual QA Workflow

1. Start the app with `wails3 dev`.
2. Reset all data via the UI (or delete `~/.cmdex/cmdex.db`) to test a fresh install state.
3. Exercise the features in the checklist below.
4. Test on your target platform (macOS, Linux, or Windows), as terminal integration and shell execution vary by OS.

## 3. Planned Testing Strategy

The project is structured to support three layers of automated testing:

### Go Unit & Integration Tests

Use Go's built-in `testing` package and standard `reflect.DeepEqual` (or `github.com/google/go-cmp`) for assertions.

**Priority targets:**

| Module | File | Why test |
|--------|------|----------|
| Script parsing | `script.go` | Pure functions (`ExtractTemplateVars`, `ReplaceTemplateVars`, `MergeDetectedVars`, `ParseScriptBody`) are deterministic and easy to unit test. |
| Executor helpers | `executor.go` | `EvalDefaults` (CEL expression evaluation), `stripShebang`, `shellQuoteDir`, `shellDialectFor`/`buildCommandLine` (per-shell cd prefix + line-submit key, see `TestBuildCommandLine`). Pure and deterministic. |
| Database layer | `db.go` | CRUD operations, migrations, and FTS search. Use a temporary SQLite file or `:memory:` database in tests. |
| Service layer | `*_service.go` | Thin wrappers around `db` methods; test input validation and error handling. `execution_service_test.go` covers working-directory resolution and `FinalCmd` construction. |
| Terminal sessions | `terminal_service.go` | Session lifecycle, concurrency, and the `MaxSessions` limit. Use the mock backend (`pty_backend_mock.go`) to avoid spawning real shells. |
| Output capture | `terminal_capture.go`, `ansi.go` | OSC 133 marker scanning across chunk boundaries, and ConPTY wrap-artifact removal — both are byte-level and highly regression-prone. |
| Shell integration | `shell_integration.go` | That the embedded scripts materialize correctly (notably that zsh's dot-prefixed files survive the `//go:embed all:` directive). |

**Example test file pattern:**

```go
// script_test.go
package main

import "testing"

func TestExtractTemplateVars(t *testing.T) {
    got := ExtractTemplateVars("echo {{greeting}} {{name}}")
    want := []string{"greeting", "name"}
    // assert equality
}
```

Run Go tests:

```bash
go test ./...
```

### Frontend Unit Tests

The frontend uses **React 19**, **Vite**, and **TypeScript**. The recommended test stack is **Vitest** (aligns with Vite) plus **React Testing Library**.

**Priority targets:**

| Module | File | Why test |
|--------|------|----------|
| Template variables | `frontend/src/utils/templateVars.ts` | Pure functions (`extractTemplateVarNames`, `mergeDetectedVariables`, `buildVariablesFromScript`). |
| Tab draft utilities | `frontend/src/utils/tabDraft.ts` | `draftsEqual`, `cloneDraft`, `draftFromCommand` contain core state logic. |
| Type utilities | `frontend/src/types.ts` | `getCommandDisplayTitle` and other data transforms. |

**Suggested devDependencies to add:**

```bash
cd frontend
pnpm add -D vitest @testing-library/react @testing-library/jest-dom jsdom
```

Then add a `test` script to `frontend/package.json`:

```json
"scripts": {
  "test": "vitest run",
  "test:watch": "vitest"
}
```

### E2E / Integration Tests

End-to-end testing for a Wails v3 desktop app is more involved because the runtime requires a native webview.

**Recommended approach (incremental):**

1. **Backend integration tests:** Test the full Go stack (DB -> Services) without the UI by calling service methods directly in `*_test.go` files.
2. **Frontend component tests:** Mount key components (e.g., `CommandDetail`, `VariablePrompt`) with mocked Wails bindings.
3. **E2E (future):** Evaluate <!-- VERIFY: Wails v3 E2E tooling or Playwright with a server-mode build --> if native E2E becomes necessary.

## 4. Testing Checklist for Common Features

Use this checklist during manual QA before a release.

### Categories

- [ ] Create a new category.
- [ ] Edit a category name and color.
- [ ] Delete a category; verify its commands become uncategorized.

### Commands

- [ ] Create a command with a title, description, tags, and script body.
- [ ] Verify `{{variable}}` syntax is auto-detected in the script body.
- [ ] Save the command and confirm it appears in the sidebar.
- [ ] Edit an existing command and save changes.
- [ ] Delete a command and confirm it disappears from the sidebar and tabs.
- [ ] Reorder commands via drag-and-drop within a category.
- [ ] Move a command to a different category via drag-and-drop.

### Variables & Execution

- [ ] Define variables with descriptions, examples, and default values.
- [ ] Test CEL default expressions: `now()`, `env("HOME")`, `date("2006-01-02")`.
- [ ] Run a command inline; verify stdout/stderr appear in the Output pane.
- [ ] Verify exit codes are captured (0 for success, non-zero for failure).
- [ ] Run a command in an external terminal; verify the terminal opens and executes.
- [ ] Confirm execution history is saved and visible in the History pane.

### Variable Presets

- [ ] Create a preset from the current variable values.
- [ ] Apply a preset and verify fields populate correctly.
- [ ] Rename a preset.
- [ ] Delete a preset.
- [ ] Reorder presets via drag-and-drop.

### Settings

- [ ] Switch themes (dark, light, custom) and verify CSS variables update.
- [ ] Change UI font and monospace font; verify font family changes.
- [ ] Switch density (compact, comfortable, spacious) and verify spacing.
- [ ] Change locale/language and verify UI strings reload.
- [ ] Open the Settings window from the menu and close it; reopen without errors.
- [ ] Restart the app and confirm all settings persist.

### Search & Command Palette

- [ ] Use the command palette (`Cmd/Ctrl + P`) to find and open a command.
- [ ] Use the sidebar search to filter commands by title or content.
- [ ] Verify full-text search (FTS5) returns relevant results.

### UI / Edge Cases

- [ ] Open a new command tab; verify the dirty indicator appears after editing.
- [ ] Close a dirty tab and confirm the discard/save prompt appears.
- [ ] Switch between tabs and verify output/history pane state is restored.
- [ ] Use keyboard shortcuts (e.g., `Cmd/Ctrl + Enter` to run, `Cmd/Ctrl + S` to save).
- [ ] Resize the window and verify minimum dimensions are respected.

## 5. How to Add Tests

### Adding a Go Unit Test

1. Create a file named `<module>_test.go` in the same package (e.g., `script_test.go` next to `script.go`).
2. Write table-driven tests for pure functions.
3. For DB tests, initialize `DB` with a temporary path or `:memory:` SQLite connection, then call `migrate()`.
4. Run the test:

```bash
go test ./... -v
go test -run TestExtractTemplateVars ./...
```

### Adding a Frontend Unit Test

1. Install Vitest and Testing Library if not already present:

```bash
cd frontend
pnpm add -D vitest @testing-library/react @testing-library/jest-dom jsdom
```

2. Create `frontend/src/utils/templateVars.test.ts`:

```typescript
import { describe, it, expect } from 'vitest';
import { extractTemplateVarNames } from './templateVars';

describe('extractTemplateVarNames', () => {
  it('detects unique variables in order', () => {
    expect(extractTemplateVarNames('echo {{name}} {{name}}')).toEqual(['name']);
  });
});
```

3. Add a `test` script to `frontend/package.json`:

```json
"scripts": {
  "test": "vitest run",
  "test:watch": "vitest"
}
```

4. Run the tests:

```bash
cd frontend && pnpm test
```

### Updating CI to Run Tests

Go tests and Playwright e2e already run via the `test` job. Once Vitest is configured for frontend unit tests, add a step to that same job:

```yaml
- name: Frontend unit tests
  run: cd frontend && pnpm test
```

Platform coverage is already partly addressed: a dedicated `test-windows` job runs `go test -race ./...` on `windows-latest`, which is where the PTY layer diverges most (ConPTY, wrap artifacts, quoting). macOS is still only covered by `build-check`, so macOS-specific terminal behavior is not exercised by CI.

## 6. Test Framework and Setup

### Go Backend

The Go backend uses the standard library [`testing`](https://pkg.go.dev/testing) package with no external assertion libraries. The test database helper uses [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) (a CGo-free SQLite driver) with an in-memory database (`:memory:`) for fast, isolated test runs.

**Current test file:**

| File | Tests | Coverage |
|------|-------|----------|
| `db_test.go` | `TestFreshDBMigrations`, `TestExistingDBIdempotent`, `TestRollbackTo` | Schema migrations, idempotent re-runs, and rollback logic |

The test helper `newTestDB(t)` in `db_test.go` creates a fresh in-memory SQLite connection per test, ensuring full isolation.

**No test configuration files exist** — Go tests use the project's `go.mod` for dependency resolution and require no additional config.

### Frontend

No JavaScript/TypeScript test framework is currently installed. The `frontend/package.json` `devDependencies` include ESLint and TypeScript for static analysis, but no test runner (`vitest`, `jest`, etc.) or component testing library (`@testing-library/react`, etc.).

**Missing test infrastructure:**
- No `vitest.config.ts` or `jest.config.ts`
- No `jsdom` or `happy-dom` environment configured
- No `test` script in `frontend/package.json` `scripts`

The frontend statically checks code quality via:
```bash
cd frontend && pnpm lint        # ESLint
cd frontend && pnpm tsc --noEmit # TypeScript type check
```

## 7. Running Tests

### Go Tests

```bash
# Run all Go tests
go test ./...

# Run all tests with verbose output
go test ./... -v

# Run a specific test by name
go test -run TestFreshDBMigrations ./...

# Run a specific test with verbose output
go test -run TestFreshDBMigrations -v ./...
```

**Expected output** (current state):
```
ok      cmdex   0.234s
```

All tests live in the root package (`db_test.go`, `execution_service_test.go`, `terminal_service_test.go` and its race/stress/max-sessions variants, `pty_backend_*_test.go`, `terminal_capture_test.go`, `ansi_test.go`, `shell_integration_test.go`, `pty_env_test.go`). The `migrations/` package has no test files of its own — migrations are covered indirectly by `db_test.go`.

Some tests are build-tagged or skipped by platform: the stress and max-sessions variants are `//go:build darwin` and use the mock PTY backend, while `TestTerminalShutdown`/`TestTerminalExit` are Windows-skipped because they call the raw `ptyStart` helper with Unix shell syntax. `TestBuildCommandLine` (`execution_service_test.go`) is platform-independent and covers POSIX/cmd.exe/PowerShell command-line construction on every OS; `shell_integration_test.go`'s `TestPwshIntegration_*` suite includes Windows-only ConPTY execution tests (`BuiltCommandLineActuallyExecutes`, `LFAloneDoesNotSubmitCommandLine`, `WorkingDirPrefixChangesDirectory`, `WorkingDirPrefixShortCircuitsOnBadDir`) gated on `pwshRealPTYSkipReason` rather than a build tag.

`make test` runs `go test ./...` followed by the frontend Playwright e2e suite (`cd frontend && pnpm test:e2e`); there is no `task test` target in `Taskfile.yml`.

### Frontend Tests

Playwright is installed and runs the e2e suite (`cd frontend && pnpm test:e2e`, config at `frontend/e2e/playwright.config.ts`). There is no **unit** test runner yet. Once Vitest is set up (see [Section 3](#3-planned-testing-strategy)), the expected commands would be:

```bash
cd frontend && pnpm test         # Run once
cd frontend && pnpm test:watch   # Watch mode
```

### Static Analysis (Pre-Commit)

Linting and type checking serve as the primary automated quality gates:

```bash
# Go compile check
go build ./...

# TypeScript type check
cd frontend && pnpm tsc --noEmit

# Frontend lint
cd frontend && pnpm lint
```

## 8. Coverage Requirements

No coverage thresholds are currently configured for either Go or frontend code. Coverage is not enforced in CI.

### Ad-hoc Coverage Measurement

Go supports on-demand coverage via the `-cover` flag:

```bash
# Coverage summary for all packages
go test ./... -cover

# Detailed coverage profile
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

**To add coverage enforcement in the future:**

For Go, set coverage thresholds using `-coverpkg` and parse `coverage.out` in CI:

```bash
go test ./... -coverprofile=coverage.out -coverpkg=./...
go tool cover -func=coverage.out | tail -1 | awk '{print $3}'  # extracts total %
```

For the frontend (once Vitest is configured), a coverage provider can be added:

```bash
cd frontend
pnpm add -D @vitest/coverage-v8
```

Then configure in `vitest.config.ts`:

```typescript
test: {
  coverage: {
    provider: 'v8',
    thresholds: {
      lines: 70,
      branches: 70,
      functions: 70,
      statements: 70,
    },
  },
}
```

## 9. CI Integration

### Current CI Pipeline

Test execution **is part of the CI configuration, but not of every automated run**. `.github/workflows/ci.yml` consists of four jobs; only `typecheck` runs automatically on push/PR — `test`, `test-windows`, and `build-check` are gated to `workflow_dispatch` (manual trigger) and must be run explicitly:

| Job | Runner | Trigger | What It Does |
|-----|--------|---------|--------------|
| `typecheck` | ubuntu-24.04 | push, PR, workflow_dispatch | Lint frontend, `go build ./...`, `golangci-lint`, generate Wails bindings, `tsc --noEmit` |
| `test` | ubuntu-24.04 | workflow_dispatch only | `go test -race ./...` and the frontend Playwright e2e suite (`pnpm test:e2e`); blocks the run on failure |
| `test-windows` | windows-latest | workflow_dispatch only | `go test -race ./...`, including the real ConPTY backend tests; blocks the run on failure |
| `build-check` | matrix: ubuntu-24.04, macos-latest, windows-latest | workflow_dispatch only | Cross-platform build via `task build` |

> Note that CI runs `go test -race`, while `make test` runs plain `go test`. A data race in the terminal session code can therefore pass locally and fail in CI — run `go test -race ./...` yourself when touching `terminal_service.go` or `terminal_capture.go`.

**Key CI details:**
- Wails CLI version: `v3.0.0-beta.12` (pinned via `WAILS_VERSION` env var in CI)
- Go version: read from `go.mod` (currently `1.26.0`)
- Node version: `24` (pinned via `NODE_VERSION` env var in CI)
- Package manager: `pnpm` 11 (pinned via `PNPM_VERSION`; installed with `pnpm/action-setup@v6`)
- `golangci-lint` and `pnpm lint` block the `typecheck` job on any finding
- CI never invokes the Makefile — `make check`/`fmt`/`lint`/`test` are local conveniences that mirror the CI steps

**e2e gotcha:** `frontend/e2e/mocks/runtime.ts` mocks `@wailsio/runtime` by hardcoding each generated binding's numeric `$Call.ByID` method hash. If you regenerate bindings and those IDs shift, the mock silently stops matching — tests fail with `[e2e mock] no handler for method ID …` console warnings rather than an obvious error. Update the handler table whenever you change a bound method signature.

### Adding Tests to CI

Go tests and the Playwright e2e suite already run in the `test` job, but only on manual `workflow_dispatch` — not on every push/PR. What's still missing is frontend **unit** tests (Vitest is not yet configured — see [Section 5 — Updating CI to Run Tests](#5-how-to-add-tests)): once added, wire `cd frontend && pnpm test` into the `test` job alongside the existing steps.
