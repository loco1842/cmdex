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
func (creackPtyBackend) Start(shellPath, shellFlag string, rows, cols int) (ptyHandle, *exec.Cmd, error) {
	ptmx, cmd, err := ptyStart(shellPath, shellFlag, rows, cols)
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
func ptyStart(shellPath, shellFlag string, rows, cols int, extraArgs ...string) (*os.File, *exec.Cmd, error) {
	args := []string{shellFlag}
	args = append(args, extraArgs...)
	cmd := exec.CommandContext(context.Background(), shellPath, args...)
	cmd.Env = os.Environ()

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, nil, fmt.Errorf("pty.StartWithSize: %w", err)
	}

	return ptmx, cmd, nil
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
