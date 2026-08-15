# SPEC: Real Windows PTY (ConPTY) Backend for TerminalService

## Status

**Historical — implemented.** This spec predates implementation and is kept as the original scoping document; it does not reflect the shipped design in every detail (notably the `ptyBackend`/`ptyProcess` interface — see "Superseded by implementation" below). See `tasks/plan.md` and `tasks/todo.md` for what was actually built and verified.

## Background

`TerminalService`'s PTY layer is split behind a build-tagged `ptyBackend` interface (`pty_backend.go`). The darwin/linux implementation (`pty_backend_unix.go`, `creackPtyBackend`) is real and works, backed by `github.com/creack/pty`. The Windows implementation (`pty_backend_windows.go`, `conptyBackend`) **had never been implemented** at spec time — `Start` and `Resize` unconditionally returned `"Windows PTY support not yet implemented"` errors; only `Kill` (via `taskkill /F /T /PID <pid>`) had real logic, and it had never been exercised on a real Windows process. It is now fully implemented, backed by `github.com/charmbracelet/x/conpty` — see `AGENTS.md`'s Tests section.

This gap is already documented in `AGENTS.md`, `CLAUDE.md`, and in full detail in `.planning/milestones/v2.1-phases/25-polish-integration/CHECKPOINT.md` (see "Windows conpty verification" section, decisions D-11 through D-14), which explicitly scoped real Windows conpty support as future work. That checkpoint's "Future work" section is the starting point for this spec, not a constraint we're bound to verbatim (see Tech Stack below on the library choice).

User-reported symptom: on Windows, terminal sessions open but input/output does not work — consistent with the stub silently failing or a session that never gets a working PTY handle.

## Objective

Implement a real Windows ConPTY-backed `ptyBackend` so that `TerminalService` terminal sessions work on Windows the same way they already work on macOS/Linux: a session opens, the user's shell (`pwsh`/`powershell`/`cmd`, per existing `detectShell()` logic) starts, keystrokes are sent to the shell, output streams back, resize works, and closing/killing the session cleans up the OS process.

**Out of scope:**
- Any change to `terminal_service.go`'s session orchestration logic (mutex handling, max-sessions guard, cwd inheritance, event naming) — the `ptyBackend` interface contract already isolates this from the OS layer, and the checkpoint confirms no other `terminal_service.go` changes should be needed once Windows implements `Start`/`Resize` for real.
- Any change to the frontend (`Terminal.tsx`, xterm rendering) — this is purely a Go backend fix.
- Signal/process-group semantics beyond what's needed for reliable shell termination (Windows has no `SIGHUP`/`SIGKILL`/process groups as first-class concepts; `taskkill /F /T /PID` is the existing, accepted approach and is not being redesigned unless the chosen conpty library requires otherwise).

## Acceptance Criteria

1. On a `windows-latest` CI runner, `go test ./...` passes, including new Windows-tagged tests that exercise the real `conptyBackend` (not the darwin-only mock).
2. `conptyBackend.Start` spawns a real ConPTY-attached process for the detected shell, returns a `ptyHandle` that supports read/write, and the returned process (a `ptyProcess`, not an `*exec.Cmd` — ConPTY cannot be driven through `os/exec`, see `tasks/plan.md`) can be waited on and killed.
3. `conptyBackend.Resize` actually resizes the underlying pseudo-console (not a no-op stub).
4. `conptyBackend.Kill` reliably terminates the shell process and any children it spawned; existing `taskkill`-based `killProcessGroup` logic is reused unless the chosen library provides an equivalent that's proven better.
5. `GOOS=windows go build ./...` continues to pass (cross-compile from any dev machine, since no Windows dev machine is available locally).
6. CI gains real Windows runtime test coverage: a `windows-latest` job runs `go test ./...` (extending or paralleling the existing `test` job, which today only runs on `ubuntu-24.04`) — see Tech Stack/CI section.
7. `AGENTS.md` and `CLAUDE.md`'s existing "Windows conpty backend is currently a stub" notes are updated to reflect the new reality (or removed if fully resolved).
8. No regression in existing darwin/linux PTY behavior or in the 12+ existing multi-session tests in `terminal_service_test.go`.

## Tech Stack

