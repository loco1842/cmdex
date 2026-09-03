package main

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type monitorTestHandle struct {
	closed bool
}

func (h *monitorTestHandle) Read([]byte) (int, error)    { return 0, io.EOF }
func (h *monitorTestHandle) Write(p []byte) (int, error) { return len(p), nil }
func (h *monitorTestHandle) Close() error {
	h.closed = true
	return nil
}

type monitorTestProcess struct{}

func (monitorTestProcess) Pid() int           { return 1 }
func (monitorTestProcess) Wait() (int, error) { return 1, nil }
func (monitorTestProcess) Exited() bool       { return true }

type monitorTestStableProcess struct{}

func (monitorTestStableProcess) Pid() int           { return 1 }
func (monitorTestStableProcess) Wait() (int, error) { return 0, nil }
func (monitorTestStableProcess) Exited() bool       { return true }

type readinessTestProcess struct {
	done chan struct{}
	once sync.Once
}

func newReadinessTestProcess() *readinessTestProcess {
	return &readinessTestProcess{done: make(chan struct{})}
}

func (p *readinessTestProcess) Pid() int { return 1 }
func (p *readinessTestProcess) Wait() (int, error) {
	<-p.done
	return 0, nil
}
func (p *readinessTestProcess) Exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}
func (p *readinessTestProcess) stop() { p.once.Do(func() { close(p.done) }) }

type readinessTestHandle struct {
	mu     sync.Mutex
	writes []byte
}

func (h *readinessTestHandle) Read([]byte) (int, error) { return 0, io.EOF }
func (h *readinessTestHandle) Write(data []byte) (int, error) {
	h.mu.Lock()
	h.writes = append(h.writes, data...)
	h.mu.Unlock()
	return len(data), nil
}
func (h *readinessTestHandle) Close() error { return nil }
func (h *readinessTestHandle) output() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]byte(nil), h.writes...)
}

type readinessTestBackend struct {
	mu           sync.Mutex
	startEntered chan struct{}
	allowStart   chan struct{}
	startHandle  *readinessTestHandle
	startProcess *readinessTestProcess
	startErr     error
	blockStart   bool
}

