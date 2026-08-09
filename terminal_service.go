package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type ptyWinsize struct {
	Rows uint16
	Cols uint16
}

// MaxSessions is the maximum number of concurrent terminal sessions. Beyond
// this limit, CreateSession returns an error to protect against unbounded
// resource use. The number 10 is chosen as a reasonable cap that exceeds
// normal user workflows (typical use is 1-3 sessions) while preventing
// accidental resource exhaustion.
const MaxSessions = 10

const (
	// defaultTerminalCols/Rows is the initial PTY size (standard VT100 default),
	// used both as the CreateSession default and as the startSessionLocked
	// clamp target when the caller passes an out-of-range size.
	defaultTerminalCols = 80
	defaultTerminalRows = 24

	// minTerminalCols/Rows are the smallest PTY dimensions startSessionLocked
	// accepts before falling back to the default.
	minTerminalCols = 10
	minTerminalRows = 3

	// readBufferSize is the PTY read chunk size in readLoop.
	readBufferSize = 8192

	// outputChannelBufferSize is the capacity of a session's output channel
	// before enqueueOutput starts dropping data.
	outputChannelBufferSize = 512

	// emitterFlushInterval is how often the batching emitter flushes buffered
	// output to the frontend even if outputFlushThresholdBytes isn't reached.
	emitterFlushInterval = 8 * time.Millisecond

	// outputFlushThresholdBytes triggers an early flush once the emitter's
	// buffer grows this large, instead of waiting for the next tick.
	outputFlushThresholdBytes = 32768

	// autoRestartDelay is the pause before restarting a shell that exited
	// unintentionally (crash), to avoid a tight restart loop.
	autoRestartDelay = 100 * time.Millisecond
)

// SessionInfo is the public metadata for a terminal session, sent to the frontend.
type SessionInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	ShellPath  string `json:"shellPath"`
	WorkingDir string `json:"workingDir"`
}

// sessionState is the internal per-session state: PTY, process, goroutine tracking.
type sessionState struct {
	id         string
	name       string
	workingDir string
	createdAt  time.Time

	mu              sync.Mutex
	ptmx            ptyHandle
	cmd             *exec.Cmd
	shellPath       string
	shellFlag       string
	lastSize        ptyWinsize
	stopCh          chan struct{}
	running         bool
	starting        bool
	intentionalStop bool
	closed          bool

	readerWg     sync.WaitGroup
	outputCh     chan string
	outputSeq    uint64
	emitterWg    sync.WaitGroup
	droppedCount atomic.Uint64
}

// TerminalService manages multiple PTY-backed shell sessions.
type TerminalService struct {
	mu              sync.RWMutex
	sessions        map[string]*sessionState
	activeSessionID string
	sessionCounter  int
	ptyBackend      ptyBackend
}

// info returns the public SessionInfo for this session.
func (ss *sessionState) info() *SessionInfo {
	return &SessionInfo{
		ID:         ss.id,
		Name:       ss.name,
		Running:    ss.running,
		ShellPath:  ss.shellPath,
		WorkingDir: ss.workingDir,
	}
}

// getWorkingDir returns the working directory for new sessions. It first
// consults the OS-keyed global default cwd from settings (D-04, D-05); if
// that is empty or settings is unavailable, it falls back to the OS user
// home directory. Existing sessions are not retroactively affected by global
// default changes — D-06 is enforced by CreateSession snapshotting the
// returned value at session creation time.
func getWorkingDir() string {
	// Step 1: read the OS-keyed global default cwd from settings.
	if db != nil {
		settings, err := db.GetSettings()
		if err != nil {
			fmt.Printf("getWorkingDir: GetSettings failed: %v\n", err)
		} else if settings.DefaultWorkingDir != nil {
			if path := settings.DefaultWorkingDir.GetCurrentOS(); path != "" {
				return path
			}
		}
	}
	// Step 2: fall back to the OS user home directory.
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// resolveSession resolves a sessionId to a *sessionState pointer.
// If sessionId is "", falls back to the active session.
// The caller must NOT hold s.mu while operating on the returned sessionState.
func (s *TerminalService) resolveSession(sessionId string) (*sessionState, error) {
	if sessionId == "" {
		s.mu.RLock()
		sessionId = s.activeSessionID
		s.mu.RUnlock()
		if sessionId == "" {
			return nil, errors.New("no active session")
		}
	}
	s.mu.RLock()
	ss, ok := s.sessions[sessionId]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionId)
	}
	return ss, nil
}

