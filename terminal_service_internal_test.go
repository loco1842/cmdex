package main

import (
	"testing"
)

// The global quick launcher runs commands in its own "internal" session. These
// tests pin the invariants that keep that session out of the main window's UI
// and out of its "run in the active terminal" path.

func mustCreateInternal(t *testing.T, s *TerminalService) string {
	t.Helper()
	info, err := s.CreateInternalSession("Launcher")
	if err != nil {
		t.Fatalf("CreateInternalSession failed: %v", err)
	}
	t.Cleanup(func() {
		s.CloseSession(info.ID)
	})
	return info.ID
}

func TestInternalSessionHiddenFromListSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	s := newTestTerminalService(t)

	visibleID := mustCreateAndStart(t, s)
	internalID := mustCreateInternal(t, s)

	listed := s.ListSessions()
	if len(listed) != 1 {
		t.Fatalf("ListSessions returned %d sessions, want 1", len(listed))
	}
	if listed[0].ID != visibleID {
		t.Errorf("ListSessions returned %s, want the non-internal session %s", listed[0].ID, visibleID)
	}
	for _, info := range listed {
		if info.ID == internalID {
			t.Error("internal session leaked into ListSessions and would appear as a terminal tab")
		}
	}
}

// An internal session created before any user session must not become the
// active one, or the main window's Run button would write into the launcher.
func TestInternalSessionNeverBecomesActive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	s := newTestTerminalService(t)

	internalID := mustCreateInternal(t, s)

	if active := s.GetActiveSession(); active != nil {
		t.Fatalf("internal session became active (%s); active should still be empty", active.ID)
	}

	visibleID := mustCreateAndStart(t, s)
	active := s.GetActiveSession()
	if active == nil {
		t.Fatal("no active session after creating a user session")
	}
	if active.ID != visibleID {
		t.Errorf("active session is %s, want the user session %s", active.ID, visibleID)
	}
	if active.ID == internalID {
		t.Error("active session is the internal launcher session")
	}
}

func TestSetActiveSessionRejectsInternal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	s := newTestTerminalService(t)

	internalID := mustCreateInternal(t, s)

	if err := s.SetActiveSession(internalID); err == nil {
		t.Error("SetActiveSession accepted an internal session, want an error")
	}
}

// Closing the last user session must not silently promote the launcher's
// session to active.
func TestCloseSessionDoesNotPromoteInternal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	s := newTestTerminalService(t)

	internalID := mustCreateInternal(t, s)
	visibleID := mustCreateAndStart(t, s)

	if err := s.CloseSession(visibleID); err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	if active := s.GetActiveSession(); active != nil {
		t.Errorf("active session is %s after closing the only user session; internal session %s should not be promoted", active.ID, internalID)
	}
}

// Internal sessions must not consume numbers from the user-visible
// "Terminal N" sequence.
func TestInternalSessionDoesNotConsumeSessionNumber(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	s := newTestTerminalService(t)

	mustCreateInternal(t, s)

	info, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	t.Cleanup(func() { s.CloseSession(info.ID) })

	if info.Name != "Terminal 1" {
		t.Errorf("first user session is named %q, want %q", info.Name, "Terminal 1")
	}
}
