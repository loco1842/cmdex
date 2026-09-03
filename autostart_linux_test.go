//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decodeDesktopEntryQuotedArg models the two unquoting passes described by
// the Desktop Entry Exec specification: general string escaping followed by
// quoted-argument escaping. It is deliberately test-only; the production
// writer is checked against the parser semantics rather than a copied list of
// expected slash counts.
func decodeDesktopEntryQuotedArg(encoded string) string {
	if len(encoded) < 2 || encoded[0] != '"' || encoded[len(encoded)-1] != '"' {
		return encoded
	}
	value := encoded[1 : len(encoded)-1]
	for pass := 0; pass < 2; pass++ {
		var decoded strings.Builder
		for i := 0; i < len(value); i++ {
			if value[i] == '\\' && i+1 < len(value) {
				decoded.WriteByte(value[i+1])
				i++
				continue
			}
			decoded.WriteByte(value[i])
		}
		value = decoded.String()
	}
	return value
}

func TestQuoteDesktopEntryArg(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{name: "spaces", arg: "/opt/CmDex App/cmdex", want: `"/opt/CmDex App/cmdex"`},
		{name: "field code marker", arg: "/opt/100%/cmdex", want: `"/opt/100%%/cmdex"`},
		{name: "desktop metacharacters", arg: "/opt/CmDex\\\"" + "`" + "/cmdex", want: `"` + `/opt/CmDex\\\"` + "\\`" + `/cmdex"`},
		{name: "literal backslash", arg: `/opt/CmDex\bin/cmdex`, want: `"/opt/CmDex\\\\bin/cmdex"`},
		{name: "literal dollar sign", arg: `/opt/$CmDex/cmdex`, want: `"/opt/\\$CmDex/cmdex"`},
		{name: "control characters", arg: "/opt/CmDex\tApp\ncmdex", want: `"/opt/CmDex\tApp\ncmdex"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteDesktopEntryArg(tt.arg); got != tt.want {
				t.Fatalf("quoteDesktopEntryArg(%q) = %q, want %q", tt.arg, got, tt.want)
			}
		})
	}
}

func TestQuoteDesktopEntryArgRoundTripsReservedCharacters(t *testing.T) {
	tests := []string{
		`/opt/CmDex\bin/cmdex`,
		`/opt/$CmDex/cmdex`,
		`/opt/CmDex\$bin/cmdex`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			encoded := quoteDesktopEntryArg(input)
			if got := decodeDesktopEntryQuotedArg(encoded); got != input {
				t.Fatalf("quoteDesktopEntryArg(%q) round-tripped as %q (encoded %q)", input, got, encoded)
			}
		})
	}
}

func TestSetAutostartDoesNotFollowExistingSymlink(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)

	path, err := autostartDesktopPath()
	if err != nil {
		t.Fatalf("autostartDesktopPath failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create autostart directory: %v", err)
	}
	victim := filepath.Join(t.TempDir(), "victim")
	original := []byte("do not overwrite")
	if err := os.WriteFile(victim, original, 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	if err := os.Symlink(victim, path); err != nil {
		t.Fatalf("create autostart symlink: %v", err)
	}

	if err := setAutostart(true); err != nil {
		t.Fatalf("setAutostart failed: %v", err)
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("victim changed through autostart symlink: got %q, want %q", got, original)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat replacement: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("autostart destination remains a symlink")
	}
}

func TestAutostartDesktopPathIgnoresRelativeXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "relative-config")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve home directory: %v", err)
	}
	want := filepath.Join(home, ".config", "autostart", "cmdex.desktop")
	got, err := autostartDesktopPath()
	if err != nil {
		t.Fatalf("autostartDesktopPath failed: %v", err)
	}
	if got != want {
		t.Fatalf("autostartDesktopPath = %q, want %q", got, want)
	}
}
