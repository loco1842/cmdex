package main

import (
	"strings"
	"testing"
)

const testNonce = "test-nonce"

func newCaptureTestSession() *sessionState {
	return &sessionState{oscNonce: testNonce}
}

func TestCaptureScan_BasicCommand(t *testing.T) {
	ss := newCaptureTestSession()
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07hello\x1b]133;D;test-nonce;0\x07"))

	if !ss.lastValid {
		t.Fatal("expected lastValid=true")
	}
	if ss.lastOutput != "hello" {
		t.Errorf("lastOutput = %q, want %q", ss.lastOutput, "hello")
	}
	if ss.lastExitCode != 0 {
		t.Errorf("lastExitCode = %d, want 0", ss.lastExitCode)
	}
	if ss.lastTruncated {
		t.Error("expected lastTruncated=false")
	}
}

func TestCaptureScan_NonZeroExitCode(t *testing.T) {
	ss := newCaptureTestSession()
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07boom\x1b]133;D;test-nonce;127\x07"))

	if ss.lastExitCode != 127 {
		t.Errorf("lastExitCode = %d, want 127", ss.lastExitCode)
	}
	if ss.lastOutput != "boom" {
		t.Errorf("lastOutput = %q, want %q", ss.lastOutput, "boom")
	}
}

func TestCaptureScan_STTerminator(t *testing.T) {
	ss := newCaptureTestSession()
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x1b\\hi\x1b]133;D;test-nonce;0\x1b\\"))

	if !ss.lastValid || ss.lastOutput != "hi" {
		t.Errorf("lastValid=%v lastOutput=%q, want true/\"hi\"", ss.lastValid, ss.lastOutput)
	}
}

func TestCaptureScan_ANSIInOutputIsStripped(t *testing.T) {
	ss := newCaptureTestSession()
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07\x1b[31mred\x1b[0m\x1b]133;D;test-nonce;1\x07"))

	if ss.lastOutput != "red" {
		t.Errorf("lastOutput = %q, want %q", ss.lastOutput, "red")
	}
	if ss.lastExitCode != 1 {
		t.Errorf("lastExitCode = %d, want 1", ss.lastExitCode)
	}
}

func TestCaptureScan_MarkerSplitAcrossCalls(t *testing.T) {
	full := "\x1b]133;C;test-nonce\x07hello world\x1b]133;D;test-nonce;0\x07"
	for split := 1; split < len(full); split++ {
		ss := newCaptureTestSession()
		ss.captureScan([]byte(full[:split]))
		ss.captureScan([]byte(full[split:]))

		if !ss.lastValid {
			t.Fatalf("split at %d: expected lastValid=true", split)
		}
		if ss.lastOutput != "hello world" {
			t.Fatalf("split at %d: lastOutput = %q, want %q", split, ss.lastOutput, "hello world")
		}
	}
}

func TestCaptureScan_ContentSplitAcrossCalls(t *testing.T) {
	ss := newCaptureTestSession()
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07hel"))
	ss.captureScan([]byte("lo\x1b]133;D;test-nonce;0\x07"))

	if !ss.lastValid || ss.lastOutput != "hello" {
		t.Errorf("lastValid=%v lastOutput=%q, want true/\"hello\"", ss.lastValid, ss.lastOutput)
	}
}

func TestCaptureScan_DWithoutPrecedingCIsIgnored(t *testing.T) {
	ss := newCaptureTestSession()
	// zsh's precmd fires once before any command has run, emitting a bare D.
	ss.captureScan([]byte("\x1b]133;D;test-nonce;0\x07"))

	if ss.lastValid {
		t.Error("expected lastValid=false for a D with no preceding C")
	}
	if ss.capturing {
		t.Error("expected capturing=false after a stray D")
	}
}

func TestCaptureScan_SecondCommandOverwritesFirst(t *testing.T) {
	ss := newCaptureTestSession()
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07first\x1b]133;D;test-nonce;0\x07"))
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07second\x1b]133;D;test-nonce;2\x07"))

	if ss.lastOutput != "second" || ss.lastExitCode != 2 {
		t.Errorf("lastOutput=%q lastExitCode=%d, want \"second\"/2", ss.lastOutput, ss.lastExitCode)
	}
}

func TestCaptureScan_OutputBetweenCommandsIsNotCaptured(t *testing.T) {
	ss := newCaptureTestSession()
	// Prompt text/echo between D and the next C must never appear in output.
	ss.captureScan([]byte(
		"\x1b]133;C;test-nonce\x07first\x1b]133;D;test-nonce;0\x07some-prompt$ " +
			"\x1b]133;C;test-nonce\x07second\x1b]133;D;test-nonce;0\x07",
	))

	if ss.lastOutput != "second" {
		t.Errorf("lastOutput = %q, want %q (prompt chatter leaked in)", ss.lastOutput, "second")
	}
}

