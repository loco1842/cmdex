package main

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// removeWrapArtifacts strips the exact byte pattern Windows ConPTY injects
// when the real console screen buffer auto-wraps a too-long output line at
// its configured column width: a genuine "\r\n", immediately followed by a
// CSI cursor-position command ("ESC [ <row> ; <cols> H") that repositions to
// THAT SAME column. Unlike a Unix pty, where line-wrapping is purely a
// client-rendering concern that never touches the byte stream, ConPTY can
// split a single long line mid-character with real CRLF bytes — observed
// directly by tracing raw PTY output for a long `ConvertTo-Json` value: a
// word like "occaecati" arrived as "occaeca" + "\r\n" + "\x1b[23;80H" +
// "ati", i.e. the actual word torn in half around the wrap. Left alone,
// stripANSI's normal CSI stripping removes the escape but leaves the
// injected "\r\n" behind as if it were a real line break — turning JSON
// strings containing any long value into invalid JSON once copied.
//
// This is what makes the pattern safe to recognize and remove outright
// (both the CRLF and the escape, joining the surrounding text back
// together) rather than treating it as a line break: genuine multi-line
// content never repositions to the far-right column right after a
// newline — a real new line always starts at column 1. cols must be the
// session's actual configured terminal width (the wrapping column ConPTY
// used); a mismatch (stale size, or 0 when unknown) simply means this pass
// matches nothing and s passes through unchanged.
//
// The character at the wrap boundary itself is also deduplicated: ConPTY's
// "deferred wrap" handling (a character written at the last column doesn't
// advance the cursor until the NEXT character arrives, the same delayed-wrap
// behavior real VT100 terminals use) re-emits that boundary character again
// after repositioning — "mai" + wrap + "iores" for what should be
// "maiores", "nobis q" + wrap + "qui" for "nobis qui". Both empirical
// examples came from the same trace as the split-word case above, and in
// both the character immediately before the CRLF and immediately after the
// escape are identical, which is what makes it safe to drop one copy rather
// than a guess: if they don't match, nothing is dropped, erring toward
// keeping a real character over risking one that happens to abut the
// pattern by coincidence. The comparison decodes full UTF-8 runes rather
// than raw bytes on either side, since the boundary character can be
// multi-byte (e.g. an accented letter); comparing single bytes would compare
// unrelated continuation bytes and never recognize the duplicate.
func removeWrapArtifacts(s string, cols int) string {
	if cols <= 0 {
		return s
	}
	colStr := strconv.Itoa(cols)

	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); {
		if j, ok := matchWrapArtifact(s, i, colStr); ok {
			if i > 0 && j < len(s) {
				prevRune, _ := utf8.DecodeLastRuneInString(s[:i])
				nextRune, nextSize := utf8.DecodeRuneInString(s[j:])
				if prevRune != utf8.RuneError && prevRune == nextRune {
					j += nextSize
				}
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// matchWrapArtifact reports whether s[i:] begins with "\r\n" + ESC '[' +
// digits + ';' + colStr + 'H' (see removeWrapArtifacts), returning the index
// just past the matched span when it does. Guards against colStr matching
// only a numeric prefix of a longer column parameter (e.g. colStr "8"
// against an actual "80") by requiring 'H' to immediately follow colStr —
// any leftover digit before 'H' fails that check and the match is rejected.
func matchWrapArtifact(s string, i int, colStr string) (int, bool) {
	prefix := "\r\n" + string(rune(escByte)) + "["
	if !strings.HasPrefix(s[i:], prefix) {
		return i, false
	}
	j := i + len(prefix)
	digitsStart := j
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == digitsStart || j >= len(s) || s[j] != ';' {
		return i, false
	}
	j++
	if !strings.HasPrefix(s[j:], colStr) {
		return i, false
	}
	j += len(colStr)
	if j >= len(s) || s[j] != 'H' {
		return i, false
	}
	return j + 1, true
}

// ASCII control bytes and CSI/OSC byte-range bounds this file's escape
// parsing recognizes (also reused by terminal_capture.go's marker scanner,
// which looks for the same ESC/BEL bytes).
const (
	escByte = 0x1B // ESC — introduces every escape sequence handled here
	belByte = 0x07 // BEL — one of the two valid OSC terminators

	csiParamLo = 0x20 // CSI parameter/intermediate byte range: 0x20-0x3F
	csiParamHi = 0x3F
	csiFinalLo = 0x40 // CSI final byte range: 0x40-0x7E
	csiFinalHi = 0x7E

	// escSeqIntroLen is the length of "ESC + one more byte" — the shape
	// shared by a CSI introducer (ESC '['), an OSC introducer (ESC ']'),
	// and the two-byte ST terminator (ESC '\').
	escSeqIntroLen = 2
)

// stripANSI removes terminal escape sequences from s and normalizes line
// endings, leaving plain text suitable for copying to the clipboard.
//
// It understands three escape shapes, in order of how they appear in real
// PTY output:
//   - CSI sequences: ESC '[' followed by parameter/intermediate bytes
//     (0x30-0x3F, 0x20-0x2F) and a final byte in 0x40-0x7E (e.g. SGR color
//     codes, cursor movement).
//   - OSC sequences: ESC ']' followed by any bytes up to a terminator, either
//     BEL (0x07) or the two-byte ST (ESC '\'). This also removes this
//     package's own OSC 133 semantic-prompt markers (see terminal_capture.go)
//     so they never leak into copied text.
//   - Other two-byte escapes: ESC followed by a single character (e.g. RIS,
//     charset selection) that don't fit the CSI/OSC shape above.
//
// Carriage returns are handled per line via collapseCarriageReturns, which
// collapses in-place progress-bar/spinner redraws down to their final state
// instead of keeping every intermediate frame. '\r\n' is normalized to '\n'
// first so real line breaks aren't treated as redraws.
//
// cols is the session's actual terminal width, used to recognize and elide
// ConPTY's own line-wrap injection artifacts before any of the above runs —
// see removeWrapArtifacts.
func stripANSI(s string, cols int) string {
	s = removeWrapArtifacts(s, cols)
	s = strings.ReplaceAll(s, "\r\n", "\n")

	buf := make([]byte, 0, len(s))
	lineStart := 0 // index into buf where the current physical line began

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != escByte {
			buf = append(buf, c)
			if c == '\n' {
				lineStart = len(buf)
			}
			continue
		}

		// c == ESC. Determine the escape shape from the next byte.
		if i+1 >= len(s) {
			break // trailing lone ESC with nothing after it — drop it
		}

		switch s[i+1] {
		case '[': // CSI
			j := i + escSeqIntroLen
			for j < len(s) && (s[j] >= csiParamLo && s[j] <= csiParamHi) {
				j++
			}
			var param string
			var final byte
			if j < len(s) && s[j] >= csiFinalLo && s[j] <= csiFinalHi {
				param = s[i+escSeqIntroLen : j]
				final = s[j]
				j++
			}
			// EL (Erase in Line) modes 0 (default/no param) and 2 both clear
			// the WHOLE line here, even though in general EL0 only erases
			// from the cursor to end of line: a bare '\r' immediately before
			// puts the cursor at column 0, so "cursor to end" already covers
			// the entire line, same as mode 2's unconditional whole-line
			// erase. Mode 1 (erase start-of-line to cursor) is deliberately
			// excluded — at column 0 it would erase nothing anyway, but more
			// importantly a bare '\r' doesn't imply mode-1 semantics the way
			// it does for 0/2. A progress bar or spinner erasing itself with
			// "content\r\x1b[K" (the common default form) or
			// "content\r\x1b[2K" must not leave "content" as the last visible
			// line: once the CSI bytes are stripped, that pattern is
			// byte-for-byte indistinguishable from a bare trailing '\r' with
			// nothing typed after it — the ConPTY repaint case
			// collapseCarriageReturns' doc comment deliberately preserves —
			// so only a "\r" immediately followed by this exact erase
			// (nothing typed in between) is treated as clearing the line; a
			// K with no preceding '\r', or with real text after the '\r', is
			// left to whatever was already written, unaffected.
			if final == 'K' && (param == "" || param == "0" || param == "2") && len(buf) > 0 &&
				buf[len(buf)-1] == '\r' {
				buf = buf[:lineStart]
			}
			i = j - 1
		case ']': // OSC
			j := i + escSeqIntroLen
			for j < len(s) {
				if s[j] == belByte {
					j++
					break
				}
				if s[j] == escByte && j+1 < len(s) && s[j+1] == '\\' {
					j += escSeqIntroLen
					break
				}
				j++
			}
			i = j - 1
		default: // other two-byte escape
			i++
		}
	}

	lines := strings.Split(string(buf), "\n")
	for idx, line := range lines {
		lines[idx] = collapseCarriageReturns(line)
	}
	return strings.Join(lines, "\n")
}

// collapseCarriageReturns collapses a single line's in-place '\r' redraws
// down to their final state, the way a terminal overwrites a line from
// column 0 every time it sees a bare CR: it returns the LAST NON-EMPTY
// segment between carriage returns, not unconditionally "everything after
// the final one".
//
// That distinction matters for a trailing CR with nothing written after it —
// e.g. a ConPTY-repainted row, or how PowerShell renders an error record —
// which means nothing actually overwrote the line. Naively keeping only the
// text after the last '\r' would discard the whole line in that case (an
// empty final segment), silently wiping real output; this previously made
// "copy last output" return blank lines for a failed command on Windows
// instead of the error text. Splitting on '\r' (a single ASCII byte, never a
// UTF-8 continuation byte) is safe to do on the raw string without decoding
// runes.
func collapseCarriageReturns(line string) string {
	if !strings.Contains(line, "\r") {
		return line
	}
	// Scan backward from the end, one '\r'-delimited segment at a time,
	// stopping at the first non-empty one — equivalent to strings.Split
	// followed by a reverse walk, but without allocating a segment per '\r'
	// in a line a spinner may have redrawn thousands of times.
	end := len(line)
	for end > 0 {
		start := strings.LastIndexByte(line[:end], '\r') + 1
		if start < end {
			return line[start:end]
		}
		end = start - 1
	}
	return ""
}
