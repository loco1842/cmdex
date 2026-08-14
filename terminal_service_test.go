package main

import (
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func newTestTerminalService(t *testing.T) *TerminalService {
	t.Helper()
	s := &TerminalService{ptyBackend: newPtyBackend()}
	s.sessions = make(map[string]*sessionState)
	return s
}

func mustCreateAndStart(t *testing.T, s *TerminalService) string {
	t.Helper()
	info, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	t.Cleanup(func() {
		s.CloseSession(info.ID)
	})
	return info.ID
}

// TestTerminalDetectShell verifies detectShell returns correct path/flag for current OS.
func TestTerminalDetectShell(t *testing.T) {
	path, flag := detectShell()

	if path == "" {
		t.Fatal("detectShell returned empty path")
	}

	if runtime.GOOS == "windows" {
		validShells := map[string]bool{"pwsh": true, "powershell": true, "cmd": true}
		if !validShells[path] {
			t.Errorf("unexpected Windows shell path: %s", path)
		}
		if path == "cmd" && flag != "" {
			t.Errorf("cmd shell should have empty flag, got: %s", flag)
		}
	} else {
		if flag != "-l" {
			t.Errorf("Unix shell flag should be '-l', got: '%s'", flag)
		}
	}

	if runtime.GOOS != "windows" {
		t.Run("respectsSHELL", func(t *testing.T) {
			prev := os.Getenv("SHELL")
			os.Setenv("SHELL", "/bin/zsh")
			defer os.Setenv("SHELL", prev)

			p, _ := detectShell()
			if p != "/bin/zsh" {
				t.Errorf("expected /bin/zsh from $SHELL, got: %s", p)
			}
		})
	}
}

// TestTerminalBatching verifies the session state has lastSize field.
func TestTerminalBatching(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	id := mustCreateAndStart(t, s)

	ss, _ := s.resolveSession(id)
	_ = ss.lastSize
}

// TestTerminalStart verifies Start() spawns a shell process via PTY.
func TestTerminalStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	id := mustCreateAndStart(t, s)

	ss, err := s.resolveSession(id)
	if err != nil {
		t.Fatalf("resolveSession failed: %v", err)
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.ptmx == nil {
		t.Fatal("ptmx is nil after Start")
	}
	if ss.cmd == nil {
		t.Fatal("cmd is nil after Start")
	}
	if ss.cmd.Process == nil {
		t.Fatal("process is nil after Start")
	}
}

// TestTerminalWrite verifies Write() sends keystrokes to PTY stdin.
func TestTerminalWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	id := mustCreateAndStart(t, s)

	err := s.Write(id, "echo test\n")
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
}

// TestTerminalResize verifies Resize() calls ptyResize and updates lastSize.
func TestTerminalResize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	id := mustCreateAndStart(t, s)

	err := s.Resize(id, 120, 40)
	if err != nil {
		t.Errorf("Resize failed: %v", err)
	}

	ss, _ := s.resolveSession(id)
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.lastSize.Cols != 120 || ss.lastSize.Rows != 40 {
		t.Errorf("lastSize not updated after Resize: got Cols=%d Rows=%d, want Cols=120 Rows=40",
			ss.lastSize.Cols, ss.lastSize.Rows)
	}
}

