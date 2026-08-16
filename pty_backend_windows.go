//go:build windows

package main

// Windows PTY backend, backed by github.com/charmbracelet/x/conpty (see
// "Library choice" in tasks/plan.md for why this library was picked over
// github.com/UserExistsError/conpty, and the Phase 2 spike in
// pty_backend_windows_conpty_spike_test.go for the real-hardware validation
// that preceded this implementation).
//
// ConPTY cannot be driven through os/exec — there is no way to pass the
// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE attribute to syscall.StartProcess (see
// golang/go#62708) — so conptyProcess wraps the raw process handle conpty's
// Spawn returns instead of an *exec.Cmd, same as execProcess wraps *exec.Cmd
// on unix.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/charmbracelet/x/conpty"
	"golang.org/x/sys/windows"
)

// conptyCloseTimeout bounds how long conptyHandle.Close waits for
// ClosePseudoConsole, which is known to be able to block until the attached
// client detaches. Close is on the critical path of Stop/CloseSession/
// ServiceShutdown, so a bounded wait (log-and-leak) beats hanging the caller.
const conptyCloseTimeout = 2 * time.Second

// conptyKillTimeout bounds how long conptyProcess.kill's CALLER waits for
// taskkill. See kill() below for why this does not shorten how long p.mu is
// actually held.
const conptyKillTimeout = 2 * time.Second

// newPtyBackend returns the conpty-backed ptyBackend for windows.
func newPtyBackend() ptyBackend {
	return conptyBackend{}
}

// conptyBackend is the real, conpty-backed ptyBackend implementation.
type conptyBackend struct{}

// Start spawns a shell attached to a new ConPTY.
func (conptyBackend) Start(
	shellPath, shellFlag, dir string,
	rows, cols int,
	opts shellLaunchOpts,
) (ptyHandle, ptyProcess, error) {
	return conptyStart(shellPath, shellFlag, dir, rows, cols, opts.ExtraEnv, opts.ExtraArgs...)
}

// conptyStart is the real implementation behind conptyBackend.Start,
// separated out (mirroring pty_backend_unix.go's ptyStart) so it can take
// extraArgs for tests. extraEnv is merged into the child's environment via
// buildPtyEnv — pass nil when there's nothing to add.
func conptyStart(
	shellPath, shellFlag, dir string,
	rows, cols int,
	extraEnv []string,
	extraArgs ...string,
) (ptyHandle, ptyProcess, error) {
	if rows < 1 || rows > 65535 || cols < 1 || cols > 65535 {
		return nil, nil, fmt.Errorf("conptyStart: invalid dimensions rows=%d cols=%d (must be 1..65535)", rows, cols)
	}

	// Resolve to an absolute path before Spawn. charmbracelet/x/conpty's
	// internal lookExtensions resolves a BARE program name relative to the
	// working directory whenever dir != "" — detectShell()'s "cmd" fallback
	// (returned when neither pwsh nor powershell is found) is exactly such a
	// bare name, so with any configured working directory, resolution would
	// fail, silently trigger the dir-fallback retry below, and every cmd.exe
	// session would lose its configured cwd. An absolute/volume-qualified
	// path skips that code path entirely.
	//
	// If exec.LookPath itself fails to resolve "cmd" (PATH broken/altered),
	// fall back to the hardcoded %SystemRoot%\System32 location rather than
	// falling through to the bare name — same reasoning as killProcessGroup's
	// taskkill resolution: a bare name is PATH-hijackable, and here it would
	// additionally be resolved by conpty relative to the session's (possibly
	// untrusted) working directory.
	resolvedShellPath := shellPath
	if resolved, err := exec.LookPath(shellPath); err == nil {
		resolvedShellPath = resolved
	} else if shellPath == "cmd" {
		resolvedShellPath = sysRootPath("cmd.exe")
	}

	argv := []string{resolvedShellPath}
	if shellFlag != "" {
		argv = append(argv, shellFlag)
	}
	argv = append(argv, extraArgs...)

	// conpty.New/Resize take (width, height) == (cols, rows) — the reverse
	// of this function's (rows, cols) parameter order.
	cp, err := conpty.New(cols, rows, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("conpty.New: %w", err)
	}

	env := buildPtyEnv(os.Environ(), extraEnv)
	spawn := func(wd string) (int, uintptr, error) {
		return cp.Spawn(resolvedShellPath, argv, &syscall.ProcAttr{Dir: wd, Env: env})
	}

	// Starting a session must not fail just because its configured working
	// directory turned out to be unusable — mirrors ptyStart's retry on
	// unix (pty_backend_unix.go), sharing resolvePtyWorkingDir's pre-check.
	wd := resolvePtyWorkingDir(dir)
	pid, rawHandle, err := spawn(wd)
	if err != nil && wd != "" {
		pid, rawHandle, err = spawn("")
	}
	if err != nil {
		_ = cp.Close()
		return nil, nil, fmt.Errorf("conpty.Spawn: %w", err)
	}

	proc := newConptyProcess(pid, windows.Handle(rawHandle))
	handle := newConptyHandle(cp, proc)
	return handle, proc, nil
}

