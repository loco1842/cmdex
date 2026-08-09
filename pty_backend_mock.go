//go:build darwin

package main

// mockPtyBackend + mockPtyHandle are a darwin-only in-memory ptyBackend
// implementation for orchestration tests. The mock covers PTY spawn + I/O +
// resize without spawning a real long-lived process. It does NOT implement
// process group signal semantics or exit detection — those rely on real
// conpty and are out of scope per D-12.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// mockPtyBackend is an in-memory ptyBackend for darwin tests.
type mockPtyBackend struct{}

// Start returns a mockPtyHandle pre-sized to rows/cols and a real *exec.Cmd
// running "sleep 0.05" so that monitorExit's cmd.Wait() returns promptly
// (the test does not need a long-lived shell to validate the orchestration
// path).
func (mockPtyBackend) Start(shellPath, shellFlag string, rows, cols int) (ptyHandle, *exec.Cmd, error) {
	cmd := exec.CommandContext(context.Background(), "sleep", "0.05")
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return &mockPtyHandle{
		output: &bytes.Buffer{},
		input:  &bytes.Buffer{},
		cols:   cols,
		rows:   rows,
	}, cmd, nil
}

// Resize updates the recorded cols/rows on a mockPtyHandle.
func (mockPtyBackend) Resize(handle ptyHandle, cols, rows int) error {
	h, ok := handle.(*mockPtyHandle)
	if !ok {
		return fmt.Errorf("mockPtyBackend.Resize: unexpected handle type %T", handle)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cols = cols
	h.rows = rows
	return nil
}

// Kill terminates the sleep process spawned by Start, if any.
func (mockPtyBackend) Kill(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// mockPtyHandle is an in-memory ptyHandle. Read drains the output buffer (or
// returns io.EOF when empty), Write records into input, Close marks the
// handle closed so subsequent I/O returns os.ErrClosed.
type mockPtyHandle struct {
	mu     sync.Mutex
	closed bool
	input  *bytes.Buffer
	output *bytes.Buffer
	cols   int
	rows   int
}

func (h *mockPtyHandle) Read(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return 0, os.ErrClosed
	}
	if h.output.Len() == 0 {
		return 0, io.EOF
	}
	return h.output.Read(p)
}

func (h *mockPtyHandle) Write(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return 0, os.ErrClosed
	}
	return h.input.Write(p)
}

func (h *mockPtyHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	return nil
}
