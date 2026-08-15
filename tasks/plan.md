# Plan: Real Windows ConPTY Backend for TerminalService

## Context

Cmdex's terminal sessions work on macOS/Linux but are dead on Windows: the terminal tab opens, but typing does nothing and no output appears. The cause is not a subtle bug — `pty_backend_windows.go`'s `conptyBackend.Start` and `Resize` are stubs that unconditionally return `"Windows PTY support not yet implemented"`. This was a deliberate, documented scoping decision in Phase 25 (`.planning/milestones/v2.1-phases/25-polish-integration/CHECKPOINT.md`, decisions D-11..D-14), which shipped a build-tagged `ptyBackend` seam and verified Windows only by cross-compiling. Real ConPTY was left as future work. This plan closes that gap.

**The blocker that shapes this plan:** ConPTY cannot be driven through `os/exec`. Attaching a process to a pseudoconsole requires `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE` in a `STARTUPINFOEX` attribute list, and Go's `syscall.StartProcess` provides no way to pass one ([golang/go#62708](https://github.com/golang/go/issues/62708)). Every ConPTY library therefore returns its own process abstraction, never an `*exec.Cmd`. Since the current interface is `Start(...) (ptyHandle, *exec.Cmd, error)`, **the interface itself must change** — this is not optional and cannot be contained inside `pty_backend_windows.go`.

**Verification reality:** there is no Windows machine available locally. A `windows-latest` CI runner is the only way to observe real behavior, so the plan front-loads building that feedback loop before writing code that depends on it.

**Intended outcome:** Windows terminal sessions behave like macOS/Linux ones — shell starts, keystrokes reach it, output streams back, resize works, close/kill cleans up — proven by a Windows CI job running the real backend.

## Decisions locked with the user

| Decision | Choice |
|---|---|
| Interface change | Introduce `ptyProcess` abstraction; keep `creack/pty` on Unix behind an `execProcess` wrapper |
| Sequencing | Windows CI job first → minimal ConPTY spike → full implementation |
| Extra scope | Include the pre-existing double-`Wait()` race fix; defer `buildPtyEnv` Windows issues and the `\n` vs `\r` gap |

## Target interface

```go
// pty_backend.go
type ptyProcess interface {
    Pid() int
    // Wait returns the REAL exit status. Safe to call repeatedly and
    // concurrently from multiple goroutines — the underlying process wait
    // runs only once (sync.Once-guarded in both execProcess and
    // conptyProcess), and every caller observes the same exit code/error.
    Wait() (exitCode int, err error)
    Exited() bool                      // replaces `cmd.ProcessState == nil` checks
}

type ptyBackend interface {
    Start(shellPath, shellFlag, dir string, rows, cols int) (ptyHandle, ptyProcess, error)
    Resize(handle ptyHandle, cols, rows int) error
    Kill(p ptyProcess) error
}

type ptyHandle interface { io.ReadWriteCloser }   // unchanged
```

Implementations: `execProcess{cmd *exec.Cmd}` (unix), `conptyProcess` (windows), `mockProcess` (darwin test mock).

## The contract the Windows backend must satisfy

Derived by reading `terminal_service.go` — these are the non-obvious constraints that make or break the implementation. Each becomes a verification point.

1. **Arg order differs between methods.** `Start(..., rows, cols)` but `Resize(handle, cols, rows)`. Easy to invert.
2. **`Start` must not fail on a bad `dir`.** `dir` may be `""`, or a deleted/unmounted path. Unix pre-checks with `resolvePtyWorkingDir` (`pty_backend_unix.go:110`) then retries once with the dir cleared (`:88-93`). Preserve this invariant: *a stale working directory must never prevent a session from starting.*
3. **Empty `shellFlag` must be skipped entirely** — `detectShell()` returns `""` for `cmd.exe`, and passing an empty argv element is an error for PowerShell.
4. **`Read` errors are terminal.** `readLoop` (`terminal_service.go:449`) kills the loop permanently on *any* read error — no retry, no classification, no logging. A spurious error silently ends output for that session forever.
5. **`Close()` must promptly unblock a pending `Read`.** `Close` is always called *before* `Kill`, with the child still alive, and is immediately followed by `readerWg.Wait()` (`:314-318`, `:407-412`, `:755-760`). If a blocked `Read` doesn't return, `CloseSession`, `Stop`, restart, and shutdown all deadlock. On Win32, `ClosePseudoConsole` can itself block until the client detaches — this is the single largest runtime risk.
6. **`Write` runs under `ss.mu`** in a `for len(b) > 0 { n, _ := Write(b); b = b[n:] }` loop (`:645`). Returning `(0, nil)` is an infinite loop holding the session mutex.
7. **`Kill` must not reap.** `monitorExit` owns `Wait()`. A `Kill` that also waits recreates the double-`Wait` race on Windows.
8. **`Wait()` must return the real exit code.** `monitorExit` (`:571`) treats a non-zero exit as a crash and **auto-restarts after 100 ms**. A wrong exit code produces an infinite restart loop.

