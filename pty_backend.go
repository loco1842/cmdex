package main

import (
	"io"
	"os/exec"
)

// ptyBackend is the seam between TerminalService and the OS PTY layer.
// It exists so the OS-specific implementation (creack/pty on darwin, conpty
// stub on windows) can be swapped at test time via the darwin mock.
type ptyBackend interface {
	Start(shellPath, shellFlag, dir string, rows, cols int) (ptyHandle, *exec.Cmd, error)
	Resize(handle ptyHandle, cols, rows int) error
	Kill(cmd *exec.Cmd) error
}

// ptyHandle is the I/O surface a started PTY exposes to TerminalService.
// *os.File satisfies this interface (it implements io.ReadWriteCloser), so
// the production code path is unchanged for darwin; the darwin-side mock
// also satisfies it via mockPtyHandle.
type ptyHandle interface {
	io.ReadWriteCloser
}
