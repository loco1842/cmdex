//go:build darwin

package main

import (
	"testing"
	"time"
)

// racyPtyBackend wraps mockPtyBackend, gating Start on an external signal so
// a test can deterministically land a concurrent Stop/CloseSession call
// exactly inside startSessionLocked's unlocked window (see its own comment
// on why it unlocks ss.mu around the blocking shell-integration setup and
// ptyBackend.Start calls). Start closes started right as it's entered (so
// the test knows it's safe to race), then blocks until unblock is closed.
type racyPtyBackend struct {
	mockPtyBackend
	started chan struct{}
	unblock chan struct{}
}

func (b *racyPtyBackend) Start(
	shellPath, shellFlag, dir string,
	rows, cols int,
	opts shellLaunchOpts,
) (ptyHandle, ptyProcess, error) {
	close(b.started)
	<-b.unblock
	return b.mockPtyBackend.Start(shellPath, shellFlag, dir, rows, cols, opts)
}

// TestStartSessionLocked_StopDuringStartDoesNotResurrectSession is a
// regression test for a review finding: startSessionLocked unlocks ss.mu
// around the blocking shell-integration setup and ptyBackend.Start calls,
// so a concurrent Stop could previously run to completion in that window
// and then be silently undone once the in-flight start relocked and
// published its own (now-unwanted) PTY as ss.ptmx/ss.proc with
// ss.running=true — resurrecting a session the caller had just asked to
// stop, with no way to ever stop it again if CloseSession had removed it
// from s.sessions in the meantime.
func TestStartSessionLocked_StopDuringStartDoesNotResurrectSession(t *testing.T) {
	s := &TerminalService{ptyBackend: mockPtyBackend{}}
	s.sessions = make(map[string]*sessionState)

	id := mustCreateAndStart(t, s)
	if err := s.Stop(id); err != nil {
		t.Fatalf("initial Stop failed: %v", err)
	}

	// Swap in a backend whose Start blocks until unblocked, opening a
	// deterministic window for a second Stop to race a fresh
	// startSessionLocked call.
	backend := &racyPtyBackend{started: make(chan struct{}), unblock: make(chan struct{})}
	s.ptyBackend = backend

	startErrCh := make(chan error, 1)
	go func() { startErrCh <- s.Start(id, 80, 24) }()

	select {
	case <-backend.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the racy backend's Start to be entered")
	}

	// Stop while startSessionLocked is unlocked and blocked inside
	// ptyBackend.Start — exactly the window this test targets.
	if err := s.Stop(id); err != nil {
		t.Fatalf("racing Stop failed: %v", err)
	}

	close(backend.unblock)

	select {
	case err := <-startErrCh:
		if err == nil {
			t.Error("expected the racing Start to report an error once its PTY was discarded as stale")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the racing Start to return")
	}

	ss, err := s.resolveSession(id)
	if err != nil {
		t.Fatalf("resolveSession failed: %v", err)
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.running {
		t.Error("session must not be running after Stop won the race — the stale start should have been discarded")
	}
	if ss.ptmx != nil || ss.proc != nil {
		t.Error("ptmx/proc must remain nil — the racing start's PTY must never be published after a concurrent Stop")
	}
}
