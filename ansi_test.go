package main

import "testing"

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		cols int
		want string
	}{
		{
			name: "plain text unchanged",
			in:   "hello world",
			cols: 80,
			want: "hello world",
		},
		{
			name: "SGR color codes stripped",
			in:   "\x1b[31mred\x1b[0m \x1b[1;32mbold green\x1b[0m",
			cols: 80,
			want: "red bold green",
		},
		{
			name: "cursor movement CSI stripped",
			in:   "a\x1b[2K\x1b[1Gb",
			cols: 80,
			want: "ab",
		},
		{
			name: "OSC 133 markers stripped",
			in:   "\x1b]133;C\x07hello\x1b]133;D;0\x07",
			cols: 80,
			want: "hello",
		},
		{
			name: "OSC terminated with ST stripped",
			in:   "\x1b]0;window title\x1b\\rest",
			cols: 80,
			want: "rest",
		},
		{
			name: "other two-byte escape stripped",
			in:   "a\x1bMb",
			cols: 80,
			want: "ab",
		},
		{
			name: "crlf normalized to lf",
			in:   "line1\r\nline2",
			cols: 80,
			want: "line1\nline2",
		},
		{
			name: "bare CR collapses progress redraw to final frame",
			in:   "10%\r50%\r100%",
			cols: 80,
			want: "100%",
		},
		{
			name: "trailing lone ESC dropped",
			in:   "abc\x1b",
			cols: 80,
			want: "abc",
		},
		{
			name: "unterminated OSC consumes rest of string",
			in:   "before\x1b]133;Cunterminated",
			cols: 80,
			want: "before",
		},
		{
			name: "empty string",
			in:   "",
			cols: 80,
			want: "",
		},
		{
			// Reproduces the exact byte pattern captured from a real
			// ConPTY session running `ConvertTo-Json` on long values at an
			// 80-column terminal: a word torn in half by an injected
			// "\r\n" + cursor-reposition-to-column-80 sequence, with the
			// boundary character ('a') duplicated on both sides. See
			// removeWrapArtifacts in ansi.go.
			name: "ConPTY line-wrap artifact rejoins the split word and drops the duplicated boundary char",
			in:   "occaeca\r\n\x1b[23;80Hati aliquam",
			cols: 80,
			want: "occaecati aliquam",
		},
		{
			// Also from the same real trace: "mai" + wrap + "iores" for
			// what should read "maiores" — the duplicated 'i' dropped.
			name: "duplicated boundary character dropped (maiiores -> maiores)",
			in:   "accusamus mai\r\n\x1b[5;80Hiores nam est",
			cols: 80,
			want: "accusamus maiores nam est",
		},
		{
			name: "wrap artifact at a different row still matches on column",
			in:   "ab\r\n\x1b[5;80Hcdef",
			cols: 80,
			want: "abcdef",
		},
		{
			name: "narrower terminal matches its own column",
			in:   "ab\r\n\x1b[3;40Hcdef",
			cols: 40,
			want: "abcdef",
		},
		{
			name: "mismatched boundary characters: no duplicate dropped",
			in:   "abc\r\n\x1b[5;80Hdef",
			cols: 80,
			want: "abcdef",
		},
		{
			name: "genuine newline before a column-1 reposition is preserved",
			in:   "line1\",\x1b[24;1H\nline2",
			cols: 80,
			want: "line1\",\nline2",
		},
		{
			name: "reposition to a different column than the terminal width is not a wrap artifact",
			in:   "abc\r\n\x1b[5;40Hdef",
			cols: 80,
			want: "abc\ndef",
		},
		{
			name: "cols <= 0 disables wrap-artifact detection entirely",
			in:   "abc\r\n\x1b[5;80Hdef",
			cols: 0,
			want: "abc\ndef",
		},
		{
			// The wrap boundary can land on a multi-byte UTF-8 rune (e.g. an
			// accented letter). The dedup must compare decoded runes, not
			// raw bytes, or it fails to recognize the duplicate and leaves
			// it in the output.
			name: "duplicated multibyte UTF-8 boundary rune dropped",
			in:   "café \xc3\xa9\r\n\x1b[5;80H\xc3\xa9clair",
			cols: 80,
			want: "café \xc3\xa9clair",
		},
		{
			// Regression: a trailing bare CR (nothing written after it) must
			// not wipe the line it terminates. This is the exact shape
			// PowerShell 7's error rendering produces under ConPTY, and was
			// silently reducing "copy last output" for a failed command to
			// blank lines.
			name: "trailing bare CR keeps the line instead of wiping it",
			in:   "The term 'x' is not recognized.\r",
			cols: 80,
			want: "The term 'x' is not recognized.",
		},
		{
			// The real pwsh shape: two SGR-colored lines, each ending in a
			// CRLF pair with an extra leading bare CR (ConPTY's own repaint,
			// collapsed to one CR by the '\r\n' -> '\n' normalization run
			// earlier in stripANSI). Both lines must survive intact.
			name: "pwsh command-not-found error block survives across two lines",
			in:   "\x1b[31;1mThe term 'x' is not recognized.\x1b[0m\r\r\n\x1b[31;1mCheck the spelling.\x1b[0m\r\r\n",
			cols: 80,
			want: "The term 'x' is not recognized.\nCheck the spelling.\n",
		},
		{
			name: "multiple CRs on one line pick the last non-empty segment",
			in:   "line1\r\rline2",
			cols: 80,
			want: "line2",
		},
		{
			name: "a line made only of CRs collapses to empty",
			in:   "\r\r\r",
			cols: 80,
			want: "",
		},
		{
			// A spinner/progress bar erasing its own line before printing
			// nothing further: "\r\x1b[2K" always means the line is now
			// blank, not "content" — unlike the bare-trailing-CR case above,
			// where nothing tells us the line was ever cleared.
			name: "CR followed by full-line erase wipes the line",
			in:   "content\r\x1b[2K",
			cols: 80,
			want: "",
		},
		{
			// The erase is followed by real replacement text on the same
			// line — the erased line must not resurrect the erased content.
			name: "CR followed by full-line erase then new text keeps only the new text",
			in:   "old status\r\x1b[2Knew status",
			cols: 80,
			want: "new status",
		},
		{
			// \x1b[2K with no preceding CR is just another CSI code being
			// stripped, same as any other cursor/erase command — it must
			// not retroactively wipe text already written on the line.
			name: "full-line erase without a preceding CR does not wipe prior text",
			in:   "a\x1b[2K\x1b[1Gb",
			cols: 80,
			want: "ab",
		},
		{
			// \x1b[K (no param, i.e. default mode 0: erase cursor-to-end)
			// right after a bare CR has the same visible effect as \x1b[2K
			// there, because the CR already put the cursor at column 0 — so
			// this common spinner form ("content\r\x1b[K") must also wipe
			// the line rather than leaving "content" behind.
			name: "CR followed by default-mode erase wipes the line",
			in:   "content\r\x1b[K",
			cols: 80,
			want: "",
		},
		{
			name: "CR followed by explicit mode-0 erase wipes the line",
			in:   "content\r\x1b[0K",
			cols: 80,
			want: "",
		},
		{
			// Mode 1 (erase start-of-line to cursor) is NOT equivalent to
			// mode 0/2 at column 0 in the general case and must not trigger
			// the same whole-line clear.
			name: "CR followed by mode-1 erase does not wipe the line",
			in:   "content\r\x1b[1K",
			cols: 80,
			want: "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripANSI(tt.in, tt.cols); got != tt.want {
				t.Errorf("stripANSI(%q, %d) = %q, want %q", tt.in, tt.cols, got, tt.want)
			}
		})
	}
}
