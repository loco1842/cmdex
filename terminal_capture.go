package main

import (
	"bytes"
	"strconv"
)

// This file implements capture of "last command output" from OSC 133
// semantic-prompt markers emitted by the shell integration scripts installed
// by shell_integration.go. A shell with integration active wraps every
// command with:
//
//	ESC ] 133 ; C BEL   -- emitted just before the command's output begins
//	ESC ] 133 ; D ; <exit-code> BEL   -- emitted once the command has finished
//
// captureScan watches the raw PTY byte stream for these markers and records
// the bytes between the most recent C and D as sessionState.lastOutput, so
// GetLastOutput can return the exact output of the last completed command —
// no reflow, no echoed command text, no prompt-regex guessing. Sessions
// running a shell without integration never see these markers, so
// lastValid stays false forever and the frontend falls back to scraping the
// xterm buffer (Terminal.tsx's getLastOutput).
const (
	// oscCapturePrefix is the fixed portion of the markers this scanner
	// looks for, shared by both the "C" (output start) and "D" (command
	// done) forms.
	oscCapturePrefix = "\x1b]133;"

	// maxCaptureBytes bounds the in-flight captured output for a single
	// command. On overflow, the tail is kept and capTruncated is set —
	// preferring the most recent output over the earliest, since that's
	// what a user copying "the last output" almost always wants.
	maxCaptureBytes = 1 << 20

	// maxMarkerCarryBytes bounds how long captureScan will keep buffering
	// bytes while waiting for a marker's terminator (BEL or ST) before
	// giving up and treating the pending bytes as ordinary content. This
	// guards against unbounded memory growth if a marker is ever malformed
	// or truncated (e.g. a shell integration bug) and its terminator never
	// arrives.
	maxMarkerCarryBytes = 4096
)

// oscCapturePrefixBytes is oscCapturePrefix pre-converted to []byte once, so
// captureScan's per-ESC-byte prefix checks (run on every ANSI escape in the
// stream, not just OSC 133 markers) don't reallocate it on every call.
var oscCapturePrefixBytes = []byte(oscCapturePrefix)

// captureScan feeds newly read PTY bytes through the OSC 133 marker scanner.
// It must be called from the session's single readLoop goroutine only (it is
// not safe to call concurrently with itself), but it takes capMu because
// GetLastOutput reads the resulting fields from a different goroutine.
//
// data is treated as read-only and immutable after this call — callers
// (readLoop) must not reuse or mutate the backing array afterward, since
// captureScan may retain a copy of a trailing partial marker in capCarry
// until the next call resolves it.
func (ss *sessionState) captureScan(data []byte) {
	ss.capMu.Lock()
	defer ss.capMu.Unlock()

	buf := data
	if len(ss.capCarry) > 0 {
		buf = make([]byte, 0, len(ss.capCarry)+len(data))
		buf = append(buf, ss.capCarry...)
		buf = append(buf, data...)
		ss.capCarry = nil
	}

	i := 0
	for i < len(buf) {
		escIdx := bytes.IndexByte(buf[i:], escByte)
		if escIdx == -1 {
			ss.appendCapture(buf[i:])
			return
		}
		escIdx += i

		if escIdx > i {
			ss.appendCapture(buf[i:escIdx])
		}

		remaining := buf[escIdx:]

		// passOneByte treats the ESC at remaining[0] as ordinary content
		// (not our marker, or not confirmed as one yet) and resumes the
		// IndexByte search one byte later, where a real terminator or the
		// next genuine marker can still be found.
		passOneByte := func() {
			ss.appendCapture(remaining[:1])
			i = escIdx + 1
		}

		if len(remaining) < len(oscCapturePrefix) {
			// Not enough bytes yet to know whether this is our marker. Only
			// worth carrying if what we have so far could still become it.
			if bytes.HasPrefix(oscCapturePrefixBytes, remaining) {
				ss.capCarry = append([]byte(nil), remaining...)
				return
			}
			passOneByte()
			continue
		}

		if !bytes.HasPrefix(remaining, oscCapturePrefixBytes) {
			// Some other escape sequence (CSI, a different OSC, etc).
			passOneByte()
			continue
		}

		kindIdx := escIdx + len(oscCapturePrefix)
		if kindIdx >= len(buf) {
			ss.capCarry = append([]byte(nil), remaining...)
			return
		}

		kind := buf[kindIdx]
		if kind != 'C' && kind != 'D' {
			// A different OSC 133 subtype (e.g. "A"/"B"/"P") we don't track.
			passOneByte()
			continue
		}

		termIdx, termLen, found := findOSCTerminator(buf, kindIdx+1)
		if !found {
			if len(remaining) > maxMarkerCarryBytes {
				// Never terminated within a generous bound — give up and
				// treat it as ordinary content rather than buffering
				// forever.
				passOneByte()
				continue
			}
			ss.capCarry = append([]byte(nil), remaining...)
			return
		}

		switch kind {
		case 'C':
			ss.capBuf.Reset()
			ss.capTruncated = false
			ss.capturing = true
		case 'D':
			if ss.capturing {
				ss.lastOutput = stripANSI(ss.capBuf.String())
				ss.lastExitCode = parseExitCode(buf[kindIdx+1 : termIdx])
				ss.lastTruncated = ss.capTruncated
				ss.lastValid = true
				ss.capturing = false
			}
			// A "D" with no preceding "C" happens on the shell's very first
			// precmd (fired before any command has run) — nothing to close.
		}

		i = termIdx + termLen
	}
}

