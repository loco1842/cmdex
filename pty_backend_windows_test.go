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
// actually round-trips through the production conptyHandle on Windows.
//
// Spawns via conptyStart directly rather than through
// TerminalService.CreateSession: CreateSession's startEmitter starts a
// goroutine that is the session's sole intended consumer of ss.outputCh.
// A test reading directly from that same channel would be a second,
// competing consumer — and since the emitter goroutine registers as a
// waiting receiver well before the test's own select can, it would almost
// always win each send, silently discarding the marker (wailsApp is nil in
// tests, so the emitter's periodic flush has nowhere to send it) and
// leaving the test to spin until its deadline every time. TestTerminalWrite
// (terminal_service_test.go) already covers TerminalService.Write returning
// no error; this test's job is only to prove real byte content round-trips
// through the actual conptyHandle, which doesn't need the emitter at all.
func TestConptyBackend_WriteReadRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	shellPath, shellFlag := detectShell()
	handle, proc, err := conptyStart(shellPath, shellFlag, "", 24, 80)
	if err != nil {
		t.Fatalf("conptyStart failed: %v", err)
	}
	defer func() {
		_ = handle.Close()
		_, _ = proc.Wait()
	}()

	const marker = "cmdex-windows-roundtrip-marker"
	cmd := []byte("echo " + marker + "\r\n")
	for len(cmd) > 0 {
		n, err := handle.Write(cmd)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n == 0 {
			t.Fatalf("Write made no progress (0 bytes, no error) with %d bytes remaining", len(cmd))
		}
		cmd = cmd[n:]
	}

	chunks := make(chan []byte, 16)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := handle.Read(buf)
			if n > 0 {
				b := make([]byte, n)
				copy(b, buf[:n])
				chunks <- b
			}
			if err != nil {
				close(chunks)
				return
			}
		}
	}()

	var accumulated strings.Builder
	deadline := time.After(10 * time.Second)
	for {
		select {
		case b, ok := <-chunks:
			if !ok {
				t.Fatalf("handle closed before marker observed; got: %q", accumulated.String())
			}
			accumulated.Write(b)
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

// TestConptyProcess_WaitForSingleObjectFailure verifies Wait() returns the
// real WaitForSingleObject error rather than swallowing it to nil (see
// Wait's doc comment in pty_backend_windows.go for why exitCode still
// reports 0 in this path regardless). Constructs a conptyProcess directly
// around a handle value that isn't a live process — WaitForSingleObject
// fails immediately on it rather than blocking, so this doesn't require a
// real child process at all.
func TestConptyProcess_WaitForSingleObjectFailure(t *testing.T) {
	p := newConptyProcess(0, windows.Handle(0xDEADBEEF))

	code, err := p.Wait()
	if err == nil {
		t.Fatal("Wait() with an invalid handle returned a nil error, want the real WaitForSingleObject failure")
	}
	if code != 0 {
		t.Errorf("exitCode = %d, want 0", code)
	}
	if !p.Exited() {
		t.Error("Exited() = false after Wait() completed")
	}

	code2, err2 := p.Wait()
	if code2 != code || err2 != err {
		t.Errorf("second Wait() call returned (%d, %v), want the identical cached result (%d, %v)", code2, err2, code, err)
	}
}

// TestConptyProcess_GetExitCodeProcessFailure verifies Wait() returns the
// real GetExitCodeProcess error rather than swallowing it to nil, exercised
// independently of TestConptyProcess_WaitForSingleObjectFailure above.
// GetExitCodeProcess requires PROCESS_QUERY_INFORMATION or
// PROCESS_QUERY_LIMITED_INFORMATION access, unlike WaitForSingleObject
// (SYNCHRONIZE alone suffices) — so a handle opened with only SYNCHRONIZE
// to a real, already-exited process makes WaitForSingleObject succeed
// immediately while GetExitCodeProcess fails on that same handle, reaching
// the second failure branch specifically.
//
// An earlier version of this test used an already-signaled event handle
// instead of a real process, assuming GetExitCodeProcess would reject any
// non-process handle; real Windows CI showed it does not reliably fail
// that way, so this uses an actual process with deliberately reduced
// rights instead — the access check GetExitCodeProcess is documented to
// make.
func TestConptyProcess_GetExitCodeProcessFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	handle, proc, err := conptyStart(cmdExePath(t), "", "", 24, 80, "/C", "exit 0")
	if err != nil {
		t.Fatalf("conptyStart failed: %v", err)
	}
	defer func() { _ = handle.Close() }()

	// Opened independently of conptyStart's own process handle, while the
	// child is guaranteed still alive (or at least not yet reaped), so the
	// PID cannot have been recycled out from under this call.
	syncOnly, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(proc.Pid()))
	if err != nil {
		t.Fatalf("OpenProcess failed: %v", err)
	}
	// syncOnly is closed by p.Wait() below (via conptyProcess's own
	// closeHandleLocked), same as every other handle this package's
	// conptyProcess owns — no separate manual close needed.

	// Reap the real process for real via the normal path, guaranteeing it
	// has actually exited so WaitForSingleObject on syncOnly below returns
	// immediately instead of blocking.
	if _, err := proc.Wait(); err != nil {
		t.Fatalf("Wait on the real process failed: %v", err)
	}

	p := newConptyProcess(0, syncOnly)
	code, waitErr := p.Wait()
	if waitErr == nil {
		t.Fatal("Wait() with a SYNCHRONIZE-only handle returned a nil error, want the real GetExitCodeProcess access-denied failure")
	}
	if code != 0 {
		t.Errorf("exitCode = %d, want 0", code)
	}
	if !p.Exited() {
		t.Error("Exited() = false after Wait() completed")
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
