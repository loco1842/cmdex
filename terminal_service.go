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

	mu        sync.Mutex
	ptmx      ptyHandle
	proc      ptyProcess
	shellPath string
	shellFlag string
	lastSize  ptyWinsize
	// oscNonce is the current shell process's OSC-marker nonce (empty if
	// this session isn't running with shell integration active). Set once
	// under ss.mu by startSessionLocked before readLoop's goroutine starts,
	// and read only from that same goroutine (via captureScan) thereafter,
	// so no further locking is needed — see oscNonceFileEnvVar in
	// shell_integration.go and stripNonce in terminal_capture.go.
	oscNonce        string
	stopCh          chan struct{}
	running         bool
	starting        bool
	intentionalStop bool
	closed          bool
	// generation counts how many times Stop/CloseSession have invalidated
	// this session's current PTY lifecycle. startSessionLocked snapshots it
	// before unlocking ss.mu for the blocking shell-integration setup and
	// ptyBackend.Start calls, then compares again once relocked: a bump in
	// between means Stop or CloseSession ran while this start was in
	// flight, and the PTY it just spawned must be torn down immediately
	// rather than resurrecting a session the caller already asked to stop
	// (see startSessionLocked's staleness check).
	generation uint64

	readerWg     sync.WaitGroup
	outputCh     chan string
	outputSeq    uint64
	emitterWg    sync.WaitGroup
	droppedCount atomic.Uint64

	// Capture state for OSC 133 shell-integration markers — see
	// terminal_capture.go. Guarded by capMu rather than mu: captureScan runs
	// on the readLoop goroutine without mu held, and GetLastOutput must not
	// block on session lifecycle operations.
	//
	// capCols mirrors lastSize.Cols specifically for captureScan's use: it
	// needs the current terminal width (to recognize ConPTY's line-wrap
	// injection artifacts — see removeWrapArtifacts in ansi.go) but runs on
	// the readLoop goroutine without mu held, while Resize/startSessionLocked
	// update lastSize.Cols under mu. A plain field read here would race;
	// this must NOT instead be read from captureScan by taking mu, since
	// Clear/resetCapture already lock mu THEN capMu — captureScan already
	// holds capMu at that point, and acquiring mu there too would invert
	// that ordering and risk deadlock. An independent atomic sidesteps both
	// problems.
	capCols atomic.Uint32
	capMu   sync.Mutex
	capBuf  bytes.Buffer
	// capCaptureCols snapshots capCols when a "C" marker opens a capture, so
	// the "D" marker's stripANSI call uses the width active while THIS
	// command's output was actually emitted rather than whatever capCols has
	// drifted to from a Resize that happened mid-command (see captureScan in
	// terminal_capture.go). Guarded by capMu like the rest of the capture
	// state, since it's only ever touched from within captureScan/GetLastOutput.
	capCaptureCols int
	capCarry       []byte
	capturing      bool
	capTruncated   bool
	lastOutput     string
	lastExitCode   int
	lastTruncated  bool
	lastValid      bool
}

// TerminalService manages multiple PTY-backed shell sessions.
type TerminalService struct {
	mu              sync.RWMutex
	sessions        map[string]*sessionState
	activeSessionID string
	sessionCounter  int
	ptyBackend      ptyBackend

	// shellIntegrationDir is where the embedded OSC 133 integration scripts
	// (shell_integration.go) were materialized at ServiceStartup, or "" if
	// materialization failed — in which case startSessionLocked skips shell
	// integration entirely and every session behaves exactly as it did
	// before shell integration existed.
	shellIntegrationDir string
}