// appendCapture writes b to capBuf when a command's output is actively being
// captured, enforcing maxCaptureBytes by keeping the tail on overflow. It is
// a no-op outside an active C..D span so unrelated shell chatter (prompts,
// key echo) is never recorded.
func (ss *sessionState) appendCapture(b []byte) {
	if !ss.capturing || len(b) == 0 {
		return
	}
	ss.capBuf.Write(b)
	if excess := ss.capBuf.Len() - maxCaptureBytes; excess > 0 {
		// Next(excess) just advances the buffer's read offset — O(1), no
		// copy — rather than re-copying the whole maxCaptureBytes tail on
		// every write once a command's output exceeds the cap.
		ss.capBuf.Next(excess)
		ss.capTruncated = true
	}
}

// findOSCTerminator looks for an OSC terminator (BEL or the two-byte ST,
// ESC '\') starting at buf[start:], returning its index and byte length.
// found is false when neither appears before the end of buf, meaning the
// caller must wait for more data.
func findOSCTerminator(buf []byte, start int) (int, int, bool) {
	for i := start; i < len(buf); i++ {
		switch {
		case buf[i] == belByte:
			return i, 1, true
		case buf[i] == escByte && i+1 < len(buf) && buf[i+1] == '\\':
			return i, escSeqIntroLen, true
		}
	}
	return 0, 0, false
}

// parseExitCode extracts the integer exit code from a "D" marker's params,
// e.g. ";0" or ";127". It defaults to 0 for the params-less/malformed case
// rather than erroring — an unparsable exit code shouldn't block returning
// the (correctly captured) output text.
func parseExitCode(params []byte) int {
	p := bytes.TrimPrefix(params, []byte(";"))
	if len(p) == 0 {
		return 0
	}
	n, err := strconv.Atoi(string(p))
	if err != nil {
		return 0
	}
	return n
}

// resetCapture clears all capture state. Called when a session (re)starts
// its shell or its screen is cleared — in both cases any in-flight or last
// captured output refers to a command the user can no longer see or that no
// longer applies.
//
// On the restart path, callers MUST NOT call this until the previous
// session's readLoop goroutine has actually exited (e.g. after
// releaseOldProcess's readerWg.Wait()) — otherwise a straggling captureScan
// call from that dying goroutine's final read can repopulate the state this
// just cleared.
func (ss *sessionState) resetCapture() {
	ss.capMu.Lock()
	defer ss.capMu.Unlock()

	ss.capBuf.Reset()
	ss.capCarry = nil
	ss.capturing = false
	ss.capTruncated = false
	ss.lastOutput = ""
	ss.lastExitCode = 0
	ss.lastTruncated = false
	ss.lastValid = false
}

// TerminalLastOutput is the result of TerminalService.GetLastOutput.
type TerminalLastOutput struct {
	// Available is false when no command has completed under shell
	// integration yet (including sessions whose shell has no integration at
	// all) — Text/ExitCode/Truncated are zero values in that case, and the
	// frontend should fall back to scraping the xterm buffer.
	Available bool   `json:"available"`
	Text      string `json:"text"`
	ExitCode  int    `json:"exitCode"`
	Truncated bool   `json:"truncated"`
}

// GetLastOutput returns the captured output of the most recently completed
// command in the given session, as recorded via OSC 133 shell-integration
// markers.
func (s *TerminalService) GetLastOutput(sessionId string) (TerminalLastOutput, error) {
	ss, err := s.resolveSession(sessionId)
	if err != nil {
		return TerminalLastOutput{}, err
	}

	ss.capMu.Lock()
	defer ss.capMu.Unlock()

	if !ss.lastValid {
		return TerminalLastOutput{}, nil
	}

	return TerminalLastOutput{
		Available: true,
		Text:      ss.lastOutput,
		ExitCode:  ss.lastExitCode,
		Truncated: ss.lastTruncated,
	}, nil
}
