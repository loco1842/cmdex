package main

import "strings"

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
func stripANSI(s string) string {
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
