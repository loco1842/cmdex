package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
)

// ptyBackend is the seam between TerminalService and the OS PTY layer.
// It exists so the OS-specific implementation (creack/pty on darwin, conpty
// on windows) can be swapped at test time via the darwin mock.
//
// Start/Kill return/accept a ptyProcess rather than *exec.Cmd because ConPTY
// cannot be driven through os/exec — there is no way to pass the
// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE attribute to syscall.StartProcess
// (see golang/go#62708) — so the windows implementation returns its own
// process handle wrapper instead of an *exec.Cmd.
type ptyBackend interface {
	Start(shellPath, shellFlag, dir string, rows, cols int, opts shellLaunchOpts) (ptyHandle, ptyProcess, error)
	Resize(handle ptyHandle, cols, rows int) error
	Kill(proc ptyProcess) error
}

// shellLaunchOpts carries shell-integration launch tweaks (see
// shell_integration.go) down to the OS-specific ptyBackend implementations.
// The zero value means "no integration" — Start behaves exactly as it did
// before shell integration existed.
type shellLaunchOpts struct {
	// ExtraArgs is appended after shellFlag (if shellFlag is non-empty).
	ExtraArgs []string
	// ExtraEnv is merged into the child's environment by buildPtyEnv,
	// alongside (and by the same by-key-overwrite rule as) TERM/COLORTERM/etc.
	ExtraEnv []string
}

// ptyHandle is the I/O surface a started PTY exposes to TerminalService.
// *os.File satisfies this interface (it implements io.ReadWriteCloser), so
// the production code path is unchanged for darwin; the darwin-side mock
// also satisfies it via mockPtyHandle.
type ptyHandle interface {
	io.ReadWriteCloser
}

// ptyProcess abstracts the spawned shell process so TerminalService never
// needs an *exec.Cmd directly. Wait must return the real exit status and be
// safe to call more than once (concurrently or repeatedly) — callers must
// never observe os/exec's "Wait was already called" error. Exited must not
// block and is used in place of the old `cmd.ProcessState == nil` check.
type ptyProcess interface {
	Pid() int
	Wait() (exitCode int, err error)
	Exited() bool
}

// execProcess adapts an *exec.Cmd to ptyProcess. It is platform-neutral
// (os/exec.Cmd works the same on every GOOS) so it's shared by the unix
// ptyBackend, the darwin test mock, and tests that call the package-level
// ptyStart helper directly without going through ptyBackend.
//
// Wait is sync.Once-guarded: callers can invoke it concurrently or more than
// once and all observe the same single physical cmd.Wait() call and result.
// This matters because os/exec forbids calling Wait concurrently or
// repeatedly on the same *exec.Cmd (a second call returns "exec: Wait was
// already called" and can race the first call's writes to
// cmd.ProcessState) — both monitorExit and killProcessGroup's SIGKILL
// escalation need to wait for exit, and previously did so via two separate
// cmd.Wait() calls on the same *exec.Cmd.
type execProcess struct {
	cmd *exec.Cmd

	waitOnce sync.Once
	waitDone chan struct{}
	exitCode int
	waitErr  error
}

func newExecProcess(cmd *exec.Cmd) *execProcess {
	return &execProcess{cmd: cmd, waitDone: make(chan struct{})}
}

func (p *execProcess) Pid() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *execProcess) Wait() (int, error) {
	p.waitOnce.Do(func() {
		defer close(p.waitDone)
		err := p.cmd.Wait()
		p.waitErr = err
		if err == nil {
			return
		}
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			p.exitCode = exitErr.ExitCode()
		} else {
			p.exitCode = -1
		}
	})
	<-p.waitDone
	return p.exitCode, p.waitErr
}

// Exited reports whether Wait has completed. It intentionally does not read
// p.cmd.ProcessState directly: that field is written by cmd.Wait() with no
// synchronization of its own, so reading it from any goroutine other than
// the one that called Wait is a data race (cmd.Wait() is the only writer,
// and it only ever runs inside p.waitOnce.Do below) — waitDone is the sole
// race-free signal for "Wait has completed", and since ProcessState is only
// ever populated as a side effect of a completed Wait, this is equivalent.
func (p *execProcess) Exited() bool {
	select {
	case <-p.waitDone:
		return true
	default:
		return false
	}
}

// resolvePtyWorkingDir returns dir if it exists and is a directory, falling
// back to the OS user home directory, then to "" (inherit the app's cwd) if
// neither is usable. This is a best-effort pre-check only — Start still
// retries without a working directory if the chosen one fails at spawn time
// (deleted between check and use, or missing execute permission) — but it
// keeps the common case (a stale/deleted configured working directory) from
// needing that fallback path. e.g. getWorkingDir() can return a
// saved-but-now-missing custom path, or "" if os.UserHomeDir() itself
// failed. Platform-neutral (os.Stat/os.UserHomeDir behave the same on every
// GOOS), so both the unix and windows backends share this.
func resolvePtyWorkingDir(dir string) string {
	if dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if info, err := os.Stat(home); err == nil && info.IsDir() {
			return home
		}
	}
	return ""
}