## Phases

Each phase ends at a checkpoint where the build is green and the change is independently verifiable. Phases 0 and 1 are verifiable without any Windows-specific code.

---

### Phase 0 — Build the feedback loop

Goal: a trustworthy `windows-latest` CI signal that is green *against today's stub*, before any behavior changes. Without this, later phases have no verification channel.

**Task 0.1 — Add a Windows Go test job to CI**

`.github/workflows/ci.yml` currently runs `go test ./...` only on `ubuntu-24.04` (the `test` job, gated to `workflow_dispatch`). The `build-check` job does use `windows-latest`, but only for packaging — it never runs tests.

Add a separate `test-windows` job: checkout → `actions/setup-go` (`go-version-file: go.mod`) → create the `frontend/dist/.gitkeep` placeholder that `//go:embed all:frontend/dist` requires → `go test ./...`. Skip all pnpm/Playwright steps; this is a Go-only job.

- *Acceptance:* job appears in CI and executes `go test ./...` on Windows.
- *Verify:* trigger the workflow and read the run log.

**Task 0.2 — Make the existing test suite honest on Windows**

Several untagged tests in `terminal_service_test.go` compile on Windows but encode Unix assumptions, so they will fail there for reasons unrelated to ConPTY:
- `TestTerminalExit` passes `-c`, `sleep 60` / `exit 0` to `ptyStart` — not valid PowerShell/cmd arguments.
- `TestTerminalShutdown` uses `proc.Signal(syscall.Signal(0))` as a liveness probe — meaningless on Windows.
- The real-backend tests (`TestTerminalService_CreateSession` and ~18 others) all call into `ptyBackend.Start`, which is still the stub during this phase.

Guard these appropriately — `runtime.GOOS == "windows"` skips with a reason, following the precedent already set at `terminal_service_test.go:40` and `:54`, and the belt-and-braces pattern in `terminal_service_max_sessions_test.go` (build tag *plus* runtime skip).

- *Acceptance:* Windows CI is green; skipped tests state why; macOS/Linux runs are unchanged (no newly-skipped tests there).
- *Verify:* `go test ./...` locally (macOS) shows the same pass count as before; Windows CI passes with explicit skips.

> **Checkpoint 0:** Windows CI green against the stub. This is the baseline every later phase is measured against.

---

### Phase 1 — Interface refactor (platform-neutral)

Goal: replace `*exec.Cmd` with `ptyProcess` throughout, with **zero behavior change on macOS/Linux**. Fully verifiable on macOS.

**Task 1.1 — Introduce `ptyProcess` and rewire consumers**

- `pty_backend.go`: add the `ptyProcess` interface; change `ptyBackend.Start`/`Kill` signatures.
- `pty_backend_unix.go`: add `execProcess{cmd *exec.Cmd}` implementing `Pid`/`Wait`/`Exited`. `creackPtyBackend.Start` wraps its `*exec.Cmd`; `Kill` takes a `ptyProcess` and unwraps via type assertion (mirroring the existing `Resize` handle-assertion pattern at `:37-43`).
- `pty_backend_mock.go`: add `mockProcess` wrapping the existing `sleep 0.05` command.
- `pty_backend_windows.go`: update the stub's signatures so it still compiles (still returns "not implemented").
- `terminal_service.go`: `sessionState.cmd *exec.Cmd` → `proc ptyProcess`; `monitorExit(ss, proc, ...)` uses `proc.Wait()` directly, which **removes the `errors.As(&exec.ExitError{})` unwrapping** (the exit code now comes from the interface); the three `oldCmd.ProcessState == nil` guards (`:315`, `:409`, `:757`) become `!oldProc.Exited()`.

- *Acceptance:* full test suite passes on macOS with no test modifications beyond mechanical type changes; `GOOS=windows go build ./...` exits 0; `make check` and `make lint` clean.
- *Verify:* `go test ./...`, `GOOS=windows go build ./...`, `make check`, `make lint`. Manually run the app (`wails3 dev`) and confirm terminal sessions still open, echo input, resize, and close cleanly on macOS.

**Task 1.2 — Fix the double-`Wait()` race**

