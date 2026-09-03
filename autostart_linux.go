//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const autostartFileMode = 0o600

func autostartDesktopPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	// XDG_CONFIG_HOME is required to be an absolute path. Ignoring a relative
	// value prevents the autostart entry from depending on the app's cwd.
	if dir == "" || !filepath.IsAbs(dir) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "cmdex.desktop"), nil
}

// quoteDesktopEntryArg quotes one argument using the escaping rules for the
// freedesktop Exec key. Go's %q is not suitable here because it can emit Go
// escapes such as \xNN that desktop-entry parsers do not understand.
func quoteDesktopEntryArg(arg string) string {
	var quoted strings.Builder
	quoted.WriteByte('"')
	for _, r := range arg {
		switch r {
		case '\\':
			// Exec values pass through the desktop-entry unquoting rules before
			// field-code expansion. Four backslashes preserve one literal
			// backslash through both parsing stages.
			quoted.WriteString(`\\\\`)
		case '"', '`':
			quoted.WriteByte('\\')
			quoted.WriteRune(r)
		case '$':
			// A dollar sign is reserved in a quoted Exec argument. Two
			// backslashes preserve it through both parsing stages.
			quoted.WriteString(`\\$`)
		case '%':
			// A doubled percent is the literal-percent escape in Exec values;
			// otherwise it starts a desktop-entry field code.
			quoted.WriteString("%%")
		case '\n':
			quoted.WriteString(`\n`)
		case '\r':
			quoted.WriteString(`\r`)
		case '\t':
			quoted.WriteString(`\t`)
		default:
			quoted.WriteRune(r)
		}
	}
	quoted.WriteByte('"')
	return quoted.String()
}

// setAutostart writes or removes an XDG autostart .desktop entry, which is
// honoured by GNOME, KDE, XFCE and other freedesktop-compliant environments.
func setAutostart(enabled bool) error {
	path, err := autostartDesktopPath()
	if err != nil {
		return err
	}

	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove login item: %w", err)
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolve executable symlinks: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create autostart directory: %w", err)
	}

	entry := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=CmDex",
		"Comment=CLI command manager with variable placeholders",
		// Exec is split on spaces by the spec, so quote the binary path using
		// the desktop-entry grammar rather than Go string-literal escaping.
		fmt.Sprintf("Exec=%s %s", quoteDesktopEntryArg(exe), backgroundFlag),
		"Terminal=false",
		"X-GNOME-Autostart-enabled=true",
		"",
	}, "\n")

	if err := writeAutostartFile(path, []byte(entry), autostartFileMode); err != nil {
		return fmt.Errorf("write login item: %w", err)
	}
	return nil
}

// autostartEnabled reports whether the autostart .desktop entry exists.
func autostartEnabled() bool {
	path, err := autostartDesktopPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