func detectShell() (string, string) {
	if runtime.GOOS == "windows" {
		for _, shell := range []string{"pwsh", "powershell"} {
			if lp, err := exec.LookPath(shell); err == nil {
				return lp, "-NoLogo"
			}
		}
		return "cmd", ""
	}

	path := os.Getenv("SHELL")
	if path == "" {
		path = "/bin/sh"
	}
	return path, "-l"
}

// ========== Service Lifecycle ==========

func (s *TerminalService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	terminalSvc = s

	s.sessions = make(map[string]*sessionState)
	s.ptyBackend = newPtyBackend()

	_, err := s.CreateSession()
	if err != nil {
		fmt.Printf("TerminalService: CreateSession failed (graceful degradation): %v\n", err)
	}
	return nil
}

func (s *TerminalService) ServiceShutdown() error {
	s.mu.Lock()
	sessionIDs := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		sessionIDs = append(sessionIDs, id)
	}
	s.mu.Unlock()

	for _, id := range sessionIDs {
		if err := s.CloseSession(id); err != nil {
			fmt.Printf("ServiceShutdown: CloseSession(%s) failed: %v\n", id, err)
		}
	}
	return nil
}

// ========== Session CRUD ==========

// CreateSession creates a new terminal session with a UUID v4 ID and default name "Terminal N".
func (s *TerminalService) CreateSession() (*SessionInfo, error) {
	s.mu.Lock()
	if n := len(s.sessions); n >= MaxSessions {
		s.mu.Unlock()
		return nil, fmt.Errorf("CreateSession: max sessions reached (%d)", MaxSessions)
	}

	s.sessionCounter++
	name := fmt.Sprintf("Terminal %d", s.sessionCounter)
	id := uuid.New().String()

	ss := &sessionState{
		id:         id,
		name:       name,
		workingDir: getWorkingDir(),
		createdAt:  time.Now(),
		lastSize:   ptyWinsize{Rows: defaultTerminalRows, Cols: defaultTerminalCols},
	}

	s.sessions[id] = ss
	if s.activeSessionID == "" {
		s.activeSessionID = id
	}
	s.mu.Unlock()

	// Start emitter goroutine for output batching.
	ss.startEmitter()

	// Start the PTY shell. startSessionLocked requires ss.mu to be held and
	// returns with ss.mu held. s.mu must NOT be held (prevents deadlock with
	// unlock-before-blocking pattern).
	ss.mu.Lock()
	if err := s.startSessionLocked(ss, defaultTerminalCols, defaultTerminalRows); err != nil {
		ss.mu.Unlock()
		ss.stopEmitter()
		s.mu.Lock()
		delete(s.sessions, id)
		if s.activeSessionID == id {
			s.activeSessionID = ""
		}
		s.mu.Unlock()
		return nil, err
	}
	ss.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	return ss.info(), nil
}

// ListSessions returns SessionInfo for all sessions in the manager.
func (s *TerminalService) ListSessions() []*SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*SessionInfo, 0, len(s.sessions))
	for _, ss := range s.sessions {
		result = append(result, ss.info())
	}
	return result
}