Pre-existing defect this refactor lands directly on top of: `monitorExit` calls `cmd.Wait()` (`terminal_service.go:572`) while `killProcessGroup` calls `cmd.Wait()` again on the same `cmd` in a goroutine (`pty_backend_unix.go:143`). `os/exec` forbids concurrent/repeated `Wait`; the racing calls can corrupt `ProcessState` and misreport exit status.

Fix inside `execProcess`: a single `sync.Once`-guarded `Wait` whose result is broadcast to all callers via a closed channel + stored result. `killProcessGroup`'s escalation then awaits *that* shared result rather than calling `cmd.Wait()` itself, **preserving the existing SIGHUP → 2 s → SIGKILL semantics** (`ptyKillTimeout`).

- *Acceptance:* `go test -race ./...` clean on macOS; kill/close/restart behavior unchanged; SIGKILL escalation still fires when a process ignores SIGHUP.
- *Verify:* `go test -race ./...`; add a focused test asserting `Wait()` is safe to call from two goroutines and both observe the same exit code.

> **Checkpoint 1:** macOS/Linux fully green including `-race`; Windows CI still green (stub unchanged behaviorally). No Windows-specific code written yet.

---

### Phase 2 — ConPTY spike

Goal: validate the library choice against real Windows before investing in the full implementation. This phase exists specifically because the riskiest properties (contract items 5 and 8) cannot be reasoned about reliably from documentation.

**Task 2.1 — Minimal ConPTY vertical slice**

Pick the library (see *Library choice* below), add it to `go.mod`, and implement the smallest thing that proves the contract: `Start` a shell, `Write` a command, `Read` its output, `Resize`, `Close`, and observe a correct exit code from `Wait`.

Write it as a `//go:build windows` test file so it runs on the Phase 0 CI job. It must answer, on real hardware:

1. **Does `Close()` unblock a pending `Read()`?** (contract 5 — the deadlock risk). Test with a live child and a blocked reader; assert the read returns within a timeout rather than hanging.
2. **Does `Wait()` return the true exit code** after the process exits normally, and separately after `taskkill /F /T`? (contract 8 — infinite-restart risk).
3. **Does `Read` ever return `(0, nil)` in a tight loop?** (contract 4/6 — busy-spin risk).

- *Acceptance:* all three questions answered by a green (or informatively red) Windows CI run.
- *Verify:* Windows CI run log. If any answer is bad, revisit the library choice *here* — before Phase 3 — rather than after.

> **Checkpoint 2:** Library validated on real Windows, or swapped based on evidence. Explicitly re-confirm direction before Phase 3 if the spike surprises us.

---

### Phase 3 — Full Windows implementation

**Task 3.1 — Implement `conptyBackend` and `conptyProcess`**

Replace the stubs in `pty_backend_windows.go`:
- `Start`: build argv (`[shellFlag if non-empty] + extraArgs`), resolve the working directory with a Windows equivalent of `resolvePtyWorkingDir` plus the retry-with-empty-dir fallback (contract 2), pass env from `buildPtyEnv(os.Environ())` to match Unix, create the pseudoconsole sized `cols × rows` (mind the arg order, contract 1).
- `Resize`: real `ResizePseudoConsole`.
- `Kill`: keep the existing `taskkill /F /T /PID` helper; must not reap (contract 7).
- `conptyProcess`: `Pid`, `Exited`, and a `sync.Once`-guarded `Wait` returning the real exit code (contract 8).
- Preserve the package-level `ptyStart`/`ptyResize`/`killProcessGroup` functions — `pty_backend_windows.go:41` documents that untagged test code depends on them existing on Windows.

If the chosen library takes a command-line *string* rather than argv, quoting must follow `CommandLineToArgvW` rules — shell paths like `C:\Program Files\PowerShell\7\pwsh.exe` contain spaces. Use a vetted quoting helper, never naive concatenation.

- *Acceptance:* Windows CI runs the real backend; `GOOS=windows go build ./...` and `make lint` clean.
- *Verify:* Windows CI.

**Task 3.2 — Windows-tagged test suite**

New `//go:build windows` test file mirroring the shape of `TestTerminalService_CreateSession` (`terminal_service_test.go:282`). Note the darwin mock helpers (`newTestTerminalServiceWithMock`, `mockPtyBackend`) **do not exist on Windows**, so construct the service inline: `&TerminalService{ptyBackend: newPtyBackend()}` with an initialized `sessions` map. Assert on the internal `ss.outputCh` channel rather than Wails events — `wailsApp` is nil in tests and all emission sites are nil-guarded.

Cover: session start, write→read round-trip, resize, kill/termination including children, and the bad-working-directory fallback.

