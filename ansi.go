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
// Carriage returns are handled per line: only the text after the last bare
// '\r' on a line survives, which collapses in-place progress-bar/spinner
// redraws down to their final state instead of keeping every intermediate
// frame. '\r\n' is normalized to '\n' first so real line breaks aren't
// treated as redraws.
//
// cols is the session's actual terminal width, used to recognize and elide
// ConPTY's own line-wrap injection artifacts before any of the above runs —
// see removeWrapArtifacts.
func stripANSI(s string, cols int) string {
	s = removeWrapArtifacts(s, cols)
	s = strings.ReplaceAll(s, "\r\n", "\n")

	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != escByte {
			b.WriteByte(c)
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
			if j < len(s) && s[j] >= csiFinalLo && s[j] <= csiFinalHi {
				j++
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

	stripped := b.String()

	lines := strings.Split(stripped, "\n")
	for idx, line := range lines {
		if last := strings.LastIndexByte(line, '\r'); last != -1 {
			lines[idx] = line[last+1:]
		}
	}
	return strings.Join(lines, "\n")
}