// CloseSession removes a session from the manager. If the closed session was the
// active one, another session is assigned as active (or cleared if none remain).
func (s *TerminalService) CloseSession(id string) error {
	s.mu.Lock()
	ss, ok := s.sessions[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("CloseSession: session not found: %s", id)
	}

	delete(s.sessions, id)

	if s.activeSessionID == id {
		s.activeSessionID = ""
		for otherID := range s.sessions {
			s.activeSessionID = otherID
			break
		}
	}

	// Snapshot PTY resources under lock before releasing.
	ss.mu.Lock()
	ss.stopSessionLocked()
	oldPtmx := ss.ptmx
	oldCmd := ss.cmd
	ss.ptmx = nil
	ss.cmd = nil
	ss.running = false
	ss.closed = true
	ss.mu.Unlock()
	s.mu.Unlock()

	// Kill PTY and wait for goroutines (no locks held — prevents deadlock).
	if oldPtmx != nil {
		_ = oldPtmx.Close()
	}
	if oldCmd != nil && oldCmd.ProcessState == nil {
		_ = s.ptyBackend.Kill(oldCmd)
	}
	ss.readerWg.Wait()
	ss.stopEmitter()

	return nil
}

// RenameSession updates the name of a session. Returns an error if name is empty
// or the session does not exist.
func (s *TerminalService) RenameSession(id string, name string) error {
	if name == "" {
		return errors.New("RenameSession: name cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ss, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("RenameSession: session not found: %s", id)
	}

	ss.name = name
	return nil
}

// SetActiveSession sets the active session by ID.
func (s *TerminalService) SetActiveSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("SetActiveSession: session not found: %s", id)
	}

	s.activeSessionID = id
	return nil
}

// GetActiveSession returns the SessionInfo for the currently active session,
// or nil if no active session exists.
func (s *TerminalService) GetActiveSession() *SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.activeSessionID == "" {
		return nil
	}

	ss, ok := s.sessions[s.activeSessionID]
	if !ok {
		return nil
	}
	return ss.info()
}

// ========== Per-Session PTY Lifecycle ==========

// startSessionLocked starts a PTY shell for the given session.
// ss.mu MUST be held by the caller. TerminalService.mu is NOT held.
func (s *TerminalService) startSessionLocked(ss *sessionState, cols, rows int) error {
	if ss.starting {
		return nil
	}
	ss.starting = true

	// Clamp minimum dimensions.
	if cols < minTerminalCols {
		cols = defaultTerminalCols
	}
	if rows < minTerminalRows {
		rows = defaultTerminalRows
	}

	// Clean up any previous PTY.
	ss.stopSessionLocked()

	oldPtmx := ss.ptmx
	oldCmd := ss.cmd
	ss.ptmx = nil
	ss.cmd = nil
	ss.running = false
	ss.intentionalStop = false

	shellPath, shellFlag := detectShell()

	// CRITICAL: unlock before blocking operations to prevent deadlock.
	ss.mu.Unlock()

	if oldPtmx != nil {
		_ = oldPtmx.Close()
	}
	if oldCmd != nil && oldCmd.ProcessState == nil {
		_ = s.ptyBackend.Kill(oldCmd)
	}
	ss.readerWg.Wait()

	handle, cmd, err := s.ptyBackend.Start(shellPath, shellFlag, rows, cols)

	ss.mu.Lock()
	ss.starting = false

	if err != nil {
		return err
	}

	ss.shellPath = shellPath
	ss.shellFlag = shellFlag
	ss.lastSize = ptyWinsize{Rows: uint16(rows), Cols: uint16(cols)}
	ss.ptmx = handle
	ss.cmd = cmd
	ss.stopCh = make(chan struct{})
	ss.running = true

	stopCh := ss.stopCh
	ss.readerWg.Add(1)
	go ss.readLoop(handle, stopCh)
	go s.monitorExit(ss, cmd, handle, stopCh)

	return nil
}

// stopSessionLocked signals the session's goroutines to stop by closing stopCh.
// ss.mu MUST be held by the caller.
func (ss *sessionState) stopSessionLocked() {
	if ss.stopCh != nil {
		close(ss.stopCh)
		ss.stopCh = nil
	}
}