// Resize updates the pseudo-console size.
func (conptyBackend) Resize(handle ptyHandle, cols, rows int) error {
	h, ok := handle.(*conptyHandle)
	if !ok {
		return fmt.Errorf("conptyBackend.Resize: unexpected handle type %T", handle)
	}
	if h.closed.Load() {
		return os.ErrClosed
	}
	h.rsz.Lock()
	defer h.rsz.Unlock()
	if h.closed.Load() {
		return os.ErrClosed
	}
	return h.cp.Resize(cols, rows)
}

// Kill terminates proc's process tree via taskkill. Does not reap —
// monitorExit's Wait() alone owns reaping (contract 7/8).
func (conptyBackend) Kill(proc ptyProcess) error {
	cp, ok := proc.(*conptyProcess)
	if !ok || cp == nil {
		return nil
	}
	return cp.kill()
}

// conptyProcess adapts conpty.Spawn's returned (pid, handle) to ptyProcess.
//
// Wait is sync.Once-guarded: WaitForSingleObject blocks WITHOUT holding mu,
// so it can never deadlock against kill() or terminate(). The process
// handle is only ever closed by closeHandleLocked, called from finish() and
// from kill()'s background goroutine — whichever of the two runs last is
// the one that actually closes it (see killing/exited below) — so no
// caller can ever be blocked on a handle another goroutine is closing —
// this is the exact defect that ruled out github.com/UserExistsError/conpty
// (see tasks/plan.md "Library choice"): its Close() closes the same handle
// its own Wait() waits on.
type conptyProcess struct {
	pid int

	mu      sync.Mutex
	h       windows.Handle // valid until closeHandleLocked actually closes it
	killing int            // count of in-flight kill() calls still referencing h
	exited  bool           // true once finish() has recorded the exit code

	waitOnce sync.Once
	waitDone chan struct{}
	code     int
	waitErr  error
}

func newConptyProcess(pid int, handle windows.Handle) *conptyProcess {
	return &conptyProcess{pid: pid, h: handle, waitDone: make(chan struct{})}
}

func (p *conptyProcess) Pid() int { return p.pid }