func (b *readinessTestBackend) Start(_, _, _ string, _, _ int, _ shellLaunchOpts) (ptyHandle, ptyProcess, error) {
	if b.startEntered != nil {
		select {
		case <-b.startEntered:
		default:
			close(b.startEntered)
		}
	}
	if b.blockStart {
		<-b.allowStart
	}
	if b.startErr != nil {
		return nil, nil, b.startErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.startHandle == nil {
		b.startHandle = &readinessTestHandle{}
	}
	if b.startProcess == nil {
		b.startProcess = newReadinessTestProcess()
	}
	return b.startHandle, b.startProcess, nil
}

func (b *readinessTestBackend) Resize(ptyHandle, int, int) error { return nil }
func (b *readinessTestBackend) Kill(proc ptyProcess) error {
	if p, ok := proc.(*readinessTestProcess); ok {
		p.stop()
	}
	return nil
}

type resizeDuringRestartBackend struct {
	mu        sync.Mutex
	startCh   chan struct{}
	startRows int
	startCols int
}

func (b *resizeDuringRestartBackend) Start(_, _, _ string, rows, cols int, _ shellLaunchOpts) (ptyHandle, ptyProcess, error) {
	b.mu.Lock()
	b.startRows = rows
	b.startCols = cols
	b.mu.Unlock()
	close(b.startCh)
	return &monitorTestHandle{}, monitorTestStableProcess{}, nil
}

func (b *resizeDuringRestartBackend) Resize(ptyHandle, int, int) error { return nil }
func (b *resizeDuringRestartBackend) Kill(ptyProcess) error            { return nil }

func TestWaitForAutoRestartStopsDuringDelay(t *testing.T) {
	stopCh := make(chan struct{})
	result := make(chan bool, 1)
	timer := time.NewTimer(time.Hour)
	go func() { result <- waitForAutoRestartTimer(stopCh, timer) }()

	close(stopCh)

	select {
	case restarted := <-result:
		if restarted {
			t.Fatal("waitForAutoRestart reported a restart after the session was stopped")
		}
	case <-time.After(autoRestartDelay):
		t.Fatal("waitForAutoRestart did not observe the stop signal")
	}
}

func TestMonitorExitRestartUsesSizeResizedDuringDelay(t *testing.T) {
	backend := &resizeDuringRestartBackend{startCh: make(chan struct{})}
	s := &TerminalService{ptyBackend: backend, sessions: make(map[string]*sessionState)}
	stopCh := make(chan struct{})
	handle := &monitorTestHandle{}
	ss := &sessionState{
		id:       "resize-restart",
		ptmx:     handle,
		proc:     monitorTestProcess{},
		stopCh:   stopCh,
		running:  true,
		lastSize: ptyWinsize{Rows: 24, Cols: 80},
	}
	s.sessions[ss.id] = ss

	// Replace the real delay with a gate so Resize is guaranteed to happen
	// between the initial exit and the replacement Start call.
	originalWait := waitForAutoRestart
	delayReady := make(chan struct{})
	continueRestart := make(chan struct{})
	waitForAutoRestart = func(<-chan struct{}) bool {
		close(delayReady)
		<-continueRestart
		return true
	}
	t.Cleanup(func() { waitForAutoRestart = originalWait })

	done := make(chan struct{})
	go func() {
		s.monitorExit(ss, monitorTestProcess{}, stopCh)
		close(done)
	}()
	select {
	case <-delayReady:
	case <-time.After(time.Second):
		t.Fatal("monitorExit did not enter the restart delay")
	}

	if err := s.Resize(ss.id, 140, 42); err != nil {
		t.Fatalf("Resize during restart delay failed: %v", err)
	}
	close(continueRestart)

	select {
	case <-backend.startCh:
	case <-time.After(time.Second):
		t.Fatal("replacement PTY was not started")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitorExit did not finish")
	}

	backend.mu.Lock()
	rows, cols := backend.startRows, backend.startCols
	backend.mu.Unlock()
	if rows != 42 || cols != 140 {
		t.Fatalf("replacement PTY dimensions = rows=%d cols=%d, want rows=42 cols=140", rows, cols)
	}
}

func TestCreateInternalSessionIsImmediatelyWritable(t *testing.T) {
	backend := &readinessTestBackend{}
	s := &TerminalService{ptyBackend: backend, sessions: make(map[string]*sessionState)}
	info, err := s.CreateInternalSession("Launcher")
	if err != nil {
		t.Fatalf("CreateInternalSession failed: %v", err)
	}
	t.Cleanup(func() { _ = s.CloseSession(info.ID) })

	if err := s.Write(info.ID, "echo ready\r"); err != nil {
		t.Fatalf("Write immediately after CreateInternalSession failed: %v", err)
	}
	if got := string(backend.startHandle.output()); got != "echo ready\r" {
		t.Fatalf("initial PTY received %q, want %q", got, "echo ready\r")
	}
}

func TestWriteWaitsForRestartBeforeWriting(t *testing.T) {
	backend := &readinessTestBackend{
		startEntered: make(chan struct{}),
		allowStart:   make(chan struct{}),
		blockStart:   true,
	}
	s := &TerminalService{ptyBackend: backend, sessions: make(map[string]*sessionState)}
	stopCh := make(chan struct{})
	ss := &sessionState{
		id:       "write-restart",
		ptmx:     &monitorTestHandle{},
		proc:     monitorTestProcess{},
		stopCh:   stopCh,
		running:  true,
		lastSize: ptyWinsize{Rows: 24, Cols: 80},
	}
	s.sessions[ss.id] = ss
	t.Cleanup(func() { _ = s.CloseSession(ss.id) })

	originalWait := waitForAutoRestart
	delayReady := make(chan struct{})
	continueRestart := make(chan struct{})
	waitForAutoRestart = func(<-chan struct{}) bool {
		close(delayReady)
		<-continueRestart
		return true
	}
	t.Cleanup(func() { waitForAutoRestart = originalWait })

	monitorDone := make(chan struct{})
	go func() {
		s.monitorExit(ss, monitorTestProcess{}, stopCh)
		close(monitorDone)
	}()
	select {
	case <-delayReady:
	case <-time.After(time.Second):
		t.Fatal("monitorExit did not enter the restart delay")
	}
	close(continueRestart)
	select {
	case <-backend.startEntered:
	case <-time.After(time.Second):
		t.Fatal("replacement PTY did not begin starting")
	}

	writeDone := make(chan error, 1)
	go func() { writeDone <- s.Write(ss.id, "echo after-restart\r") }()
	select {
	case err := <-writeDone:
		t.Fatalf("Write returned before replacement PTY was ready: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(backend.allowStart)
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("Write after replacement PTY became ready failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Write did not complete after replacement PTY became ready")
	}
	if got := string(backend.startHandle.output()); got != "echo after-restart\r" {
		t.Fatalf("replacement PTY received %q, want %q", got, "echo after-restart\r")
	}

	select {
	case <-monitorDone:
	case <-time.After(time.Second):
		t.Fatal("monitorExit did not finish")
	}
}

func TestWriteReturnsStartErrorInsteadOfTerminalNotStarted(t *testing.T) {
	startErr := errors.New("backend start failed")
	backend := &readinessTestBackend{startErr: startErr}
	s := &TerminalService{ptyBackend: backend, sessions: make(map[string]*sessionState)}
	ss := &sessionState{
		id:        "write-start-error",
		starting:  true,
		startDone: make(chan struct{}),
		lastSize:  ptyWinsize{Rows: 24, Cols: 80},
	}
	s.sessions[ss.id] = ss

	originalWait := waitForSessionReady
	startDone := ss.startDone
	waitForSessionReady = func(<-chan struct{}) error {
		close(startDone)
		ss.mu.Lock()
		ss.starting = false
		ss.startErr = startErr
		ss.mu.Unlock()
		return nil
	}
	t.Cleanup(func() { waitForSessionReady = originalWait })

	err := s.Write(ss.id, "echo should-fail\r")
	if !errors.Is(err, startErr) {
		t.Fatalf("Write error = %v, want backend start error %v", err, startErr)
	}
}

func TestWaitForSessionReadyTimesOut(t *testing.T) {
	originalTimeout := sessionReadyTimeout
	sessionReadyTimeout = time.Millisecond
	t.Cleanup(func() { sessionReadyTimeout = originalTimeout })

	err := waitForSessionReady(make(chan struct{}))
	if err == nil || err.Error() != "terminal start timed out" {
		t.Fatalf("waitForSessionReady error = %v, want terminal start timed out", err)
	}
}