// readLoop reads PTY output, handles UTF-8 boundaries, and dispatches to enqueueOutput.
func (ss *sessionState) readLoop(ptmx ptyHandle, stopCh chan struct{}) {
	defer ss.readerWg.Done()

	buf := make([]byte, readBufferSize)
	var leftover []byte

	for {
		select {
		case <-stopCh:
			if len(leftover) > 0 {
				ss.enqueueOutput(string(leftover))
			}
			return
		default:
		}

		n, err := ptmx.Read(buf)
		if err != nil {
			if len(leftover) > 0 {
				ss.enqueueOutput(string(leftover))
			}
			return
		}

		data := make([]byte, n+len(leftover))
		copy(data, leftover)
		copy(data[len(leftover):], buf[:n])
		leftover = nil

		// Find last valid UTF-8 boundary to avoid splitting multi-byte sequences.
		validEnd := len(data)
		for validEnd > 0 {
			if data[validEnd-1]&0x80 == 0 {
				break
			}
			if validEnd >= 2 && data[validEnd-2]&0xE0 == 0xC0 {
				break
			}
			if validEnd >= 3 && data[validEnd-3]&0xF0 == 0xE0 {
				break
			}
			if validEnd >= 4 && data[validEnd-4]&0xF8 == 0xF0 {
				break
			}
			validEnd--
		}

		if validEnd < len(data) {
			leftover = make([]byte, len(data)-validEnd)
			copy(leftover, data[validEnd:])
			data = data[:validEnd]
		}

		if len(data) > 0 {
			ss.enqueueOutput(string(data))
		}
	}
}

// startEmitter creates the output channel and starts the batching emitter goroutine.
func (ss *sessionState) startEmitter() {
	ss.outputCh = make(chan string, outputChannelBufferSize)

	ss.emitterWg.Go(func() {
		var buf bytes.Buffer
		ticker := time.NewTicker(emitterFlushInterval)
		defer ticker.Stop()

		flush := func() {
			if buf.Len() == 0 {
				return
			}
			seq := atomic.AddUint64(&ss.outputSeq, 1)
			if wailsApp != nil {
				wailsApp.Event.Emit("pty-output:"+ss.id, map[string]any{
					"data": buf.String(),
					"seq":  seq,
				})
			}
			buf.Reset()
		}

		for {
			select {
			case data, ok := <-ss.outputCh:
				if !ok {
					flush()
					return
				}
				buf.WriteString(data)
				if buf.Len() >= outputFlushThresholdBytes {
					flush()
				}
			case <-ticker.C:
				flush()
			}
		}
	})
}

// stopEmitter closes the output channel and waits for the emitter goroutine to finish.
func (ss *sessionState) stopEmitter() {
	if ss.outputCh != nil {
		close(ss.outputCh)
	}
	ss.emitterWg.Wait()
	ss.outputCh = nil
}

// enqueueOutput sends data to the output channel non-blocking.
func (ss *sessionState) enqueueOutput(data string) {
	select {
	case ss.outputCh <- data:
	default:
		count := ss.droppedCount.Add(1)
		if count%100 == 1 {
			fmt.Printf("pty output queue full (session %s), dropped %d times\n", ss.id, count)
		}
	}
}

// monitorExit watches for shell exit and handles event emission and auto-restart.
func (s *TerminalService) monitorExit(ss *sessionState, cmd *exec.Cmd, ptmx ptyHandle, stopCh chan struct{}) {
	err := cmd.Wait()

	select {
	case <-stopCh:
		return
	default:
	}

	exitCode := 0
	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	ss.mu.Lock()
	intentional := ss.intentionalStop || exitCode == 0
	cols := int(ss.lastSize.Cols)
	rows := int(ss.lastSize.Rows)
	ss.mu.Unlock()

	if wailsApp != nil {
		wailsApp.Event.Emit("pty-exit:"+ss.id, map[string]any{
			"exitCode":       exitCode,
			"wasIntentional": intentional,
		})
	}

	if intentional {
		ss.mu.Lock()
		ss.running = false
		ss.mu.Unlock()
		return
	}

	// Unintentional exit (crash) — auto-restart after brief delay.
	time.Sleep(autoRestartDelay)
	if err := s.Start(ss.id, cols, rows); err != nil {
		fmt.Printf("monitorExit: auto-restart failed for session %s: %v\n", ss.id, err)
	}
}

