//go:build darwin

package main

import (
	"runtime"
	"strings"
	"testing"
)

// TestTerminalService_MaxSessionsLimit verifies that CreateSession rejects
// calls once the active session count would reach MaxSessions, returning an
// error whose string contains "max sessions reached". The test uses
// newTestTerminalServiceWithMock (also //go:build darwin) so the loop does
// not depend on a real PTY — the limit is an orchestration concern.
//
// Deviations from PLAN: 25-03 task 1 says the test goes in
// terminal_service_test.go. That file has no build tag; referencing
// newTestTerminalServiceWithMock (which is in a //go:build darwin file) from
// a non-tagged _test.go would break cross-platform test compilation, the
// same reason the 25-02 stress test was placed in terminal_service_stress_test.go.
// The behavior under test is unchanged.
func TestTerminalService_MaxSessionsLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test (uses newTestTerminalServiceWithMock from Plan 25-01)")
	}

	s := newTestTerminalServiceWithMock(t)
	defer s.ServiceShutdown()

	for i := 0; i < MaxSessions; i++ {
		info, err := s.CreateSession()
		if err != nil {
			t.Fatalf("CreateSession %d (within limit) failed: %v", i+1, err)
		}
		if info == nil {
			t.Fatalf("CreateSession %d returned nil SessionInfo", i+1)
		}
	}

	info, err := s.CreateSession()
	if err == nil {
		t.Fatalf("expected error on CreateSession past MaxSessions, got SessionInfo=%+v", info)
	}
	if info != nil {
		t.Errorf("expected nil SessionInfo on max-sessions error, got %+v", info)
	}
	if !strings.Contains(err.Error(), "max sessions reached") {
		t.Errorf("expected error to contain 'max sessions reached', got: %v", err)
	}
}

// TestTerminalService_MaxSessionsIgnoresInternal pins the boundary from the
// other side: the launcher's hidden session must neither consume one of the
// user's MaxSessions slots nor be refused once those slots are full.
func TestTerminalService_MaxSessionsIgnoresInternal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test (uses newTestTerminalServiceWithMock from Plan 25-01)")
	}

	s := newTestTerminalServiceWithMock(t)
	defer s.ServiceShutdown()

	// An internal session created first must not reduce user capacity.
	if _, err := s.CreateInternalSession("Launcher"); err != nil {
		t.Fatalf("CreateInternalSession failed: %v", err)
	}

	for i := 0; i < MaxSessions; i++ {
		if _, err := s.CreateSession(); err != nil {
			t.Fatalf("CreateSession %d (within limit) failed with an internal session present: %v", i+1, err)
		}
	}

	if info, err := s.CreateSession(); err == nil {
		t.Fatalf("expected error on CreateSession past MaxSessions, got SessionInfo=%+v", info)
	}

	// And a second internal session must still be allowed with the user's
	// slots full, so the launcher keeps working.
	if _, err := s.CreateInternalSession("Launcher 2"); err != nil {
		t.Errorf("CreateInternalSession refused while user sessions are at the limit: %v", err)
	}
}