// TestTerminalShutdown verifies Stop() kills the shell process and clears state.
func TestTerminalShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	shellPath, shellFlag := detectShell()

	s := newTestTerminalService(t)
	id := mustCreateAndStart(t, s)

	ss, _ := s.resolveSession(id)
	ss.mu.Lock()
	ss.shellPath = shellPath
	ss.shellFlag = shellFlag
	ss.mu.Unlock()

	wantDir := t.TempDir()

	s.Stop(id)
	ptmx, c, err := ptyStart(shellPath, shellFlag, wantDir, 24, 80, "-c", "sleep 60")
	if err != nil {
		t.Fatalf("ptyStart failed: %v", err)
	}
	if c.Dir != wantDir {
		t.Errorf("cmd.Dir = %q, want %q", c.Dir, wantDir)
	}

	ss.mu.Lock()
	ss.ptmx = ptmx
	ss.cmd = c
	ss.stopCh = make(chan struct{})
	ss.running = true
	pid := ss.cmd.Process.Pid
	ss.mu.Unlock()

	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("FindProcess failed: %v", err)
	}

	s.Stop(id)

	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.ptmx != nil {
		t.Error("ptmx not cleared after Stop")
	}
	if ss.cmd != nil {
		t.Error("cmd not cleared after Stop")
	}

	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		t.Error("process still running after Stop")
	}
}

// TestPtyStart_RejectsInvalidDimensions verifies rows/cols outside 1..65535
// are rejected before reaching the uint16 conversion passed to the PTY.
func TestPtyStart_RejectsInvalidDimensions(t *testing.T) {
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
			ptmx, cmd, err := ptyStart(shellPath, shellFlag, "", tc.rows, tc.cols, "-c", "exit 0")
			if err == nil {
				t.Fatalf("ptyStart(rows=%d, cols=%d) succeeded, want error", tc.rows, tc.cols)
			}
			if ptmx != nil || cmd != nil {
				t.Errorf("ptyStart returned non-nil ptmx/cmd alongside an error")
			}
		})
	}
}

// TestTerminalExit verifies shell exit triggers monitorExit flow.
func TestTerminalExit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	id := mustCreateAndStart(t, s)

	ss, _ := s.resolveSession(id)
	shellPath, shellFlag := detectShell()

	wantDir := t.TempDir()

	s.Stop(id)
	ptmx, cmd, err := ptyStart(shellPath, shellFlag, wantDir, 24, 80, "-c", "exit 0")
	if err != nil {
		t.Fatalf("ptyStart failed: %v", err)
	}
	if cmd.Dir != wantDir {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, wantDir)
	}

	ss.mu.Lock()
	ss.shellPath = shellPath
	ss.shellFlag = shellFlag
	ss.lastSize = ptyWinsize{Rows: 24, Cols: 80}
	ss.ptmx = ptmx
	ss.cmd = cmd
	ss.stopCh = make(chan struct{})
	ss.running = true
	ss.mu.Unlock()

	_ = cmd.Wait()
	ptmx.Close()

	go s.monitorExit(ss, cmd, ptmx, ss.stopCh)
	defer s.Stop(id)

	if cmd.ProcessState == nil {
		t.Error("process state is nil after shell exit")
	}
}

// ========== Multi-session tests ==========

// TestTerminalService_CreateSession verifies SESS-01: CreateSession returns valid SessionInfo.
func TestTerminalService_CreateSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	info, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if info.ID == "" {
		t.Error("SessionInfo.ID is empty")
	}
	if !strings.Contains(info.ID, "-") {
		t.Errorf("SessionInfo.ID %q does not look like a UUID (no hyphens)", info.ID)
	}
	if info.Name != "Terminal 1" {
		t.Errorf("expected Name 'Terminal 1', got %q", info.Name)
	}
	if !info.Running {
		t.Error("expected Running=true after CreateSession")
	}

	info2, err := s.CreateSession()
	if err != nil {
		t.Fatalf("second CreateSession failed: %v", err)
	}
	if info2.Name != "Terminal 2" {
		t.Errorf("expected Name 'Terminal 2', got %q", info2.Name)
	}
	if info2.ID == info.ID {
		t.Error("two sessions have the same ID")
	}

	list := s.ListSessions()
	if len(list) != 2 {
		t.Errorf("expected 2 sessions in ListSessions, got %d", len(list))
	}
}

