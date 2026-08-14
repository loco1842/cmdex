//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// newPtyBackend returns the creack/pty-backed ptyBackend for darwin/linux.
const ptyKillTimeout = 2 * time.Second

func newPtyBackend() ptyBackend {
	return creackPtyBackend{}
}

// creackPtyBackend is the creack/pty-backed ptyBackend implementation.
type creackPtyBackend struct{}

// Start spawns a shell attached to a new PTY, returning an io.ReadWriteCloser
// handle for the PTY master.
func (creackPtyBackend) Start(shellPath, shellFlag, dir string, rows, cols int) (ptyHandle, *exec.Cmd, error) {
	ptmx, cmd, err := ptyStart(shellPath, shellFlag, dir, rows, cols)
	if err != nil {
		return nil, nil, err
	}
	return osFileHandle{f: ptmx}, cmd, nil
}

// Resize updates the PTY window size.
func (creackPtyBackend) Resize(handle ptyHandle, cols, rows int) error {
	oh, ok := handle.(osFileHandle)
	if !ok {
		return fmt.Errorf("creackPtyBackend.Resize: unexpected handle type %T", handle)
	}
	return ptyResize(oh.f, cols, rows)
}

// Kill terminates the process group associated with cmd.
func (creackPtyBackend) Kill(cmd *exec.Cmd) error {
	return killProcessGroup(cmd)
}

// osFileHandle adapts a *os.File (creack/pty's PTY master) to ptyHandle.
type osFileHandle struct {
	f *os.File
}

func (h osFileHandle) Read(p []byte) (int, error)  { return h.f.Read(p) }
func (h osFileHandle) Write(p []byte) (int, error) { return h.f.Write(p) }
func (h osFileHandle) Close() error                { return h.f.Close() }

// ptyStart spawns the shell with a fresh PTY sized to rows/cols. The returned
// *os.File is the PTY master; callers should Close it on shutdown.
//
// Starting a session must not fail just because its configured working
// directory turned out to be unusable (deleted, unmounted, permission
// denied, or a stat/chdir race) — that used to be true by construction
// because cmd.Dir was never set at all. To preserve it, a failed attempt
// with cmd.Dir set is retried once with cmd.Dir cleared (inheriting the
// app's own cwd) before giving up.
func ptyStart(shellPath, shellFlag, dir string, rows, cols int, extraArgs ...string) (*os.File, *exec.Cmd, error) {
	if rows < 1 || rows > 65535 || cols < 1 || cols > 65535 {
		return nil, nil, fmt.Errorf("ptyStart: invalid dimensions rows=%d cols=%d (must be 1..65535)", rows, cols)
	}

	var args []string
	if shellFlag != "" {
		args = append(args, shellFlag)
	}
	args = append(args, extraArgs...)
	env := buildPtyEnv(os.Environ())
	winsize := &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}

	newCmd := func(dir string) *exec.Cmd {
		cmd := exec.CommandContext(context.Background(), shellPath, args...)
		cmd.Env = env
		cmd.Dir = dir
		return cmd
	}

	cmd := newCmd(resolvePtyWorkingDir(dir))
	ptmx, err := pty.StartWithSize(cmd, winsize)
	if err != nil && cmd.Dir != "" {
		cmd = newCmd("")
		ptmx, err = pty.StartWithSize(cmd, winsize)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("pty.StartWithSize: %w", err)
	}

	return ptmx, cmd, nil
}

// resolvePtyWorkingDir returns dir if it exists and is a directory, falling
// back to the OS user home directory, then to "" (inherit the app's cwd) if
// neither is usable. This is a best-effort pre-check only — ptyStart still
// retries without a working directory if the chosen one fails at exec time
// (deleted between check and use, or missing execute permission) — but it
// keeps the common case (a stale/deleted configured working directory) from
// needing that fallback path. e.g. getWorkingDir() can return a
// saved-but-now-missing custom path, or "" if os.UserHomeDir() itself
// failed.
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

func ptyResize(ptmx *os.File, cols, rows int) error {
	return pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid

	_ = syscall.Kill(-pid, syscall.SIGHUP)

	if cmd.ProcessState != nil {
		return nil
	}

	done := make(chan struct{}, 1)
	go func() {
		_ = cmd.Wait()
		done <- struct{}{}
	}()

	select {
	case <-done:
		return nil
	case <-time.After(ptyKillTimeout):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-done
		return nil
	}
}
