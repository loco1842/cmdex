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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripANSI(tt.in, tt.cols); got != tt.want {
				t.Errorf("stripANSI(%q, %d) = %q, want %q", tt.in, tt.cols, got, tt.want)
			}
		})
	}
}