// Wait blocks until the process exits and reports its exit code. Safe to
// call concurrently or repeatedly — only the first call actually waits,
// and every caller (including repeated calls) observes the identical
// cached (exitCode, err) result.
//
// On a WaitForSingleObject/GetExitCodeProcess syscall failure, the real
// error is returned (rather than swallowed to nil) so this method doesn't
// lie about a failure having occurred, matching execProcess's equivalent
// handling on unix (pty_backend.go). exitCode is still reported as 0 in
// this case, deliberately NOT repurposed to signal the failure (e.g. to
// -1): monitorExit (terminal_service.go) discards Wait's error return
// entirely and decides "crash vs. intentional exit" from exitCode alone,
// so if this exceedingly rare syscall failure ever happened on every
// session across repeated auto-restarts, treating it as a crash would risk
// an infinite restart loop, whereas exitCode == 0 costs at most one missed
// auto-restart.
func (p *conptyProcess) Wait() (int, error) {
	p.waitOnce.Do(func() {
		defer close(p.waitDone)

		p.mu.Lock()
		h := p.h
		p.mu.Unlock()
		if h == 0 {
			p.finish(0, nil)
			return
		}

		if _, err := windows.WaitForSingleObject(h, windows.INFINITE); err != nil {
			fmt.Printf("conptyProcess.Wait: WaitForSingleObject(pid=%d): %v\n", p.pid, err)
			p.finish(0, fmt.Errorf("conptyProcess.Wait: WaitForSingleObject(pid=%d): %w", p.pid, err))
			return
		}

		var code uint32
		if err := windows.GetExitCodeProcess(h, &code); err != nil {
			fmt.Printf("conptyProcess.Wait: GetExitCodeProcess(pid=%d): %v\n", p.pid, err)
			p.finish(0, fmt.Errorf("conptyProcess.Wait: GetExitCodeProcess(pid=%d): %w", p.pid, err))
			return
		}
		p.finish(int(code), nil)
	})
	<-p.waitDone
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.code, p.waitErr
}

// finish records the exit code/error and, unless a kill() call is still in
// flight, closes the process handle. It only ever runs after
// WaitForSingleObject has returned. Never blocks — see closeHandleLocked.
func (p *conptyProcess) finish(code int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.code = code
	p.waitErr = err
	p.exited = true
	p.closeHandleLocked()
}

// closeHandleLocked closes p.h once both finish() has recorded the exit
// code and no kill() call is still referencing the pid/handle. Call with
// p.mu held. This is the only place the handle is ever closed, and it's
// called from both finish() and kill()'s background goroutine — whichever
// runs last performs the actual close — so a handle is never closed while
// an in-flight taskkill /PID lookup could still resolve it to a (possibly
// since-recycled) PID, without either side needing to block waiting for
// the other.
func (p *conptyProcess) closeHandleLocked() {
	if p.exited && p.killing == 0 && p.h != 0 {
		_ = windows.CloseHandle(p.h)
		p.h = 0
	}
}

// Exited reports whether Wait has completed — see execProcess.Exited in
// pty_backend.go for why this is backed by waitDone alone rather than any
// direct field read.
func (p *conptyProcess) Exited() bool {
	select {
	case <-p.waitDone:
		return true
	default:
		return false
	}
}

// terminate is a fast, tree-less TerminateProcess used only by
// conptyHandle.Close, so ClosePseudoConsole's teardown has nothing left to
// wait on. Distinct from kill() (used by ptyBackend.Kill), which kills the
// whole process tree via taskkill — Close only needs the immediate child
// gone quickly, and every caller of Close() already calls Kill() right
// after anyway (see terminal_service.go's CloseSession/Stop/
// startSessionLocked), so a broader tree-kill here would be redundant.
func (p *conptyProcess) terminate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.h != 0 {
		_ = windows.TerminateProcess(p.h, 1)
	}
}

