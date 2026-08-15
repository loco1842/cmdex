//go:build windows

package main

// Phase 2 spike (see tasks/plan.md): validates github.com/charmbracelet/x/conpty
// (the library decided on after design review — see "Library choice" in
// tasks/plan.md; github.com/UserExistsError/conpty was rejected because its
// Close() destroys the process handle its own Wait() depends on) against the
// three highest-risk contract items before committing to the full Phase 3
// implementation:
//
//  1. Does Close() unblock a pending Read()? (deadlock risk — TerminalService's
//     Close/Stop/restart/shutdown paths call handle.Close() then
//     readerWg.Wait(), so a wedged Read hangs the whole app.)
//  2. Does Wait (WaitForSingleObject + GetExitCodeProcess) return the true
//     exit code, both on a normal exit and after an external TerminateProcess
//     (standing in for conptyBackend.Kill)? A wrong/erroring result here
//     triggers monitorExit's 100ms auto-restart loop forever.
//  3. Does Read ever return (0, nil) in a tight loop? (busy-spin risk —
//     readLoop has no protection against this.)
//
// This talks to the conpty library directly via its raw API (New/Spawn/Read/
// Write/Resize/Close), not through ptyBackend/TerminalService — wiring a real
// conptyBackend into TerminalService is Phase 3's job. This file exists
// because none of the above can be verified any other way: there is no local
// Windows machine in this project's environment, only this repo's Windows CI
// job (added in Phase 0).

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/conpty"
	"golang.org/x/sys/windows"
)

func cmdExePath(t *testing.T) string {
	t.Helper()
	path, err := windows.GetSystemDirectory()
	if err != nil {
		path = `C:\Windows\System32`
	}
	full := path + `\cmd.exe`
	return full
}

// spawnConpty starts cmd.exe (optionally with /c and a command string)
// attached to a fresh ConPty, sized 80x24. Callers own the returned process
// handle and must windows.CloseHandle it, and must cp.Close() the ConPty.
func spawnConpty(t *testing.T, args ...string) (cp *conpty.ConPty, handle windows.Handle, pid int) {
	t.Helper()
	cp, err := conpty.New(80, 24, 0)
	if err != nil {
		t.Fatalf("conpty.New: %v", err)
	}
	shellPath := cmdExePath(t)
	argv := append([]string{shellPath}, args...)
	rawPid, rawHandle, err := cp.Spawn(shellPath, argv, nil)
	if err != nil {
		_ = cp.Close()
		t.Fatalf("Spawn: %v", err)
	}
	return cp, windows.Handle(rawHandle), rawPid
}

func waitForExit(t *testing.T, h windows.Handle, timeout time.Duration) uint32 {
	t.Helper()
	ev, err := windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
	if err != nil {
		t.Fatalf("WaitForSingleObject: %v", err)
	}
	if ev != windows.WAIT_OBJECT_0 {
		t.Fatalf("WaitForSingleObject returned event %d, want WAIT_OBJECT_0", ev)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		t.Fatalf("GetExitCodeProcess: %v", err)
	}
	return code
}

// TestConptySpike_CloseUnblocksRead answers risk #1.
func TestConptySpike_CloseUnblocksRead(t *testing.T) {
	cp, handle, _ := spawnConpty(t) // interactive cmd.exe, sits at a prompt
	defer func() { _ = windows.CloseHandle(handle) }()

	readReturned := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		_, err := cp.Read(buf)
		readReturned <- err
	}()

	// Give the read a moment to actually start blocking before closing.
	time.Sleep(200 * time.Millisecond)
	closeErr := cp.Close()

	select {
	case err := <-readReturned:
		t.Logf("Read unblocked after Close (readErr=%v, closeErr=%v)", err, closeErr)
	case <-time.After(5 * time.Second):
		t.Fatal("Read did not unblock within 5s of Close() — deadlock risk confirmed; " +
			"Phase 3 must use the defensive pump-goroutine design regardless (see tasks/plan.md)")
	}

	_ = windows.TerminateProcess(handle, 1)
}

// TestConptySpike_ExitCodeNormal answers risk #2, part A.
func TestConptySpike_ExitCodeNormal(t *testing.T) {
	cp, handle, _ := spawnConpty(t, "/c", "exit 7")
	defer func() { _ = cp.Close() }()
	defer func() { _ = windows.CloseHandle(handle) }()

	code := waitForExit(t, handle, 10*time.Second)
	if code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
}