Then un-skip the Phase 0.2 guards that were only skipped because the backend was a stub, keeping skips only for genuinely Unix-specific assertions.

- *Acceptance:* Windows CI green with real ConPTY tests running; macOS/Linux unaffected.
- *Verify:* Windows CI + local `go test ./...` on macOS.

> **Checkpoint 3:** Windows terminals proven working in CI.

---

### Phase 4 — Documentation

**Task 4.1 — Update the stub notes**

`AGENTS.md:145` and `CLAUDE.md:250` both state the conpty backend is a stub. Update to reflect reality. Record the deferred items (`buildPtyEnv` Windows correctness, `\n` vs `\r`) as known limitations rather than dropping them silently. Follow the repo's `.planning/` checkpoint convention if appropriate — confirm at the time.

Also fix the stale cross-reference while here: `AGENTS.md:145` points at `.planning/phases/25-polish-integration/CHECKPOINT.md`, but the file actually lives at `.planning/milestones/v2.1-phases/25-polish-integration/CHECKPOINT.md`.

- *Verify:* `rg -i "not yet implemented|conpty.*stub"` returns nothing stale.

---

## Library choice — DECIDED: `github.com/charmbracelet/x/conpty` v0.2.0

The design review (read both libraries' full source, not just docs) found this isn't a close call: **`UserExistsError/conpty`'s `Close()` destroys the process handle its own `Wait()` depends on** (`conpty.go:238` calls `closeHandles(cpty.pi.Process, ...)`; `Wait` at `:252` calls `WaitForSingleObject` on that same handle). In cmdex, `monitorExit` is parked in `Wait()` exactly while `Stop`/`CloseSession` calls `Close()` — a blocked wait on a handle another thread just closed is undefined behavior on Win32, and on return `GetExitCodeProcess` fails, yielding `(259 STILL_ACTIVE, err)` — precisely the "non-zero + error" shape contract #9 says triggers the 100 ms auto-restart loop. Working around it means never using the library's `Wait` at all, at which point its only remaining advantage (a ready-made `Wait`) is gone and its command-line-string API (quoting hazard, see below) remains a liability.

`charmbracelet/x/conpty` doesn't have this defect: `Close()` never touches the process handle (`Spawn` returns `pi.Process` to the caller, who owns its lifecycle independently). It's also argv-based (`Spawn(name string, args []string, attr *syscall.ProcAttr)`) — no command-line quoting hazard — and already shares `golang.org/x/sys` v0.47.0 with the existing indirect dependency tree, so it adds no new module tree.

**Known landmine to fix while integrating (found by the review):** `charmbracelet/x/conpty`'s internal `lookExtensions` resolves a *bare* program name relative to the working directory. `detectShell()` returns the bare string `"cmd"` when neither `pwsh` nor `powershell` is found on PATH — with this library, any non-empty `dir` would make that resolution fail, silently trigger the dir-fallback retry, and **every `cmd.exe` session would lose its configured working directory**. Fix: resolve `shellPath` via `exec.LookPath` to an absolute path *before* calling `Spawn`, in `conptyStart`.

**Command-line quoting is why the "reject" verdict on `UserExistsError` is firm, not close:** it calls `CreateProcess(nil /* lpApplicationName */, cmdLine, ...)`, so Windows resolves *which binary runs* by parsing the command-line string itself — not via `CommandLineToArgvW` — creating the classic "unquoted path" ambiguity class the moment a shell path contains a space or quote. `charmbracelet` passes `lpApplicationName` explicitly (like `os/exec` does), removing this class entirely.

**Rejected:** `aymanbagabas/go-pty` (unified Unix+Windows API) — would replace `creack/pty` on macOS/Linux too, putting regression risk on the platform in daily use. Not revisited — the primary candidate cleared review.

## Refined design from the review (fold into Phase 2/3 tasks)

- **Read must not trust the library's blocking `Read` to unblock on `Close()`.** Neither library's pipe breaks on child exit (both keep their own write-end handle open), and closing a handle with a pending synchronous `ReadFile` is undefined behavior. Defensive design (see `tasks/todo.md` 3.1 for the concrete sketch): a dedicated pump goroutine drives the blocking `ReadFile` into a channel; `conptyHandle.Read` selects on that channel *or* a `done` channel closed by `Close()`. This makes `Close()` unblock `Read` in native Go time regardless of what the syscall does, at the cost of a possibly-leaked pump goroutine bounded by `MaxSessions` in the worst case.
- **`Kill` must hold a lock across the `taskkill` call to close a PID-reuse race**: once our process handle is closed, Windows can recycle the PID before `taskkill /F /T /PID <pid>` runs, potentially killing an unrelated process. Keep the process handle open (don't close it until `Wait()`'s `finish()` runs) and serialize `Kill`/`terminate`/`Wait` through the same mutex.
- **Resolve `taskkill` from `%SystemRoot%\System32` explicitly**, not via `exec.Command("taskkill", ...)` (`PATH`-hijackable, and a lint/security-scan flag). Set `CREATE_NO_WINDOW` so a console window doesn't flash on every session close.
- **Move `resolvePtyWorkingDir` out of `pty_backend_unix.go` into `pty_backend.go`** — it's pure `os.Stat`/`os.UserHomeDir`, already platform-neutral, and Windows needs the identical retry-with-empty-dir logic (contract 2).
- **`conpty.New`/`Resize` take `(width, height)` == `(cols, rows)`** — an extra place the rows/cols transpose (contract 1) can bite; the recommended `Start` sketch in `tasks/todo.md` handles this explicitly.

## Additional defect found (not introduced by this refactor, pre-existing)

- **`terminal_service_test.go:41`** already asserts `detectShell()` returns one of the bare strings `{"pwsh","powershell","cmd"}`, but the function actually returns `exec.LookPath`'s **absolute path** for `pwsh`/`powershell`. This assertion is wrong today and will fail the moment Windows CI actually runs it (Phase 0.2/3.2) — fix alongside the other Windows test guards, it's not new scope, just newly-observable once CI exists.
- **After a clean shell exit, `monitorExit` never closes `ss.ptmx`** (`terminal_service.go` — returns at the intentional-exit branch without closing the handle). Harmless-ish fd behavior on Unix; on Windows it holds a live `conhost.exe` until the session restarts or closes. Documented here rather than silently folded into Phase 1 (which is scoped as "zero behavior change on macOS/Linux") — flag to the user as a candidate for Phase 3 or a fast-follow, not decided unilaterally.

## Deferred (documented, not fixed here)

- **`buildPtyEnv` Windows correctness** (`pty_env.go:27`): Windows `os.Environ()` includes hidden `=C:=C:\...` drive entries, which `strings.Cut(kv, "=")` collapses into a single malformed `""`-keyed entry. Windows env names are also case-insensitive while the function keys a Go map case-sensitively. Neither blocks basic I/O.
- **`\n` vs `\r` line submission**: `execution_service.go:153` terminates injected command lines with `"\n"`. Windows shells generally expect `\r` to submit a line. Affects "run command in terminal", not interactive typing.

## Files touched

| File | Phase |
|---|---|
| `.github/workflows/ci.yml` | 0 |
| `terminal_service_test.go` | 0, 3 |
| `pty_backend.go` | 1 |
| `pty_backend_unix.go` | 1 |
| `pty_backend_mock.go` | 1 |
| `terminal_service.go` | 1 |
| `pty_backend_windows.go` | 1 (signatures), 3 (implementation) |
| `go.mod` / `go.sum` | 2 |
| new `pty_backend_windows_test.go` | 2, 3 |
| `AGENTS.md`, `CLAUDE.md` | 4 |

## Verification summary

- **Every phase:** `make check` (`go build ./...` + `pnpm tsc --noEmit`) and `make lint` clean.
- **Phases 0–1:** `go test ./...` and `go test -race ./...` green on macOS; `GOOS=windows go build ./...` exits 0; app manually exercised via `wails3 dev` to confirm macOS terminals still work end-to-end.
- **Phases 2–3:** `windows-latest` CI running `go test ./...` against the real backend — the only real Windows signal available.
- **Regression bar:** the ~20 existing real-backend tests and the darwin mock-backed stress/max-sessions tests must keep passing unchanged on macOS throughout.

## Task list

All tasks below are complete; see `tasks/todo.md` for the full record including Phase 5 (`/ship`-review follow-up fixes) and the commits/CI runs that verified each one.

- [x] 0.1 Add `test-windows` CI job running `go test ./...`
- [x] 0.2 Guard Unix-only assumptions in untagged tests → **Checkpoint 0**
- [x] 1.1 Introduce `ptyProcess`; rewire backends, mock, and `terminal_service.go`
- [x] 1.2 Fix the double-`Wait()` race in `execProcess` → **Checkpoint 1**
- [x] 2.1 ConPTY spike answering the three runtime risks → **Checkpoint 2**
- [x] 3.1 Implement `conptyBackend` + `conptyProcess`
- [x] 3.2 Windows-tagged test suite; un-skip stub-blocked tests → **Checkpoint 3**
- [x] 4.1 Update `AGENTS.md` / `CLAUDE.md`; record deferred items