// kill terminates the process tree via taskkill. Idempotent and safe on an
// already-exited process. Never holds p.mu for the duration of the
// taskkill subprocess call — only brief lock/unlock pairs at the start and
// end — so a wedged taskkill.exe (unresponsive process, AV interference —
// both real on Windows) can never block terminate(), Wait(), or a
// subsequent kill()/finish() call the way holding the mutex across the
// call would. Instead, the in-flight call is tracked via p.killing, and
// closeHandleLocked (called both here and from finish()) only actually
// closes p.h once every in-flight kill has finished — preventing the
// PID-reuse race (Windows can recycle a PID once its handle is closed,
// and a still-in-flight taskkill /PID lookup could then hit an unrelated
// process) without requiring either side to block waiting for the other.
//
// kill itself must not block its CALLER indefinitely either: it sits on
// the critical path of CloseSession/Stop, and ServiceShutdown closes
// sessions sequentially, so one wedged taskkill.exe would otherwise hang
// the whole app's shutdown. conptyKillTimeout bounds only how long the
// caller waits for the *result* — the background goroutine keeps running
// (and p.killing stays elevated, deferring the handle close) until the
// real killProcessGroup call actually returns, however long that takes.
func (p *conptyProcess) kill() error {
	p.mu.Lock()
	if p.h == 0 {
		p.mu.Unlock()
		return nil
	}
	p.killing++
	p.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		err := killProcessGroup(p.pid)
		p.mu.Lock()
		p.killing--
		p.closeHandleLocked()
		p.mu.Unlock()
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(conptyKillTimeout):
		return fmt.Errorf(
			"conptyProcess.kill: taskkill(pid=%d) did not return within %s (still running in background)",
			p.pid,
			conptyKillTimeout,
		)
	}
}

// conptyHandle adapts a *conpty.ConPty to ptyHandle.
//
// Read does not trust the underlying ReadFile to unblock on Close(): ConPTY
// keeps its own copy of the output pipe's write end open for its lifetime,
// so the pipe does not break when the child exits, and closing a handle
// with a pending synchronous ReadFile is undefined behavior on Win32 in
// general. Instead, a dedicated pump goroutine owns the blocking Read call,
// and the public Read method selects on a channel fed by the pump or a
// done channel closed by Close — this makes Close() unblock Read in native
// Go time regardless of what the underlying syscall does.
type conptyHandle struct {
	cp   *conpty.ConPty
	proc *conptyProcess

	out  chan []byte
	done chan struct{}

	rmu     sync.Mutex
	rem     []byte
	readErr error

	wmu sync.Mutex
	rsz sync.Mutex

	closeOnce sync.Once
	closed    atomic.Bool
}

func newConptyHandle(cp *conpty.ConPty, proc *conptyProcess) *conptyHandle {
	h := &conptyHandle{
		cp:   cp,
		proc: proc,
		out:  make(chan []byte, 1),
		done: make(chan struct{}),
	}
	go h.pump()
	return h
}

// pump runs the blocking conpty Read in its own goroutine so Close() can
// unblock callers of Read via the done channel without waiting on the
// syscall itself. Never forwards a (0, nil) read (contract 4: readLoop
// would busy-spin on it) — a zero-byte, no-error read is treated as EOF,
// which cannot happen for a byte-mode pipe but is handled defensively.
func (h *conptyHandle) pump() {
	defer close(h.out)
	buf := make([]byte, readBufferSize)
	for {
		n, err := h.cp.Read(buf)
		if n > 0 {
			b := make([]byte, n)
			copy(b, buf[:n])
			select {
			case h.out <- b:
			case <-h.done:
				return
			}
		}
		if err != nil {
			h.rmu.Lock()
			switch {
			case errors.Is(err, windows.ERROR_BROKEN_PIPE),
				errors.Is(err, windows.ERROR_OPERATION_ABORTED),
				errors.Is(err, windows.ERROR_INVALID_HANDLE):
				h.readErr = io.EOF
			default:
				h.readErr = err
			}
			h.rmu.Unlock()
			return
		}
		if n == 0 {
			h.rmu.Lock()
			h.readErr = io.EOF
			h.rmu.Unlock()
			return
		}
	}
}

func (h *conptyHandle) Read(p []byte) (int, error) {
	h.rmu.Lock()
	if len(h.rem) > 0 {
		n := copy(p, h.rem)
		h.rem = h.rem[n:]
		h.rmu.Unlock()
		return n, nil
	}
	h.rmu.Unlock()

	select {
	case b, ok := <-h.out:
		if !ok {
			h.rmu.Lock()
			err := h.readErr
			h.rmu.Unlock()
			if err == nil {
				err = io.EOF
			}
			return 0, err
		}
		n := copy(p, b)
		h.rmu.Lock()
		h.rem = b[n:]
		h.rmu.Unlock()
		return n, nil
	case <-h.done:
		return 0, os.ErrClosed
	}
}

