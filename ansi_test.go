package main

import "testing"

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain text unchanged",
			in:   "hello world",
			want: "hello world",
		},
		{
			name: "SGR color codes stripped",
			in:   "\x1b[31mred\x1b[0m \x1b[1;32mbold green\x1b[0m",
			want: "red bold green",
		},
		{
			name: "cursor movement CSI stripped",
			in:   "a\x1b[2K\x1b[1Gb",
			want: "ab",
		},
		{
			name: "OSC 133 markers stripped",
			in:   "\x1b]133;C\x07hello\x1b]133;D;0\x07",
			want: "hello",
		},
		{
			name: "OSC terminated with ST stripped",
			in:   "\x1b]0;window title\x1b\\rest",
			want: "rest",
		},
		{
			name: "other two-byte escape stripped",
			in:   "a\x1bMb",
			want: "ab",
		},
		{
			name: "crlf normalized to lf",
			in:   "line1\r\nline2",
			want: "line1\nline2",
		},
		{
			name: "bare CR collapses progress redraw to final frame",
			in:   "10%\r50%\r100%",
			want: "100%",
		},
		{
			name: "trailing lone ESC dropped",
			in:   "abc\x1b",
			want: "abc",
		},
		{
			name: "unterminated OSC consumes rest of string",
			in:   "before\x1b]133;Cunterminated",
			want: "before",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripANSI(tt.in); got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