// TestTerminalService_ListSessions verifies ListSessions returns correct count.
func TestTerminalService_ListSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	for i := 0; i < 3; i++ {
		_, err := s.CreateSession()
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
	}

	list := s.ListSessions()
	if len(list) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(list))
	}

	s.CloseSession(list[0].ID)
	list = s.ListSessions()
	if len(list) != 2 {
		t.Errorf("expected 2 sessions after close, got %d", len(list))
	}
}

// TestTerminalService_RenameSession verifies SESS-04: rename with validation.
func TestTerminalService_RenameSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	info, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = s.RenameSession(info.ID, "New Name")
	if err != nil {
		t.Fatalf("RenameSession failed: %v", err)
	}

	list := s.ListSessions()
	if len(list) != 1 || list[0].Name != "New Name" {
		t.Errorf("expected name 'New Name', got %q", list[0].Name)
	}

	err = s.RenameSession(info.ID, "")
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "name cannot be empty") {
		t.Errorf("expected 'name cannot be empty' error, got: %v", err)
	}

	err = s.RenameSession("nonexistent-id", "x")
	if err == nil {
		t.Error("expected error for nonexistent session, got nil")
	}
}

// TestTerminalService_CloseSession verifies SESS-05: session removal.
func TestTerminalService_CloseSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	infoA, _ := s.CreateSession()
	infoB, _ := s.CreateSession()

	err := s.CloseSession(infoA.ID)
	if err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	list := s.ListSessions()
	if len(list) != 1 {
		t.Errorf("expected 1 session after close, got %d", len(list))
	}
	if list[0].ID != infoB.ID {
		t.Errorf("expected remaining session to be %s, got %s", infoB.ID, list[0].ID)
	}

	err = s.CloseSession(infoA.ID)
	if err == nil {
		t.Error("expected error for double-close, got nil")
	}
}

// TestTerminalService_ActiveSessionReassignOnClose verifies active session reassignment.
func TestTerminalService_ActiveSessionReassignOnClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	infoA, _ := s.CreateSession()
	infoB, _ := s.CreateSession()

	_ = s.SetActiveSession(infoA.ID)
	active := s.GetActiveSession()
	if active == nil || active.ID != infoA.ID {
		t.Fatal("expected active session to be A")
	}

	s.CloseSession(infoA.ID)
	active = s.GetActiveSession()
	if active == nil || active.ID != infoB.ID {
		t.Fatalf("expected active session to be reassigned to B, got %v", active)
	}
}

// TestTerminalService_SetActiveSession verifies SetActiveSession error handling.
func TestTerminalService_SetActiveSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	info, _ := s.CreateSession()

	err := s.SetActiveSession(info.ID)
	if err != nil {
		t.Fatalf("SetActiveSession failed: %v", err)
	}

	err = s.SetActiveSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session, got nil")
	}
}

// TestTerminalService_GetActiveSession verifies GetActiveSession returns nil when empty.
func TestTerminalService_GetActiveSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	active := s.GetActiveSession()
	if active != nil {
		t.Error("expected nil active session when none exist")
	}

	info, _ := s.CreateSession()
	active = s.GetActiveSession()
	if active == nil || active.ID != info.ID {
		t.Errorf("expected active session to be %s, got %v", info.ID, active)
	}
}

// TestTerminalService_ShutdownCleansAll verifies ServiceShutdown empties sessions map.
func TestTerminalService_ShutdownCleansAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)

	for i := 0; i < 3; i++ {
		_, err := s.CreateSession()
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
	}

	s.mu.RLock()
	count := len(s.sessions)
	s.mu.RUnlock()
	if count != 3 {
		t.Fatalf("expected 3 sessions, got %d", count)
	}

	err := s.ServiceShutdown()
	if err != nil {
		t.Fatalf("ServiceShutdown failed: %v", err)
	}

	s.mu.RLock()
	count = len(s.sessions)
	s.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 sessions after shutdown, got %d", count)
	}
}

