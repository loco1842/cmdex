# Todo: Real Windows ConPTY Backend for TerminalService

See `tasks/plan.md` for full context, the `ptyBackend` contract, and rationale.

## Phase 0 — Build the feedback loop

- [x] **0.1** Add `test-windows` CI job to `.github/workflows/ci.yml`
  - Acceptance: new job runs on `windows-latest`, does checkout → setup-go (`go-version-file: go.mod`) → create `frontend/dist/.gitkeep` → `go test ./...`. No pnpm/Playwright steps.
  - Verify: trigger via `workflow_dispatch`, confirm the job appears and executes.
  - **Done:** added `test-windows` job to `.github/workflows/ci.yml` (checkout → setup-go → `frontend/dist/.gitkeep` → `go test ./...`, `workflow_dispatch`-gated like `test`). Triggered on real Windows CI (see below) — confirmed working.
- [x] **0.2** Guard Unix-only test assumptions so Windows CI is honest
  - `TestTerminalExit`: uses `-c`, `sleep 60`/`exit 0` — not valid on Windows shells.
  - `TestTerminalShutdown`: uses `proc.Signal(syscall.Signal(0))` — meaningless on Windows.
  - All real-backend tests will fail against the still-stubbed backend — skip with `runtime.GOOS == "windows"` + reason, matching precedent at `terminal_service_test.go:40`/`:54` and the belt-and-braces pattern in `terminal_service_max_sessions_test.go`.
  - Acceptance: Windows CI green with explicit, reasoned skips; macOS/Linux test count unchanged.
  - Verify: `go test ./...` on macOS (same pass count as before); Windows CI run green.
  - **Done:** single DRY skip added inside `newTestTerminalService` (all 21 real-backend test call sites funnel through it — covers `TestTerminalStart/Write/Resize/Shutdown/Exit` and all 14 `TestTerminalService_*` multi-session tests in one place, including the `TestTerminalShutdown`/`TestTerminalExit` Unix-specific args since those tests never reach that code once skipped). Separately fixed `TestTerminalDetectShell`'s pre-existing wrong assertion (compared against bare shell names; `detectShell()` returns `exec.LookPath`'s absolute path for pwsh/powershell) — now compares basenames. `TestPtyStart_RejectsInvalidDimensions` needed no change: the stub already errors unconditionally, so it passes vacuously today.
  - **Fixed after first real CI run caught a gap:** the first Windows run (workflow run 31872382345) failed — 5 tests in `execution_service_test.go` construct `TerminalService` via `testWithTerminalSvc`/direct `ServiceStartup` rather than `newTestTerminalService`, so the guard above didn't cover them. `ServiceStartup` swallows `CreateSession` failure ("graceful degradation" — logs and returns `nil`), so the existing `t.Skipf(err != nil)` never fired; tests ran against the stub and failed on the resulting empty session state. Added the same `runtime.GOOS == "windows"` skip to `testWithTerminalSvc` and to `TestTerminalService_ServiceStartupAssignsTerminalSvc` directly (commit `33ba61a`).
  - Verified: `go build ./...`, `go vet ./...`, `go test ./...` (63 passed, same as baseline) on macOS; `GOOS=windows go build/vet ./...` clean; `GOOS=windows go test -c` produces a working test binary; `make lint` 0 issues.
  - **→ Checkpoint 0 CONFIRMED on real CI** — workflow run [31872608196](https://github.com/loco1842/cmdex/actions/runs/31872608196): `Test (Windows)`, `Test`, `Type check`, and all three `Build check` platforms (ubuntu/macos/windows) all green.

## Phase 1 — Interface refactor (platform-neutral, no behavior change)

- [ ] **1.1** Introduce `ptyProcess` interface; rewire all consumers
  - `pty_backend.go`: add `ptyProcess{Pid() int; Wait() (int, error); Exited() bool}`; change `ptyBackend.Start`/`Kill` signatures to use it instead of `*exec.Cmd`.
  - `pty_backend_unix.go`: add `execProcess{cmd *exec.Cmd}`; `creackPtyBackend.Start` wraps it; `Kill` type-asserts (mirror existing `Resize` pattern).
  - `pty_backend_mock.go`: add `mockProcess` wrapping the existing `sleep 0.05` cmd.
  - `pty_backend_windows.go`: update stub signatures only (still "not implemented").
  - `terminal_service.go`: `sessionState.cmd` → `proc ptyProcess`; `monitorExit` uses `proc.Wait()` (drop `errors.As(&exec.ExitError{})`); the three `oldCmd.ProcessState == nil` guards become `!oldProc.Exited()`.
  - Acceptance: no behavior change on macOS/Linux; full suite passes; `GOOS=windows go build ./...` exits 0; `make check` + `make lint` clean.
  - Verify: `go test ./...`, `GOOS=windows go build ./...`, `make check`, `make lint`; manually run `wails3 dev` and confirm terminal open/type/resize/close still work on macOS.
- [ ] **1.2** Fix the pre-existing double-`Wait()` race
  - `monitorExit` (`terminal_service.go:572`) and `killProcessGroup` (`pty_backend_unix.go:143`) both call `cmd.Wait()` on the same process — forbidden by `os/exec`, can corrupt `ProcessState`.
  - Fix in `execProcess`: single `sync.Once`-guarded `Wait`, result broadcast via closed channel + stored value; `killProcessGroup`'s SIGKILL escalation awaits the shared result instead of calling `Wait()` itself. Preserve SIGHUP → 2s → SIGKILL timing (`ptyKillTimeout`).
  - Acceptance: `go test -race ./...` clean; kill/close/restart behavior and SIGKILL escalation unchanged.
  - Verify: `go test -race ./...`; new focused test calling `Wait()` from two goroutines, asserting both get the same exit code.
  - **→ Checkpoint 1**

## Phase 2 — ConPTY spike (validate before committing)

- [ ] **2.1** Minimal ConPTY vertical slice on real Windows CI
  - Library: **`github.com/charmbracelet/x/conpty` v0.2.0** (decided — see `tasks/plan.md` "Library choice"; `UserExistsError/conpty` rejected because its `Close()` destroys the process handle its own `Wait()` depends on, which directly breaks contract 5/8/9).
  - New `//go:build windows` test file: start a shell, write a command, read output, resize, close, check exit code.
  - Must answer on real hardware (acceptance criteria per risk, ranked by the design review):
    1. **Teardown/deadlock risk** (highest probability): 200 `CreateSession`/`CloseSession` cycles + 50 `Stop`/`Start` cycles under `-race`; each `CloseSession` returns in < 3s; `runtime.NumGoroutine()` within +2 of baseline after a 5s settle; no orphaned `conhost.exe`/`OpenConsole.exe` in `tasklist` afterward.
    2. **Exit-code fidelity / auto-restart storm risk**: `pwsh -NoLogo -Command "exit 7"` → `Wait()` returns exactly `(7, nil)` once; clean interactive `exit` → `(0, nil)`, zero restarts in the next 3s; `Stop()` mid-session → zero `pty-exit` events; external `taskkill` on the shell → exactly one restart.
    3. **Working-directory / argv fidelity risk**: session with `dir = t.TempDir()` + `$PWD.Path` echo confirms cwd (assert the dir-fallback retry did NOT silently fire); repeat forcing `cmd` + `%CD%` (this is the bare-`"cmd"` `lookExtensions` landmine — see below); confirm `pwsh` starts with `-NoLogo` honored (no banner in first 500ms); confirm `Resize(120,40)` → `$Host.UI.RawUI.WindowSize` shows `Width=120,Height=40`.
    4. *(stretch)* **Write-blocking risk**: paste 1MB into a `pwsh` sitting at `Read-Host`; a concurrent `Resize` on that session must still return within 500ms (Write runs under `ss.mu` — a full pipe would wedge the session).
  - Acceptance: all four questions answered by the Windows CI run.
  - Verify: Windows CI run log. If risk 1 or 2 comes back bad, stop and reconsider before Phase 3 — these are correctness-blocking, not polish.
  - **→ Checkpoint 2** (re-confirm direction with user only if the spike contradicts the design review's analysis)

## Phase 3 — Full Windows implementation

- [ ] **3.1** Implement real `conptyBackend` + `conptyProcess` in `pty_backend_windows.go`
  - **Fix the bare-`"cmd"` landmine first**: `charmbracelet/x/conpty`'s internal `lookExtensions` resolves a bare program name relative to the working directory. `detectShell()` returns the bare string `"cmd"` when neither `pwsh` nor `powershell` is found. With any non-empty `dir`, `Spawn` would fail resolution, silently trigger the dir-fallback retry, and every `cmd.exe` session would lose its configured working directory. Fix: `exec.LookPath(shellPath)` to an absolute path *before* calling `Spawn`.
  - `Start`: build argv (`[shellPath, shellFlag if non-empty] + extraArgs`, skip empty flag — `Spawn` takes argv natively, no quoting needed); move `resolvePtyWorkingDir` to `pty_backend.go` (it's pure `os.Stat`/`os.UserHomeDir`, already platform-neutral) and reuse it + the retry-with-empty-dir pattern; env via `buildPtyEnv(os.Environ())`; `conpty.New(cols, rows, 0)` — **note `New`/`Resize` take `(width, height)` == `(cols, rows)`, an extra place the rows/cols transpose can bite** distinct from the `Start`/`Resize` arg-order inversion already tracked.
  - `Resize`: real `cp.Resize(cols, rows)` via `ResizePseudoConsole`; type-assert the handle, fast path, must not call back into `TerminalService`.
  - **`conptyHandle.Read`**: do NOT trust the library's blocking `Read` to unblock on `Close()` — neither library's pipe breaks on child exit, and closing a handle with a pending synchronous `ReadFile` is undefined behavior. Implement a dedicated pump goroutine driving the blocking read into a channel; `Read` selects on that channel or a `done` channel closed by `Close()`. Never return `(0, nil)`; translate `ERROR_BROKEN_PIPE`/`ERROR_OPERATION_ABORTED`/`ERROR_INVALID_HANDLE` to `io.EOF`.
  - **`conptyHandle.Close`**: idempotent (`sync.Once`), closes `done` first (unblocks `Read` immediately), pre-terminates the child (`TerminateProcess`) so `ClosePseudoConsole` has nothing to wait for, then calls `cp.Close()` with a bounded timeout (reuse `ptyKillTimeout`'s 2s) — log-and-leak rather than hang the caller if it doesn't return in time (this is on the critical path of `Stop`/`CloseSession`/`ServiceShutdown`).
  - **`conptyProcess.Wait`**: `sync.Once`-guarded `WaitForSingleObject` + `GetExitCodeProcess` on the handle `Spawn` returned; close the handle only in the `finish()` step that runs *after* the wait completes, so no concurrent caller can be blocked on a handle another goroutine closes. On any syscall error, report `(0, nil)` rather than a non-zero/error — contract 9 says a wrong signal here causes an infinite restart loop, and a missed single auto-restart is the safer failure mode.
  - **`conptyProcess.kill`** (used by `ptyBackend.Kill`): hold the process's mutex across the `taskkill` call — closing the handle before `taskkill` runs lets Windows recycle the PID and `taskkill` could hit an unrelated process. Resolve `taskkill.exe` explicitly from `%SystemRoot%\System32` (not via `PATH` — lint/security-scan flag, `exec.Command("taskkill", ...)` is PATH-hijackable); set `CREATE_NO_WINDOW` so no console flash on session close; must not call `Wait()` itself (contract 7).
  - Preserve package-level `ptyStart`/`ptyResize`/`killProcessGroup` functions for existing test-code compatibility (per `pty_backend_windows.go:41`'s existing comment).
  - Acceptance: Windows CI runs the real backend; `GOOS=windows go build ./...` + `make lint` clean.
  - Verify: Windows CI.
  - **Fix in passing** (pre-existing, found by review, not introduced here): `terminal_service_test.go:41` asserts `detectShell()` returns a bare string (`"pwsh"`/`"powershell"`/`"cmd"`), but the function actually returns `exec.LookPath`'s absolute path for pwsh/powershell — this assertion is wrong today and will fail as soon as Windows CI runs it. Fix alongside 0.2/3.2.
- [ ] **3.2** Windows-tagged test suite + un-skip stub-blocked tests
  - New `//go:build windows` file mirroring `TestTerminalService_CreateSession` shape. Darwin mock helpers don't exist on Windows — construct `&TerminalService{ptyBackend: newPtyBackend()}` inline. Assert on `ss.outputCh`, not Wails events (`wailsApp` is nil in tests).
  - Cover: start, write→read round-trip, resize, kill (incl. child processes), bad-working-dir fallback.
  - Un-skip the Phase 0.2 guards that were only skipped due to the stub; keep skips only for genuinely Unix-specific assertions.
  - Acceptance: Windows CI green with real ConPTY tests; macOS/Linux unaffected.
  - Verify: Windows CI + `go test ./...` on macOS.
  - **→ Checkpoint 3**

## Phase 4 — Documentation

- [ ] **4.1** Update stub notes and fix stale cross-reference
  - `AGENTS.md:145`: update "conpty backend is a stub" note to reflect reality; fix the path — points at `.planning/phases/25-polish-integration/CHECKPOINT.md`, actual path is `.planning/milestones/v2.1-phases/25-polish-integration/CHECKPOINT.md`.
  - `CLAUDE.md:250`: update "Windows conpty backend is currently a stub" note.
  - Record deferred items (`buildPtyEnv` Windows correctness, `\n` vs `\r` line submission) as known limitations, not silently dropped.
  - Verify: `rg -i "not yet implemented|conpty.*stub"` returns nothing stale.

## Deferred (not in this plan's scope — documented only)

- `buildPtyEnv` (`pty_env.go:27`): Windows `os.Environ()` hidden `=C:=C:\...` entries collapse into a malformed key under `strings.Cut`; env var names are case-insensitive on Windows but the map keys case-sensitively.
- `execution_service.go:153`: injected command lines end with `\n`; Windows shells generally expect `\r` to submit.
- **Flagged, not decided:** after a clean shell exit, `monitorExit` returns without ever closing `ss.ptmx`. Harmless-ish fd behavior on macOS/Linux; on Windows it holds a live `conhost.exe` alive until the session restarts or closes. Fixing it touches `terminal_service.go`'s intentional-exit branch, which Phase 1 is scoped to leave behavior-unchanged on macOS/Linux — so this needs an explicit call from the user (fast-follow vs. fold into Phase 3) rather than silently doing it.
