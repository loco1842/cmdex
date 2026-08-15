//go:build windows

package main

// Windows-tagged integration tests for the real conptyBackend (Phase 3, see
// tasks/plan.md). These are deliberately narrow: un-skipping the Windows
// guard in newTestTerminalService/testWithTerminalSvc (Phase 0) already
// makes the bulk of the existing untagged test suite exercise the real
// backend on Windows for free — TestTerminalStart/Write/Resize,
// TestTerminalService_CreateSession/ListSessions/RenameSession/CloseSession/
// ActiveSessionReassignOnClose/SetActiveSession/GetActiveSession/
// ShutdownCleansAll/ConcurrentAccess/ProcessPersistAcrossSessionSwitch/
// OutputIsolation/NamespacedEvents/GlobalDefaultCwdInheritance/
// CwdInheritance_ExistingSessionUnaffected all now run against the real
// conpty backend. ConcurrentAccess and ShutdownCleansAll in particular
// already create/close many sessions rapidly, which is the same class of
// deadlock risk a dedicated stress test would probe (contract item 5) — a
// hang there fails CI just as loudly as a dedicated test would, so this file
// does not duplicate that coverage.
//
// What's added here is what the existing suite does NOT cover: real output
// round-tripping through outputCh (not just "Write returned no error"),
// confirming Kill actually terminates the OS process (not just that
// TerminalService's own bookkeeping was cleared), the bad-working-directory
// retry path, and dimension validation against the real conptyStart (the
// legacy ptyStart shim on Windows no longer does real work — see
// pty_backend_windows.go).

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestConptyBackend_WriteReadRoundTrip verifies that real shell output
// actually round-trips through TerminalService's outputCh on Windows.
func TestConptyBackend_WriteReadRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	info, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	const marker = "cmdex-windows-roundtrip-marker"
	if err := s.Write(info.ID, "echo "+marker+"\r\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	ss, err := s.resolveSession(info.ID)
	if err != nil {
		t.Fatalf("resolveSession failed: %v", err)
	}

	var accumulated strings.Builder
	deadline := time.After(10 * time.Second)
	for {
		select {
		case data := <-ss.outputCh:
			accumulated.WriteString(data)
			if strings.Contains(accumulated.String(), marker) {
				return
			}
		case <-deadline:
			t.Fatalf("marker %q not observed in output within timeout; got: %q", marker, accumulated.String())
		}
	}
}

// TestConptyBackend_CloseSessionTerminatesProcess verifies that
// CloseSession's Kill actually terminates the OS process (contract 7/8),
// not just that TerminalService's own bookkeeping is cleared (already
// covered by TestTerminalService_CloseSession).
func TestConptyBackend_CloseSessionTerminatesProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	info, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	ss, err := s.resolveSession(info.ID)
	if err != nil {
		t.Fatalf("resolveSession failed: %v", err)
	}
	ss.mu.Lock()
	pid := uint32(0)
	if ss.proc != nil {
		pid = uint32(ss.proc.Pid())
	}
	ss.mu.Unlock()
	if pid == 0 {
		t.Fatal("session process has no PID")
	}

	if err := s.CloseSession(info.ID); err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			return // process is gone — success
		}
		_ = windows.CloseHandle(h)
		if time.Now().After(deadline) {
			t.Fatalf("process pid=%d still exists 5s after CloseSession", pid)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestConptyBackend_BadWorkingDirFallback verifies contract 2: a stale or
// nonexistent configured working directory must not prevent a session from
// starting — conptyStart retries with the directory cleared, mirroring
// ptyStart's behavior on unix (pty_backend_unix.go).
func TestConptyBackend_BadWorkingDirFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	defer cwdInheritanceTestDB(t)()

	badDir := filepath.Join(t.TempDir(), "does-not-exist")
	setGlobalDefaultCwd(t, badDir)

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	info, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession with a nonexistent configured working dir failed "+
			"(should have fallen back instead): %v", err)
	}

	ss, err := s.resolveSession(info.ID)
	if err != nil {
		t.Fatalf("resolveSession failed: %v", err)
	}
	ss.mu.Lock()
	running := ss.running
	pid := 0
	if ss.proc != nil {
		pid = ss.proc.Pid()
	}
	ss.mu.Unlock()
	if !running || pid == 0 {
		t.Error("session did not start a real process despite the bad-working-dir fallback")
	}
}