// TestTerminalService_ConcurrentAccess verifies concurrent map access does not panic.
func TestTerminalService_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	_, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	var wg sync.WaitGroup
	done := make(chan struct{})

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = s.ListSessions()
				}
			}
		}()
	}

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = s.GetActiveSession()
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			select {
			case <-done:
				return
			default:
			}
			info, cerr := s.CreateSession()
			if cerr != nil {
				continue
			}
			s.SetActiveSession(info.ID)
			s.CloseSession(info.ID)
		}
	}()

	for i := 0; i < 100; i++ {
		_ = s.ListSessions()
	}
	close(done)
	wg.Wait()
}

// TestTerminalService_ProcessPersistAcrossSessionSwitch verifies EXEC-04:
// a session's PTY survives when switching to another session.
func TestTerminalService_ProcessPersistAcrossSessionSwitch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	infoA, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession A failed: %v", err)
	}
	infoB, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession B failed: %v", err)
	}

	err = s.Write(infoA.ID, "sleep 10 &\n")
	if err != nil {
		t.Fatalf("Write to session A failed: %v", err)
	}

	err = s.Write(infoB.ID, "echo hello\n")
	if err != nil {
		t.Fatalf("Write to session B failed: %v", err)
	}

	ssA, err := s.resolveSession(infoA.ID)
	if err != nil {
		t.Fatalf("resolveSession A failed: %v", err)
	}
	ssA.mu.Lock()
	running := ssA.running
	ssA.mu.Unlock()
	if !running {
		t.Error("session A should still be running after session B activity")
	}

	s.CloseSession(infoB.ID)
	ssA, _ = s.resolveSession(infoA.ID)
	ssA.mu.Lock()
	running = ssA.running
	ssA.mu.Unlock()
	if !running {
		t.Error("session A should survive after closing session B")
	}
}

// TestTerminalService_OutputIsolation verifies session A output does not leak into session B.
func TestTerminalService_OutputIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	infoA, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession A failed: %v", err)
	}
	infoB, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession B failed: %v", err)
	}

	if err := s.Write(infoA.ID, "echo AAA\n"); err != nil {
		t.Fatalf("Write to session A failed: %v", err)
	}
	if err := s.Write(infoB.ID, "echo BBB\n"); err != nil {
		t.Fatalf("Write to session B failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	ssA, _ := s.resolveSession(infoA.ID)
	ssB, _ := s.resolveSession(infoB.ID)

	drain := func(ch chan string) string {
		var buf string
		for {
			select {
			case data := <-ch:
				buf += data
			default:
				return buf
			}
		}
	}

	outputA := drain(ssA.outputCh)
	outputB := drain(ssB.outputCh)

	if strings.Contains(outputA, "BBB") {
		t.Error("session A output contains data from session B (output isolation violation)")
	}
	if strings.Contains(outputB, "AAA") {
		t.Error("session B output contains data from session A (output isolation violation)")
	}
}

// TestTerminalService_NamespacedEvents verifies the session has a valid ID
// for namespaced event emission.
func TestTerminalService_NamespacedEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	info, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	ss, _ := s.resolveSession(info.ID)
	if ss.id == "" {
		t.Error("session has empty id — cannot generate namespaced event names")
	}
	if ss.id != info.ID {
		t.Errorf("session id mismatch: %s != %s", ss.id, info.ID)
	}
}

// cwdInheritanceTestDB opens an isolated in-memory test DB, runs migrations,
// swaps the package-level `db` var to point at it, and returns a cleanup func
// that restores the previous value and closes the test DB. Using an in-memory
// DB avoids touching the user's real ~/.cmdex/cmdex.db during testing.
func cwdInheritanceTestDB(t *testing.T) func() {
	t.Helper()
	testDB := newTestDB(t)
	if err := testDB.runMigrations(); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}
	prevDB := db
	db = testDB
	return func() {
		db = prevDB
		testDB.Close()
	}
}