// ========== Session Dispatch Methods ==========

// Write sends input data to the specified session's PTY.
func (s *TerminalService) Write(sessionId string, data string) error {
	ss, err := s.resolveSession(sessionId)
	if err != nil {
		return err
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.closed {
		return fmt.Errorf("session closed: %s", sessionId)
	}

	if !ss.running {
		if err := s.startSessionLocked(ss, int(ss.lastSize.Cols), int(ss.lastSize.Rows)); err != nil {
			return err
		}
	}

	if ss.ptmx == nil {
		return errors.New("terminal not started")
	}

	b := []byte(data)
	for len(b) > 0 {
		n, err := ss.ptmx.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

// Resize changes the terminal dimensions for the specified session.
func (s *TerminalService) Resize(sessionId string, cols, rows int) error {
	if cols < 1 || cols > 65535 || rows < 1 || rows > 65535 {
		return fmt.Errorf("Resize: invalid dimensions cols=%d rows=%d (must be 1..65535)", cols, rows)
	}

	ss, err := s.resolveSession(sessionId)
	if err != nil {
		return err
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.closed {
		return fmt.Errorf("session closed: %s", sessionId)
	}

	if ss.ptmx == nil {
		return errors.New("terminal not started")
	}

	ss.lastSize = ptyWinsize{Cols: uint16(cols), Rows: uint16(rows)}
	return s.ptyBackend.Resize(ss.ptmx, cols, rows)
}

// Clear sends the ANSI clear sequence and emits a namespaced clear event.
func (s *TerminalService) Clear(sessionId string) error {
	ss, err := s.resolveSession(sessionId)
	if err != nil {
		return err
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.closed {
		return fmt.Errorf("session closed: %s", sessionId)
	}

	if ss.ptmx == nil {
		return errors.New("terminal not started")
	}

	clearSeq := "\x1b[H\x1b[2J\x1b[3J"
	b := []byte(clearSeq)
	for len(b) > 0 {
		n, err := ss.ptmx.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}

	if wailsApp != nil {
		wailsApp.Event.Emit("pty-cleared:"+ss.id, nil)
	}

	return nil
}

// Start starts the PTY shell for the specified session.
func (s *TerminalService) Start(sessionId string, cols, rows int) error {
	ss, err := s.resolveSession(sessionId)
	if err != nil {
		return err
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.closed {
		return fmt.Errorf("session closed: %s", sessionId)
	}

	return s.startSessionLocked(ss, cols, rows)
}

// Stop stops the PTY shell for the specified session.
func (s *TerminalService) Stop(sessionId string) error {
	ss, err := s.resolveSession(sessionId)
	if err != nil {
		return err
	}

	ss.mu.Lock()
	if ss.closed {
		ss.mu.Unlock()
		return fmt.Errorf("session closed: %s", sessionId)
	}
	ss.intentionalStop = true
	ss.stopSessionLocked()

	oldPtmx := ss.ptmx
	oldCmd := ss.cmd
	ss.ptmx = nil
	ss.cmd = nil
	ss.running = false
	ss.mu.Unlock()

	if oldPtmx != nil {
		_ = oldPtmx.Close()
	}
	if oldCmd != nil && oldCmd.ProcessState == nil {
		_ = s.ptyBackend.Kill(oldCmd)
	}
	ss.readerWg.Wait()

	return nil
}
