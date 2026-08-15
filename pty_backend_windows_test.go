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
