# Testing Guide

This document covers the current testing status, manual testing workflows, and a roadmap for introducing automated tests in the Cmdex project.

## 1. Current Testing Status

**Cmdex has automated tests across the stack.**

- **Go backend:** `*_test.go` files exist across the root package (migrations, terminal sessions, PTY backends, execution, output capture/ANSI, shell integration — see [Section 7](#7-running-tests) for the full list).
- **Frontend unit tests:** Vitest specs under `frontend/src/**/*.test.ts`, covering pure logic in `src/utils` (`tabDraft`, `templateVars`, `tab`, `path`) and `src/lib` (`shortcuts`). Run via the `test` script in `frontend/package.json`.
- **Frontend e2e tests:** Playwright specs under `frontend/e2e/tests/*.spec.ts` — tabs, presets, variables/execution, the command palette and keyboard shortcuts, sidebar, settings, error-toast coverage, an i18n raw-key guard, terminal, commands, categories, themes, a smoke test, and a mock/bindings drift guard (`mock-contract.spec.ts`). Run via the `test:e2e` script.
- **CI:** `.github/workflows/ci.yml`'s `typecheck` job (build check, lint, type check) and `e2e` job (Vitest + Playwright) both run on every push/PR. The `go-test`, `test-windows`, and `build-check` jobs run the Go `-race` suites and cross-platform build verification but are gated to `workflow_dispatch` — they do **not** run automatically on push/PR.

So automated push/PR verification covers build + lint + type check + frontend unit/e2e tests; only the Go `-race` suites and cross-platform build verification must be triggered manually via `workflow_dispatch`, or run locally with `make test` / `go test ./...` / `cd frontend && pnpm test` / `cd frontend && pnpm test:e2e`.

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

## 3. Testing Strategy

The project has three layers of automated testing, all implemented:

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

The frontend uses **React 19**, **Vite**, and **TypeScript**. Unit tests use **Vitest** (`frontend/vitest.config.ts`, `environment: 'node'` — no jsdom/React Testing Library, since every covered function is pure and touches no DOM; UI behavior is covered by the Playwright e2e suite instead).

**Covered today** (`frontend/src/**/*.test.ts`, colocated with the code they test):

| Module | File | What's covered |
|--------|------|-----------------|
| Tab draft utilities | `frontend/src/utils/tabDraft.ts` | `draftsEqual` (every field, incl. tag order-sensitivity and the `revealed` flags), `cloneDraft`, `draftFromCommand`, `makePlaceholderCommand`. |
| Template variables | `frontend/src/utils/templateVars.ts` | `extractTemplateVarNames` (word-chars-only regex), the `mergeDetectedVariables` (keeps orphans) vs. `buildVariablesFromScript` (drops orphans) asymmetry, `normalizeVariablesForCompare`, `variableDefinitionsToPrompts`. |
| Tab id / display title | `frontend/src/utils/tab.ts` | `isNewCommandTabId`/`createNewTabId`, `getCommandDisplayTitle`'s fallback chain (title → shebang-stripped script → 50-char truncation). |
| OS path map helpers | `frontend/src/utils/path.ts` | `normalizeOS`, `getOSPath`, `setOSPath` (deletes the key on an empty path rather than storing `""`), `shortenPath`. |
| Keyboard shortcut labels | `frontend/src/lib/shortcuts.ts` | `shortcutLabelParts`/`shortcutLabelString`/`isCmdOrCtrl` on both Mac and non-Mac user agents (module-scoped `isMac` requires `vi.resetModules()` + a stubbed `navigator` per case). |

Run with:

```bash
cd frontend && pnpm test         # run once
cd frontend && pnpm test:watch   # watch mode
```

### E2E / Integration Tests

End-to-end testing for a Wails v3 desktop app would normally be complicated by the runtime requiring a native webview — Cmdex sidesteps that by running the real React app under plain Vite/Chromium with `@wailsio/runtime` aliased to an in-memory mock (`frontend/e2e/mocks/runtime.ts`), rather than the native Wails webview. That mock:

- Dispatches by the generated bindings' numeric `$Call.ByID` method hash, named once in a `METHOD_IDS` table (`mock-contract.spec.ts` asserts this stays in sync with `frontend/bindings/cmdex/*.js`).
- Supports one-shot/sticky RPC fault injection (`window.__cmdexE2E.failNext(method, message)` / `setFailure(method, message)`) for methods the real backend can actually fail on — refused for the handful that are log-and-return-empty in production (`GetCategories`, `GetCommands`, `GetCommandsByCategory`, `SearchCommands`, `GetPresets`, `GetVariables`, `GetSettings`) or that never reject at all (`RunCommand`).
- Exposes a call log (`window.__cmdexE2E.callLog`) for asserting exactly which RPCs fired.
- Wraps every `Events.Emit` in the same `WailsEvent` envelope (`{ name, data, sender }`) the real runtime does, and provides `emitPtyOutput`/`emitPtyExit`/`emitPtyCleared` helpers to simulate backend-driven terminal events.

`e2e/fixtures.ts` provides shared Playwright fixtures (`seed`, `gotoApp`, `gotoSettings`, `toast`, `visibleTabShell`, `pressCmdOrCtrl`) that every spec builds on instead of hand-rolling navigation and toast assertions per file.

Backend integration tests still live separately in `*_test.go` files (calling service methods directly, no UI); there are no frontend component tests (e.g. React Testing Library mounting `CommandDetail` in isolation) — UI behavior is covered end-to-end via Playwright instead.

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

Vitest is already configured (`frontend/vitest.config.ts`, `test` script in `frontend/package.json`) — just add a colocated `*.test.ts` file next to the module it covers:

```typescript
// frontend/src/utils/templateVars.test.ts
import { describe, it, expect } from 'vitest';
import { extractTemplateVarNames } from './templateVars';

describe('extractTemplateVarNames', () => {
  it('detects unique variables in order', () => {
    expect(extractTemplateVarNames('echo {{name}} {{name}}')).toEqual(['name']);
  });
});
```

Run it:

```bash
cd frontend && pnpm test
```

### Adding a Playwright e2e Test

Add a `*.spec.ts` file under `frontend/e2e/tests/`, importing `test`/`expect` from `../fixtures` (not `@playwright/test` directly) so seeding, navigation, and toast assertions share the existing fixtures:

```typescript
import { test, expect } from '../fixtures';
import { sel } from '../utils/selectors';

test('does the thing', async ({ page, seed, gotoApp }) => {
  await seed({ commands: [/* ... */] });
  await gotoApp();
  // ...
});
```

To exercise a failure path, inject a fault into the mock rather than hand-writing a new handler:

```typescript
await page.evaluate(() => window.__cmdexE2E!.failNext('UpdateCommand', 'disk full'));
```

`failNext`/`setFailure` refuse (with a console warning) for methods the real backend can never fail on (log-and-return-empty reads, and `RunCommand`) — see `e2e/mocks/runtime.ts`'s `NEVER_REJECTS`.

Run it:

```bash
cd frontend && pnpm test:e2e
```

### CI Wiring

Both `pnpm test` and `pnpm test:e2e` already run in CI's `e2e` job, which triggers on every push/PR (unlike `go-test`/`test-windows`/`build-check`, which are `workflow_dispatch`-gated). Platform coverage is already partly addressed there too: a dedicated `test-windows` job runs `go test -race ./...` on `windows-latest`, which is where the PTY layer diverges most (ConPTY, wrap artifacts, quoting). macOS is still only covered by `build-check`, so macOS-specific terminal behavior is not exercised by CI.

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

**Unit tests:** Vitest (`frontend/vitest.config.ts`), configured with `environment: 'node'` — no jsdom/`@testing-library/react`, since the covered modules (`src/utils`, `src/lib`) are pure functions with no DOM dependency. Specs are colocated `*.test.ts` files (`include: ['src/**/*.test.ts']`). Run with `cd frontend && pnpm test` (`pnpm test:watch` for watch mode).

**E2E tests:** Playwright (`frontend/e2e/playwright.config.ts`), Chromium only, against a real Vite dev server (`e2e/vite.config.ts`) with `@wailsio/runtime` aliased to `e2e/mocks/runtime.ts`. A pinned 1440×900 viewport avoids `ResizablePanel`'s sidebar auto-collapse (`innerWidth <= 600`) and the terminal panel's viewport-derived default height from interfering with unrelated specs. `frontend/e2e/tsconfig.json` (run via `pnpm typecheck:e2e`) type-checks the specs and mock, which the main `tsconfig.json` (`pnpm tsc --noEmit`) does not cover.

The frontend also statically checks code quality via:
```bash
cd frontend && pnpm lint            # ESLint
cd frontend && pnpm tsc --noEmit    # TypeScript type check (src/, wailsjs/)
cd frontend && pnpm typecheck:e2e   # TypeScript type check (e2e/)
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

`make test` runs `go test ./...`, then the frontend Vitest unit suite (`cd frontend && pnpm test`), then the Playwright e2e suite (`cd frontend && pnpm test:e2e`); there is no `task test` target in `Taskfile.yml`.

### Frontend Tests

```bash
cd frontend && pnpm test         # Vitest unit tests, run once
cd frontend && pnpm test:watch   # Vitest, watch mode
cd frontend && pnpm test:e2e     # Playwright e2e suite (config: frontend/e2e/playwright.config.ts)
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

`.github/workflows/ci.yml` consists of five jobs. `typecheck` and `e2e` run automatically on every push/PR; `go-test`, `test-windows`, and `build-check` are gated to `workflow_dispatch` (manual trigger) and must be run explicitly:

| Job | Runner | Trigger | What It Does |
|-----|--------|---------|--------------|
| `typecheck` | ubuntu-24.04 | push, PR, workflow_dispatch | Lint frontend, `go build ./...`, `golangci-lint`, generate Wails bindings, `tsc --noEmit` |
| `e2e` | ubuntu-24.04 | push, PR, workflow_dispatch | Generate Wails bindings, frontend Vitest unit suite (`pnpm test`), e2e specs' own type check (`pnpm typecheck:e2e`), Playwright e2e suite (`pnpm test:e2e`); uploads the HTML report on failure |
| `go-test` | ubuntu-24.04 | workflow_dispatch only | `go test -race ./...`; blocks the run on failure |
| `test-windows` | windows-latest | workflow_dispatch only | `go test -race ./...`, including the real ConPTY backend tests; blocks the run on failure |
| `build-check` | matrix: ubuntu-24.04, macos-latest, windows-latest | workflow_dispatch only | Cross-platform build via `task build` |

> Note that CI runs `go test -race` (in `go-test`/`test-windows`), while `make test` runs plain `go test`. A data race in the terminal session code can therefore pass locally and fail in CI — run `go test -race ./...` yourself when touching `terminal_service.go` or `terminal_capture.go`. `go-test` is `workflow_dispatch`-gated, so this only surfaces when someone triggers it manually (e.g. before a release) — it does not block PRs.

**Key CI details:**
- Wails CLI version: `v3.0.0-beta.12` (pinned via `WAILS_VERSION` env var in CI)
- Go version: read from `go.mod` (currently `1.26.0`)
- Node version: `24` (pinned via `NODE_VERSION` env var in CI)
- Package manager: `pnpm` 11 (pinned via `PNPM_VERSION`; installed with `pnpm/action-setup@v6`)
- `golangci-lint` and `pnpm lint` block the `typecheck` job on any finding
- CI never invokes the Makefile — `make check`/`fmt`/`lint`/`test` are local conveniences that mirror the CI steps

**e2e gotcha (mostly closed):** `frontend/e2e/mocks/runtime.ts` mocks `@wailsio/runtime` by dispatching on each generated binding's numeric `$Call.ByID` method hash (named once in its `METHOD_IDS` table, not scattered as bare numeric literals). Regenerating bindings can still shift those hashes, but `mock-contract.spec.ts` now parses `frontend/bindings/cmdex/*.js` and asserts the mock's table matches exactly — a mismatch fails that spec loudly instead of the old silent `[e2e mock] no handler for method ID …` console warning. Update `METHOD_IDS` (and add a handler under the matching name in `handlersByName`) whenever you change a bound method signature.

### Adding Tests to CI

Vitest and the Playwright e2e suite both already run in the `e2e` job on every push/PR. To add a Go check to a PR-blocking job (rather than the `workflow_dispatch`-gated `go-test`), add a step to the `typecheck` or `e2e` job directly — be mindful that doing so slows down every push/PR, which is exactly why `go test -race ./...` (slower, and the PTY suite occasionally needs real timing) was kept manual-only instead.