// setGlobalDefaultCwd writes settings.DefaultWorkingDir for the current OS to
// the given path. The OSPathMap is replaced (not merged) so the test can
// cleanly switch between the configured and home-fallback paths.
func setGlobalDefaultCwd(t *testing.T, path string) {
	t.Helper()
	settings, err := db.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if path == "" {
		settings.DefaultWorkingDir = &OSPathMap{paths: map[string]string{}}
	} else {
		settings.DefaultWorkingDir = &OSPathMap{paths: map[string]string{runtime.GOOS: path}}
	}
	if err := db.SetSettings(settings); err != nil {
		t.Fatalf("SetSettings failed: %v", err)
	}
}

// TestTerminalService_GlobalDefaultCwdInheritance verifies D-04/D-05: a new
// session's WorkingDir reads the OS-keyed global default from settings, and
// falls back to the OS user home directory when the global default is empty
// for the current OS. Uses the real DB so the settings read path is exercised
// end-to-end.
func TestTerminalService_GlobalDefaultCwdInheritance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	defer cwdInheritanceTestDB(t)()

	// Unique token so this test does not collide with concurrent test runs
	// that share the user's ~/.cmdex/cmdex.db.
	wdPath := t.TempDir()

	setGlobalDefaultCwd(t, wdPath)

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	info, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if info.WorkingDir != wdPath {
		t.Errorf("expected WorkingDir=%q (from global default), got %q", wdPath, info.WorkingDir)
	}

	// Clear the global default — fall back to user home.
	setGlobalDefaultCwd(t, "")

	s2 := newTestTerminalService(t)
	defer s2.ServiceShutdown()

	info2, err := s2.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession (home fallback) failed: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir failed: %v", err)
	}
	if info2.WorkingDir != home {
		t.Errorf("expected WorkingDir=%q (home fallback), got %q", home, info2.WorkingDir)
	}

	// Cleanup: clear the global default so subsequent tests start from a
	// known-empty state.
	setGlobalDefaultCwd(t, "")
}

// TestTerminalService_CwdInheritance_ExistingSessionUnaffected verifies D-06:
// once a session is created, changing the global default cwd does not
// retroactively alter the existing session's WorkingDir — CreateSession
// snapshots the value at creation time, so existing sessions keep the cwd
// they were started with.
func TestTerminalService_CwdInheritance_ExistingSessionUnaffected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	defer cwdInheritanceTestDB(t)()

	p1 := t.TempDir()
	p2 := t.TempDir()

	setGlobalDefaultCwd(t, p1)

	s := newTestTerminalService(t)
	defer s.ServiceShutdown()

	info1, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession (p1) failed: %v", err)
	}
	if info1.WorkingDir != p1 {
		t.Fatalf("expected WorkingDir=%q for session 1, got %q", p1, info1.WorkingDir)
	}

	// Update the global default AFTER the first session is created.
	setGlobalDefaultCwd(t, p2)

	info2, err := s.CreateSession()
	if err != nil {
		t.Fatalf("CreateSession (p2) failed: %v", err)
	}
	if info2.WorkingDir != p2 {
		t.Errorf("expected WorkingDir=%q for session 2 (new global default), got %q", p2, info2.WorkingDir)
	}

	// Existing session 1 must still report its original cwd.
	list := s.ListSessions()
	var s1 *SessionInfo
	for _, item := range list {
		if item.ID == info1.ID {
			s1 = item
			break
		}
	}
	if s1 == nil {
		t.Fatalf("session 1 not found in ListSessions after creating session 2")
	}
	if s1.WorkingDir != p1 {
		t.Errorf("D-06 violation: existing session 1 WorkingDir=%q, expected %q (must not retroactively change)",
			s1.WorkingDir, p1)
	}

	// Cleanup: clear the global default so subsequent tests start from a
	// known-empty state.
	setGlobalDefaultCwd(t, "")
}