func TestCaptureScan_OverflowKeepsTailAndTruncates(t *testing.T) {
	ss := newCaptureTestSession()
	big := strings.Repeat("x", maxCaptureBytes+100)
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07"))
	ss.captureScan([]byte(big))
	ss.captureScan([]byte("\x1b]133;D;test-nonce;0\x07"))

	if !ss.lastTruncated {
		t.Error("expected lastTruncated=true after overflow")
	}
	if len(ss.lastOutput) != maxCaptureBytes {
		t.Errorf("lastOutput length = %d, want %d", len(ss.lastOutput), maxCaptureBytes)
	}
	if !strings.HasSuffix(big, ss.lastOutput) {
		t.Error("expected lastOutput to be the tail of the overflowing output")
	}
}

// Covers the repeated-overflow path a single big write doesn't reach: many
// small writes that each push capBuf further past maxCaptureBytes, exercising
// appendCapture's front-discard on every one of them, not just once.
func TestCaptureScan_RepeatedOverflowKeepsCorrectTail(t *testing.T) {
	ss := newCaptureTestSession()
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07"))

	const chunk = "0123456789"                   // 10 bytes
	chunks := (maxCaptureBytes/len(chunk))*2 + 5 // well past the cap, many writes
	var want strings.Builder
	for i := 0; i < chunks; i++ {
		ss.captureScan([]byte(chunk))
		want.WriteString(chunk)
	}
	ss.captureScan([]byte("\x1b]133;D;test-nonce;0\x07"))

	if !ss.lastTruncated {
		t.Error("expected lastTruncated=true after repeated overflow")
	}
	if len(ss.lastOutput) != maxCaptureBytes {
		t.Errorf("lastOutput length = %d, want %d", len(ss.lastOutput), maxCaptureBytes)
	}
	if !strings.HasSuffix(want.String(), ss.lastOutput) {
		t.Error("expected lastOutput to be the tail of the overflowing output")
	}
}

func TestCaptureScan_UnrelatedOSCPassesThroughAsContent(t *testing.T) {
	ss := newCaptureTestSession()
	// A window-title OSC 0 sequence appears inside the captured output; it
	// must be preserved as content (and stripped by stripANSI, same as any
	// other escape sequence) rather than confusing the marker scanner.
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07before\x1b]0;window title\x07after\x1b]133;D;test-nonce;0\x07"))

	if ss.lastOutput != "beforeafter" {
		t.Errorf("lastOutput = %q, want %q", ss.lastOutput, "beforeafter")
	}
}

func TestCaptureScan_UnrelatedOSCSplitAcrossCallsDoesNotCorruptNextMarker(t *testing.T) {
	ss := newCaptureTestSession()
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07before\x1b]0;win"))
	ss.captureScan([]byte("dow\x07after\x1b]133;D;test-nonce;0\x07"))

	if !ss.lastValid || ss.lastOutput != "beforeafter" {
		t.Errorf("lastValid=%v lastOutput=%q, want true/\"beforeafter\"", ss.lastValid, ss.lastOutput)
	}
}

func TestCaptureScan_UnterminatedMarkerBeyondCarryLimitGivesUp(t *testing.T) {
	ss := newCaptureTestSession()
	// Simulate a "C" marker whose terminator never arrives — after
	// maxMarkerCarryBytes worth of params, the scanner must give up rather
	// than buffering forever, so a later genuine marker is still found.
	ss.captureScan([]byte("\x1b]133;C" + strings.Repeat("z", maxMarkerCarryBytes+10)))
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07hello\x1b]133;D;test-nonce;0\x07"))

	if !ss.lastValid || ss.lastOutput != "hello" {
		t.Errorf(
			"lastValid=%v lastOutput=%q, want true/\"hello\" (scanner should recover)",
			ss.lastValid,
			ss.lastOutput,
		)
	}
}

// --- Nonce authentication: a command's own output must never be able to
// forge a marker (see stripNonce in terminal_capture.go). ---

func TestCaptureScan_WrongNonceIsTreatedAsContentNotMarker(t *testing.T) {
	ss := newCaptureTestSession()
	// A command prints what looks exactly like a real "D" marker, but with
	// the wrong nonce (it can't know the real one) — this must not close
	// the capture or forge the exit code; the bytes should just be part of
	// the captured output.
	ss.captureScan([]byte(
		"\x1b]133;C;test-nonce\x07real output\x1b]133;D;forged;99\x07more output\x1b]133;D;test-nonce;0\x07",
	))

	if !ss.lastValid {
		t.Fatal("expected lastValid=true")
	}
	if ss.lastExitCode != 0 {
		t.Errorf("lastExitCode = %d, want 0 (the forged D must be ignored)", ss.lastExitCode)
	}
	want := "real output" + "more output"
	if ss.lastOutput != want {
		t.Errorf(
			"lastOutput = %q, want %q (forged marker bytes should be kept as literal content)",
			ss.lastOutput,
			want,
		)
	}
}

