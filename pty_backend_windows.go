//go:build windows

package main

// Windows PTY (conpty) is not yet implemented. The ptyBackend seam exists so
// TerminalService never has to know which OS it's running on; on windows it
// receives a conptyBackend stub that returns "not yet implemented" for Start
// and Resize, and a working killProcessGroup for Kill (the taskkill helper
// is platform-portable).

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// newPtyBackend returns the conpty stub ptyBackend for windows.
func newPtyBackend() ptyBackend {
	return conptyBackend{}
}

// conptyBackend is the windows conpty-stub ptyBackend implementation.
type conptyBackend struct{}

// Start returns an error — Windows conpty is not yet implemented.
func (conptyBackend) Start(shellPath, shellFlag, dir string, rows, cols int) (ptyHandle, *exec.Cmd, error) {
	return nil, nil, fmt.Errorf("Windows PTY support not yet implemented — see Plan 16-03")
}

// Resize returns an error — Windows conpty is not yet implemented.
func (conptyBackend) Resize(handle ptyHandle, cols, rows int) error {
	return fmt.Errorf("Windows PTY support not yet implemented")
}

// Kill terminates the process group using taskkill.
func (conptyBackend) Kill(cmd *exec.Cmd) error {
	return killProcessGroup(cmd)
}

// ptyStart is the windows-side stub preserved as a package-level function so
// test code referencing ptyStart continues to compile on windows.
func ptyStart(shellPath, shellFlag, dir string, rows, cols int, extraArgs ...string) (*os.File, *exec.Cmd, error) {
	return nil, nil, fmt.Errorf("Windows PTY support not yet implemented — see Plan 16-03")
}

func ptyResize(ptmx *os.File, cols, rows int) error {
	return fmt.Errorf("Windows PTY support not yet implemented")
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}