// info returns the public SessionInfo for this session. Locks ss.mu: Running
// and ShellPath are written under ss.mu by startSessionLocked, and all three
// callers (CreateSession, ListSessions, GetActiveSession) hold only s.mu
// (the service-level lock) when calling this, never ss.mu.
func (ss *sessionState) info() *SessionInfo {
	ss.mu.Lock()
	defer ss.mu.Unlock()
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

	if dir, err := setupShellIntegrationDir(); err != nil {
		// Non-fatal: leaves shellIntegrationDir at its zero value "", so
		// startSessionLocked just skips shell integration for every
		// session — GetLastOutput reports Available=false and the frontend
		// falls back to scraping the xterm buffer, same as before this
		// feature existed.
		fmt.Printf("TerminalService: shell integration setup failed (graceful degradation): %v\n", err)
	} else {
		s.shellIntegrationDir = dir
	}

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
	// Invalidates any startSessionLocked call currently unlocked and
	// mid-flight (shell-integration setup, ptyBackend.Start) for this
	// session, so it discards whatever PTY it spawns instead of publishing
	// it into a session we're about to remove — see the generation field's
	// comment and startSessionLocked's staleness check.
	ss.generation++
	oldPtmx := ss.ptmx
	oldProc := ss.proc
	ss.ptmx = nil
	ss.proc = nil
	ss.running = false
	ss.closed = true
	ss.mu.Unlock()
	s.mu.Unlock()

	// Kill PTY and wait for goroutines (no locks held — prevents deadlock).
	s.releaseOldProcess(ss, oldPtmx, oldProc)
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
	// Snapshotted before unlocking below for the blocking shell-integration
	// setup and ptyBackend.Start calls — see the staleness check once
	// relocked, and the generation field's own comment for why this exists.
	startGen := ss.generation

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
	oldProc := ss.proc
	ss.ptmx = nil
	ss.proc = nil
	ss.running = false
	ss.intentionalStop = false

	shellPath, shellFlag := detectShell()

	// CRITICAL: unlock before blocking operations to prevent deadlock.
	ss.mu.Unlock()

	// Activate OSC 133 shell integration when possible: integrationFor may
	// override shellFlag entirely (bash drops its usual -l — see
	// shell_integration.go) as well as add args/env, so launchFlag/opts,
	// not shellFlag, are what actually get passed to ptyBackend.Start. This
	// runs unlocked: shellIntegrationEnabled() takes a blocking DB
	// round-trip, and nothing here touches session state that needs ss.mu.
	launchFlag := shellFlag
	var opts shellLaunchOpts
	integrated := false
	var nonce string
	var nonceFileCleanup func()
	if s.shellIntegrationDir != "" && shellIntegrationEnabled() {
		if n, nErr := generateOSCNonce(); nErr == nil {
			if nonceFile, cleanup, fErr := writeNonceFile(n); fErr == nil {
				if flag, sOpts, ok := integrationFor(shellPath, shellFlag, s.shellIntegrationDir, nonceFile); ok {
					launchFlag = flag
					opts = sOpts
					integrated = true
					nonce = n
					nonceFileCleanup = cleanup
				} else {
					cleanup()
				}
			} else {
				fmt.Println("Shell integration disabled for this session, could not write OSC nonce file:", fErr)
			}
		} else {
			fmt.Println("Shell integration disabled for this session, could not generate OSC nonce:", nErr)
		}
	}

	s.releaseOldProcess(ss, oldPtmx, oldProc)

	// Only safe to reset capture state once the previous session's readLoop
	// goroutine has actually exited (releaseOldProcess just joined it above)
	// — otherwise a straggling captureScan from the dying goroutine's final
	// read (or its stopCh leftover flush) can repopulate stale capture state
	// for the new session right after it was cleared.
	ss.resetCapture()

	handle, proc, err := s.ptyBackend.Start(shellPath, launchFlag, ss.workingDir, rows, cols, opts)

	ss.mu.Lock()
	ss.starting = false

	// Stop or CloseSession bumped ss.generation while we were unlocked
	// above — the caller already asked to stop this session, so the PTY
	// ptyBackend.Start just spawned (if it succeeded at all) must be torn
	// down immediately rather than being published as ss.ptmx/ss.proc:
	// without this check, it would silently resurrect a session the app no
	// longer tracks (CloseSession may have already removed it from
	// s.sessions), leaking a real shell process and its would-be
	// readLoop/monitorExit goroutines with no way to ever stop them again.
	if stale := ss.generation != startGen; stale || err != nil {
		ss.mu.Unlock()
		if nonceFileCleanup != nil {
			nonceFileCleanup()
		}
		if err == nil {
			_ = handle.Close()
			if !proc.Exited() {
				_ = s.ptyBackend.Kill(proc)
			}
		}
		ss.mu.Lock()
		if stale {
			return errors.New("session stopped while starting")
		}
		return err
	}

	// The integration script deletes the nonce file itself within
	// milliseconds of shell startup (see oscNonceFileEnvVar) — this is only
	// the fail-safe path, for the shell crashing before running its
	// startup files or some other abnormal case.
	if nonceFileCleanup != nil {
		time.AfterFunc(nonceFileCleanupGrace, nonceFileCleanup)
	}

	ss.shellPath = shellPath
	ss.shellFlag = shellFlag
	ss.oscNonce = nonce
	ss.lastSize = ptyWinsize{Rows: uint16(rows), Cols: uint16(cols)}
	ss.capCols.Store(uint32(cols))
	ss.ptmx = handle
	ss.proc = proc
	ss.stopCh = make(chan struct{})
	ss.running = true

	stopCh := ss.stopCh
	ss.readerWg.Add(1)
	go ss.readLoop(handle, stopCh, integrated)
	go s.monitorExit(ss, proc, stopCh)

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

// releaseOldProcess closes oldPtmx, kills oldProc if it hasn't already exited,
// and waits for ss's reader goroutine to finish. Shared by CloseSession, Stop,
// and startSessionLocked, which all snapshot ss.ptmx/ss.proc under ss.mu, clear
// them, and then must run this sequence with ss.mu NOT held — Close/Kill can
// block, and readLoop needs to run unimpeded to observe stopCh and exit.
func (s *TerminalService) releaseOldProcess(ss *sessionState, oldPtmx ptyHandle, oldProc ptyProcess) {
	if oldPtmx != nil {
		_ = oldPtmx.Close()
	}
	if oldProc != nil && !oldProc.Exited() {
		_ = s.ptyBackend.Kill(oldProc)
	}
	ss.readerWg.Wait()
}

// readLoop reads PTY output, handles UTF-8 boundaries, and dispatches to
// enqueueOutput. captureActive is a per-invocation snapshot (not read live
// off ss) of whether this session's shell was actually launched with OSC 133
// integration — sessions on an unrecognized shell, or with the setting
// disabled, never emit markers, so skipping captureScan entirely avoids
// taking capMu and scanning every chunk for ESC bytes for no benefit.
func (ss *sessionState) readLoop(ptmx ptyHandle, stopCh chan struct{}, captureActive bool) {
	defer ss.readerWg.Done()

	buf := make([]byte, readBufferSize)
	var leftover []byte

	for {
		select {
		case <-stopCh:
			if len(leftover) > 0 {
				if captureActive {
					ss.captureScan(leftover)
				}
				ss.enqueueOutput(string(leftover))
			}
			return
		default:
		}

		n, err := ptmx.Read(buf)
		if err != nil {
			if len(leftover) > 0 {
				if captureActive {
					ss.captureScan(leftover)
				}
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
			if captureActive {
				ss.captureScan(data)
			}
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
func (s *TerminalService) monitorExit(ss *sessionState, proc ptyProcess, stopCh chan struct{}) {
	exitCode, _ := proc.Wait()

	select {
	case <-stopCh:
		return
	default:
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
	ss.capCols.Store(uint32(cols))
	return s.ptyBackend.Resize(ss.ptmx, cols, rows)
}

// clearKeyFor returns the bytes Clear should write to the shell's stdin to
// make IT perform its own screen clear, dispatching on shellPath's basename
// the same way integrationFor does.
//
// Ctrl+L (0x0C) is the portable choice: it's the default "clear screen,
// redraw the current input line" key binding in PSReadLine (pwsh/powershell)
// and in bash/zsh/fish's own readline/ZLE/line editor — sent as a single
// control byte, exactly as if the user had pressed it, so it does not
// disturb whatever the user has already typed on the current line the way
// appending a literal "clear\r" command would (that would submit the
// in-progress line with "clear" tacked onto the end). cmd.exe has no such
// binding and no interactive line editor to speak of, so it gets "cls\r"
// instead — the same as a user typing the command and pressing Enter.
func clearKeyFor(shellPath string) []byte {
	if shellBaseName(shellPath) == "cmd" {
		return []byte("cls\r")
	}
	return []byte{0x0C}
}

// Clear makes the shell clear its own screen, the same way it would if the
// user pressed Ctrl+L (or typed cls/clear and Enter) themselves — see
// clearKeyFor. This is why letting the shell do it (rather than writing an
// ANSI clear sequence straight into ss.ptmx, which a previous version did)
// matters: PSReadLine (and bash/zsh's own line editor) track the real
// console's cursor position via absolute Win32/terminfo addressing, which a
// purely client-side xterm.js clear can never see. If only the frontend's
// buffer is wiped, the shell's next prompt redraw still targets whatever
// absolute row it last believed the cursor was at, and xterm.js — now
// otherwise empty — pads up to that row with blank lines to honor the
// positioning request, which is exactly the "empty line from before"
// glitch this fixes. Driving the shell's own clear keeps its internal
// cursor tracking and the frontend's rendered buffer in sync, since both
// are driven by the same clear-and-redraw the shell performs. The
// pty-cleared event still fires for an immediate, optimistic frontend wipe;
// the shell's own (slightly delayed) redraw arriving over the normal
// output stream is what keeps the two in sync afterward.
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

	b := clearKeyFor(ss.shellPath)
	for len(b) > 0 {
		n, err := ss.ptmx.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}

	ss.resetCapture()

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
	// See CloseSession's identical bump and the generation field's comment.
	ss.generation++

	oldPtmx := ss.ptmx
	oldProc := ss.proc
	ss.ptmx = nil
	ss.proc = nil
	ss.running = false
	ss.mu.Unlock()

	s.releaseOldProcess(ss, oldPtmx, oldProc)

	return nil
}