// TestConptySpike_ExitCodeAfterTerminate answers risk #2, part B — standing
// in for conptyBackend.Kill (contract item 7: Kill must not reap; monitorExit
// alone owns Wait, so Wait must still complete correctly after Kill runs).
func TestConptySpike_ExitCodeAfterTerminate(t *testing.T) {
	cp, handle, _ := spawnConpty(t) // interactive; would otherwise never exit
	defer func() { _ = cp.Close() }()
	defer func() { _ = windows.CloseHandle(handle) }()

	const killExitCode = 1
	if err := windows.TerminateProcess(handle, killExitCode); err != nil {
		t.Fatalf("TerminateProcess: %v", err)
	}

	code := waitForExit(t, handle, 10*time.Second)
	if code != killExitCode {
		t.Errorf("exit code after TerminateProcess = %d, want %d", code, killExitCode)
	}
}

// TestConptySpike_ReadNeverBusySpins answers risk #3. Read runs on its own
// goroutine so a subsequent indefinite block (expected once the burst ends —
// neither library's pipe breaks on child exit) doesn't hang the test; hitting
// the overall timeout is a pass, not a failure. Only a repeated (0, nil)
// mid-burst would indicate the busy-spin risk.
func TestConptySpike_ReadNeverBusySpins(t *testing.T) {
	cp, handle, _ := spawnConpty(t, "/c", "dir /s C:\\Windows\\System32\\drivers")
	defer func() { _ = windows.CloseHandle(handle) }()

	type readResult struct {
		n   int
		err error
	}
	results := make(chan readResult, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := cp.Read(buf)
			results <- readResult{n, err}
			if err != nil {
				return
			}
		}
	}()

	zeroByteNoErrStreak := 0
	totalReads := 0
	timeout := time.After(8 * time.Second)
loop:
	for {
		select {
		case r := <-results:
			totalReads++
			if r.err != nil {
				break loop
			}
			if r.n == 0 {
				zeroByteNoErrStreak++
				if zeroByteNoErrStreak > 3 {
					t.Fatalf("Read returned (0, nil) %d times in a row — busy-spin risk confirmed", zeroByteNoErrStreak)
				}
			} else {
				zeroByteNoErrStreak = 0
			}
		case <-timeout:
			// Burst finished and Read is now blocked waiting for more input
			// forever (expected, per the pipe-doesn't-break-on-exit finding)
			// — not a failure.
			break loop
		}
	}
	t.Logf("totalReads=%d", totalReads)

	_ = windows.TerminateProcess(handle, 1)
	_ = cp.Close()
}

// TestConptySpike_FullCycle exercises Start→Write→Read→Resize→Close end to
// end — the "smallest thing that proves the contract" bar from
// tasks/plan.md Phase 2.
func TestConptySpike_FullCycle(t *testing.T) {
	cp, handle, _ := spawnConpty(t) // interactive cmd.exe
	defer func() { _ = windows.CloseHandle(handle) }()

	const marker = "hello-conpty-spike"
	if _, err := cp.Write([]byte("echo " + marker + "\r\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	type readResult struct {
		data []byte
		err  error
	}
	results := make(chan readResult, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := cp.Read(buf)
			data := append([]byte(nil), buf[:n]...)
			results <- readResult{data, err}
			if err != nil {
				return
			}
		}
	}()

	var accumulated strings.Builder
	found := false
	timeout := time.After(9 * time.Second)
readLoop:
	for !found {
		select {
		case r := <-results:
			accumulated.Write(r.data)
			if strings.Contains(accumulated.String(), marker) {
				found = true
			}
			if r.err != nil {
				break readLoop
			}
		case <-timeout:
			break readLoop
		}
	}
	if !found {
		t.Fatalf("did not observe echoed marker %q within timeout; got: %q", marker, accumulated.String())
	}

	if err := cp.Resize(120, 40); err != nil {
		t.Errorf("Resize: %v", err)
	}

	if _, err := cp.Write([]byte("exit 0\r\n")); err != nil {
		t.Fatalf("Write exit: %v", err)
	}

	code := waitForExit(t, handle, 10*time.Second)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}

	_ = cp.Close()
}