- **Language/runtime**: Go 1.26.x, matching `go.mod` — no change.
- **ConPTY library**: left open for evaluation during implementation (not pre-committed to `github.com/UserExistsError/conpty`, despite that being CHECKPOINT.md's prior suggestion — the conpty Go ecosystem has been unstable, so re-survey current options before pinning one). Candidates to evaluate:
  - `github.com/UserExistsError/conpty`
  - `github.com/philippgille/gogosseract`-adjacent or other actively maintained conpty wrappers, if any surface during evaluation
  - A hand-rolled `golang.org/x/sys/windows` ConPTY implementation, if no maintained wrapper is suitable
  - Evaluation criteria: actively maintained (commits/releases within the last ~1-2 years), compatible with Go 1.26, minimal/no CGo requirement (project has no CGo dependencies today — `modernc.org/sqlite` is deliberately pure Go), and an API that maps cleanly onto the existing `ptyBackend` interface (`Start`, `Resize`, `Kill`) without leaking library-specific types into `terminal_service.go`.
  - Use the `source-driven-development` skill/Context7 to verify the chosen library's actual current API before writing code against it, since no Context7 documentation was found for Windows Go conpty bindings at spec time — don't rely on memorized API shapes.
- **No other stack changes.** No new frontend dependencies, no new services, no schema changes.

## CI Changes

- Add real Windows Go test execution. The existing `test` job (`.github/workflows/ci.yml`) runs `go test ./...` only on `ubuntu-24.04`, gated to `workflow_dispatch`. Extend this coverage to `windows-latest` — either:
  - (a) add `windows-latest` to a matrix on the existing `test` job (frontend e2e/Playwright steps would need `if: runner.os == 'Linux'` guards, since e2e is not being extended to Windows in this spec), or
  - (b) add a new, narrower `test-windows` job that just does Go setup + `go test ./...` on `windows-latest`, skipping the pnpm/Playwright steps entirely.
  - Prefer (b) for simplicity: it avoids conditionally guarding every frontend step in the existing job and keeps the Windows job fast (Go-only).
- The existing `build-check` job's `windows-latest` matrix entry (full `wails3 build`/packaging) is unaffected and stays as-is.
- Trigger: match the existing `test` job's `workflow_dispatch`-only gating, unless the user wants Windows Go tests to run on every push/PR (flag this as a follow-up question if it comes up during planning — not decided here).

## Project Structure

No new top-level structure. Changes concentrated in:
- `pty_backend_windows.go` — replace stub `Start`/`Resize` with real ConPTY calls; keep `Kill`/`killProcessGroup` unless the library changes what's optimal.
- `go.mod`/`go.sum` — add the chosen conpty dependency (Windows-only import, but Go doesn't support OS-conditional `go.mod` entries — the dependency will be present for all platforms at the module level, same as how `pty_backend_unix.go`'s `github.com/creack/pty` already is).
- A new `//go:build windows` test file (e.g. `pty_backend_windows_test.go`) exercising the real backend — mirroring the shape of `TestTerminalService_CreateSession` and friends, per CHECKPOINT.md's future-work item 4.
- `.github/workflows/ci.yml` — new/extended Windows test job.
- `AGENTS.md`, `CLAUDE.md` — update the stub note.
- `.planning/milestones/v2.1-phases/25-polish-integration/CHECKPOINT.md` — leave as historical record (do not rewrite history); note resolution in a new checkpoint or follow-up doc if the project's convention calls for one — confirm with user during planning rather than assumed here.

## Code Style

Follow existing conventions already documented in `CLAUDE.md`:
- gofmt/tabs, `make fmt` / `make lint` (golangci-lint v2, same config as CI) must pass clean.
- Match `pty_backend_unix.go`'s existing shape: a struct implementing `ptyBackend`, package-level helper functions preserved only if legacy test code still references them (check before assuming — CHECKPOINT.md notes this pattern was intentional for `pty_backend_unix.go`/`pty_backend_windows.go` both).
- No new abstractions beyond what's needed to satisfy the existing `ptyBackend`/`ptyHandle` interfaces — don't introduce a second interface layer over the conpty library.
- Comments only where genuinely non-obvious (e.g. why a particular conpty quirk is worked around), matching the project's "no explanatory comments for obvious code" convention.

## Testing Strategy

- **Primary bar**: `windows-latest` CI (new/extended job above) running real `go test ./...` against the real `conptyBackend`. This is the actual verification mechanism, since there is no local Windows machine available.
- **New Windows-tagged tests** (`//go:build windows`) should cover, at minimum:
  - Session start: shell process spawns, handle is readable/writable.
  - Write → read round-trip: a simple command's output is observable.
  - Resize: no error, and (if the library exposes it) the pseudo-console reports the new size.
  - Kill: process actually terminates (check via `cmd.Wait()` or process lookup), including any child processes it spawned.
  - Bad working directory: verify behavior matches the unix backend's graceful-degradation pattern (retry with cleared `cmd.Dir`) if applicable to conpty, or document why Windows can't/shouldn't replicate it.
- **Do not modify** the existing `//go:build darwin` mock-backed tests (`pty_backend_mock.go`, stress test, max-sessions test) — those stay as orchestration-level coverage and are out of scope here.
- **Cross-compile check** (`GOOS=windows go build ./...` from darwin) remains a fast local sanity check during development, but is not sufficient alone — CI runtime tests are the real bar per the acceptance criteria.

## Boundaries

**Always do:**
- Verify the chosen conpty library's actual current API via Context7/official docs before writing code against it (per `source-driven-development`) — don't hand-roll from memory given known ecosystem instability.
- Keep `terminal_service.go` untouched unless CI reveals the `ptyBackend` contract itself needs to change (would be a surprise — flag it immediately if so, don't silently expand scope).
- Run `make check` and `make lint` before considering any change complete, in addition to the new Windows CI job.
- Preserve the existing `Kill`/`taskkill` behavior unless there's a concrete, demonstrated reason to change it.

**Ask first about:**
- Whether to extend the existing `test` job's matrix vs. add a separate `test-windows` job (spec recommends the latter, but confirm before touching `ci.yml`).
- Whether Windows Go tests should run on every push/PR or stay `workflow_dispatch`-gated like the rest of the `test` job.
- Any change to `killProcessGroup`/signal semantics beyond what's strictly needed.
- Whether to write a new CHECKPOINT-style doc under `.planning/` recording this work, given the project's existing convention of documenting phase completions there.

**Never do:**
- Modify frontend terminal code (`Terminal.tsx`, `TerminalTabBar.tsx`, xterm wiring) as part of this fix.
- Introduce CGo dependencies (breaks the project's pure-Go build story, notably `modernc.org/sqlite`).
- Silently drop or weaken the existing darwin/linux PTY test coverage while adding Windows coverage.
- Rewrite the historical CHECKPOINT.md's acknowledged-gap section to make it look like the gap was always going to be closed this way — it's a historical record of a real scoping decision.

## Open Questions (for planning phase)

- Exact conpty library choice — deferred to implementation-time evaluation per Tech Stack section above.
- `test-windows` CI job trigger (workflow_dispatch vs. push/PR) — needs explicit user decision before touching `ci.yml`.
- Whether a new `.planning/` checkpoint doc should accompany this work, per repo convention.