func TestCaptureScan_WrongNonceCIsTreatedAsContentNotReset(t *testing.T) {
	ss := newCaptureTestSession()
	// A command prints a forged "C" mid-output; it must not reset the
	// in-progress capture buffer.
	ss.captureScan([]byte(
		"\x1b]133;C;test-nonce\x07before\x1b]133;C;forged\x07after\x1b]133;D;test-nonce;0\x07",
	))

	if !ss.lastValid {
		t.Fatal("expected lastValid=true")
	}
	if !strings.Contains(ss.lastOutput, "before") {
		t.Errorf(
			"lastOutput = %q, want it to still contain %q (forged C must not reset the buffer)",
			ss.lastOutput,
			"before",
		)
	}
}

func TestCaptureScan_EmptyNonceTrustsNoMarkers(t *testing.T) {
	// A session with no nonce set (e.g. shell integration inactive) must
	// never treat any marker-shaped bytes as real, even a well-formed one.
	ss := &sessionState{}
	ss.captureScan([]byte("\x1b]133;C;\x07hello\x1b]133;D;;0\x07"))

	if ss.lastValid {
		t.Error("expected lastValid=false when the session has no nonce")
	}
}

// TestCaptureScan_ResizeBeforeDUsesWidthAtCaptureStart covers a Resize
// happening mid-command: the wrap-artifact bytes in the captured output were
// emitted by the shell at the terminal's width when the command STARTED, so
// stripping them at "D" time must use that width, not whatever the terminal
// has since been resized to.
func TestCaptureScan_ResizeBeforeDUsesWidthAtCaptureStart(t *testing.T) {
	ss := newCaptureTestSession()
	ss.capCols.Store(80)
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07occaeca\r\n\x1b[23;80Hati aliquam"))

	// Simulate Resize() being called while the command is still running.
	ss.capCols.Store(40)

	ss.captureScan([]byte("\x1b]133;D;test-nonce;0\x07"))

	if !ss.lastValid {
		t.Fatal("expected lastValid=true")
	}
	want := "occaecati aliquam"
	if ss.lastOutput != want {
		t.Errorf(
			"lastOutput = %q, want %q (wrap artifact must be stripped using the width active when it was emitted)",
			ss.lastOutput,
			want,
		)
	}
}

func TestResetCapture_ClearsAllState(t *testing.T) {
	ss := newCaptureTestSession()
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07hello\x1b]133;D;test-nonce;1\x07"))
	if !ss.lastValid {
		t.Fatal("setup: expected lastValid=true before reset")
	}

	ss.resetCapture()

	if ss.lastValid || ss.lastOutput != "" || ss.lastExitCode != 0 || ss.lastTruncated {
		t.Errorf("resetCapture left stale state: valid=%v output=%q exit=%d truncated=%v",
			ss.lastValid, ss.lastOutput, ss.lastExitCode, ss.lastTruncated)
	}
	if ss.capturing || ss.capBuf.Len() != 0 || len(ss.capCarry) != 0 {
		t.Error("resetCapture left in-flight capture state")
	}
}

func TestGetLastOutput_UnavailableBeforeAnyCommand(t *testing.T) {
	s := &TerminalService{sessions: map[string]*sessionState{}}
	ss := &sessionState{id: "s1"}
	s.sessions["s1"] = ss

	out, err := s.GetLastOutput("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Available {
		t.Error("expected Available=false when no command has completed")
	}
}

func TestGetLastOutput_ReturnsCapturedCommand(t *testing.T) {
	s := &TerminalService{sessions: map[string]*sessionState{}}
	ss := newCaptureTestSession()
	ss.id = "s1"
	ss.captureScan([]byte("\x1b]133;C;test-nonce\x07output text\x1b]133;D;test-nonce;3\x07"))
	s.sessions["s1"] = ss

	out, err := s.GetLastOutput("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Available || out.Text != "output text" || out.ExitCode != 3 {
		t.Errorf("got %+v, want Available=true Text=%q ExitCode=3", out, "output text")
	}
}

func TestGetLastOutput_UnknownSession(t *testing.T) {
	s := &TerminalService{sessions: map[string]*sessionState{}}
	if _, err := s.GetLastOutput("does-not-exist"); err == nil {
		t.Error("expected an error for an unknown session id")
	}
}