// TestConptyStart_RejectsInvalidDimensions mirrors
// TestPtyStart_RejectsInvalidDimensions (terminal_service_test.go) against
// the real conptyStart implementation — the legacy ptyStart shim on Windows
// no longer does real work, so it can't exercise this validation for real.
func TestConptyStart_RejectsInvalidDimensions(t *testing.T) {
	shellPath, shellFlag := detectShell()

	cases := []struct {
		name       string
		rows, cols int
	}{
		{"zero rows", 0, 80},
		{"zero cols", 24, 0},
		{"negative rows", -1, 80},
		{"negative cols", 24, -1},
		{"rows too large", 65536, 80},
		{"cols too large", 24, 65536},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handle, proc, err := conptyStart(shellPath, shellFlag, "", tc.rows, tc.cols)
			if err == nil {
				t.Fatalf("conptyStart(rows=%d, cols=%d) succeeded, want error", tc.rows, tc.cols)
			}
			if handle != nil || proc != nil {
				t.Errorf("conptyStart returned non-nil handle/proc alongside an error")
			}
		})
	}
}

// TestConptyProcess_ConcurrentWaitReturnsSameResult exercises the exact
// property conptyProcess.Wait's sync.Once guard exists for — the same
// invariant TestExecProcess_ConcurrentWaitReturnsSameResult
// (pty_backend_unix_test.go) proves for the unix execProcess type. Unlike
// execProcess (waited on concurrently by monitorExit and
// killProcessGroup's SIGKILL-escalation goroutine in production),
// conptyProcess.Wait is never called concurrently anywhere in production —
// conptyBackend.Kill deliberately never reaps (contract 7/8, tasks/plan.md)
// — so nothing else in this suite exercises the guard under real
// concurrency. Uses cmd.exe directly (via the spike test's cmdExePath
// helper, same package) rather than detectShell's result so the exit code
// is deterministic regardless of whether pwsh/powershell is on CI's PATH.
func TestConptyProcess_ConcurrentWaitReturnsSameResult(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	handle, proc, err := conptyStart(cmdExePath(t), "", "", 24, 80, "/C", "exit 7")
	if err != nil {
		t.Fatalf("conptyStart failed: %v", err)
	}
	defer func() { _ = handle.Close() }()

	const goroutines = 10
	codes := make([]int, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range codes {
		go func(i int) {
			defer wg.Done()
			codes[i], errs[i] = proc.Wait()
		}(i)
	}
	wg.Wait()

	if !proc.Exited() {
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

// TestConptyHandle_CloseUnblocksRead verifies the production conptyHandle's
// Close() unblocks a pending Read() (contract 5, tasks/plan.md). The Phase-2
// spike (TestConptySpike_CloseUnblocksRead,
// pty_backend_windows_conpty_spike_test.go) only proved this for the raw
// conpty library — conptyHandle adds its own pump-goroutine/done-channel
// layer specifically because Close-unblocks-Read isn't trusted to hold in
// general on Win32 (see the comment on conptyHandle in
// pty_backend_windows.go), so this exercises the actual shipped wrapper
// rather than just the library underneath it.
func TestConptyHandle_CloseUnblocksRead(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	shellPath, shellFlag := detectShell()
	handle, proc, err := conptyStart(shellPath, shellFlag, "", 24, 80)
	if err != nil {
		t.Fatalf("conptyStart failed: %v", err)
	}

	readReturned := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		_, readErr := handle.Read(buf)
		readReturned <- readErr
	}()

	if err := handle.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	select {
	case readErr := <-readReturned:
		if readErr == nil {
			t.Error("Read returned nil error after Close; want a non-nil error (os.ErrClosed or io.EOF)")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not return within 2s of Close() — conptyHandle.Close did not unblock a pending Read")
	}

	// Close already calls proc.terminate(); reap here so the process handle
	// is closed rather than leaked for the remainder of the test run.
	_, _ = proc.Wait()
}