func (h *conptyHandle) Write(p []byte) (int, error) {
	if h.closed.Load() {
		return 0, os.ErrClosed
	}
	h.wmu.Lock()
	defer h.wmu.Unlock()
	if h.closed.Load() {
		return 0, os.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}
	n, err := h.cp.Write(p)
	if err != nil {
		return n, err
	}
	if n == 0 {
		// Contract 5/6: Write is called under ss.mu in a loop that retries
		// on partial writes; a (0, nil) return there is an infinite loop
		// holding the session mutex.
		return 0, fmt.Errorf("conptyHandle.Write: wrote 0 bytes with no error")
	}
	return n, nil
}

// Close is idempotent and safe to call while the child is alive. It closes
// done first (unblocking any pending Read immediately, satisfying contract
// 5 regardless of ClosePseudoConsole's own behavior), pre-terminates the
// child so ClosePseudoConsole has nothing left to wait for, then bounds the
// wait on ClosePseudoConsole itself — logging and leaking rather than
// hanging the caller, since Close sits on the critical path of
// Stop/CloseSession/ServiceShutdown.
func (h *conptyHandle) Close() error {
	var err error
	h.closeOnce.Do(func() {
		h.closed.Store(true)
		close(h.done)

		if h.proc != nil {
			h.proc.terminate()
		}

		errCh := make(chan error, 1)
		go func() { errCh <- h.cp.Close() }()
		select {
		case err = <-errCh:
		case <-time.After(conptyCloseTimeout):
			err = fmt.Errorf(
				"conptyHandle.Close: ClosePseudoConsole did not return within %s (leaked)",
				conptyCloseTimeout,
			)
			fmt.Println(err.Error())
		}
	})
	return err
}

// ptyStart is preserved as a package-level function with its legacy
// *os.File/*exec.Cmd signature purely so TestPtyStart_RejectsInvalidDimensions
// (terminal_service_test.go, untagged — runs on every platform) keeps
// compiling and keeps testing real dimension validation on Windows. It
// cannot return real conpty objects through this legacy signature (conpty's
// handle/process types don't fit *os.File/*exec.Cmd) — the real
// implementation is conptyStart above, reached via conptyBackend.Start
// through the ptyBackend interface.
func ptyStart(
	shellPath, shellFlag, dir string,
	rows, cols int,
	extraEnv []string,
	extraArgs ...string,
) (*os.File, *exec.Cmd, error) {
	if rows < 1 || rows > 65535 || cols < 1 || cols > 65535 {
		return nil, nil, fmt.Errorf("ptyStart: invalid dimensions rows=%d cols=%d (must be 1..65535)", rows, cols)
	}
	return nil, nil, fmt.Errorf("ptyStart: use conptyBackend.Start via the ptyBackend interface instead")
}

// killProcessGroup terminates pid's process tree via taskkill, resolved
// explicitly from %SystemRoot%\System32 rather than via PATH (exec.Command
// with a bare name is PATH-hijackable), with CREATE_NO_WINDOW so no console
// window flashes on every session close.
func killProcessGroup(pid int) error {
	if pid == 0 {
		return nil
	}
	cmd := exec.Command(sysRootPath("taskkill.exe"), "/F", "/T", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd.Run()
}

// sysRootPath resolves name explicitly under %SystemRoot%\System32 rather
// than via PATH (exec.Command/exec.LookPath with a bare name is
// PATH-hijackable, and — for a bare "cmd" — charmbracelet/x/conpty resolves
// an unresolved bare program name relative to the working directory, which
// could be an untrusted session cwd; see conptyStart).
func sysRootPath(name string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return root + `\System32\` + name
}
