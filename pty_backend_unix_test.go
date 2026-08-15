//go:build !windows

package main

import (
	"context"
	"os/exec"
	"sync"
	"testing"
)

// TestExecProcess_ConcurrentWaitReturnsSameResult targets the exact defect
// this refactor exists to fix: os/exec forbids calling cmd.Wait() more than
// once, but monitorExit and killProcessGroup's SIGKILL-escalation goroutine
// both need to wait for the same process to exit. execProcess's
// sync.Once-guarded Wait (pty_backend.go) is supposed to make this safe —
// this test calls it from many goroutines concurrently and asserts they all
// observe the identical result, with no -race failure.
func TestExecProcess_ConcurrentWaitReturnsSameResult(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "exit 7")
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start failed: %v", err)
	}
	p := newExecProcess(cmd)

	const goroutines = 10
	codes := make([]int, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range codes {
		go func(i int) {
			defer wg.Done()
			codes[i], errs[i] = p.Wait()
		}(i)
	}
	wg.Wait()

	if !p.Exited() {
		t.Error("Exited() = false after Wait() completed")
	}

	for i := 1; i < goroutines; i++ {
		if codes[i] != codes[0] {
			t.Errorf("goroutine %d observed exit code %d, want %d (same as goroutine 0)", i, codes[i], codes[0])
		}
		if errs[i] != errs[0] {
			t.Errorf("goroutine %d observed err %v, want %v (same as goroutine 0)", i, errs[i], errs[0])
		}
	}
	if codes[0] != 7 {
		t.Errorf("exit code = %d, want 7", codes[0])
	}
}
