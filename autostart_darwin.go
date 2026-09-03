//go:build darwin

package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

const autostartFileMode = 0o644
const autostartDirectoryMode = 0o700

func autostartPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", autostartLabel+".plist"), nil
}

// setAutostart writes or removes a per-user LaunchAgent plist.
func setAutostart(enabled bool) error {
	path, err := autostartPlistPath()
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

	if err := os.MkdirAll(filepath.Dir(path), autostartDirectoryMode); err != nil {
		return fmt.Errorf("create LaunchAgents directory: %w", err)
	}

	var plist bytes.Buffer
	plist.WriteString(xml.Header)
	plist.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" ` +
		`"http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	plist.WriteString("<plist version=\"1.0\">\n<dict>\n")
	plist.WriteString("\t<key>Label</key>\n\t<string>" + escapeXML(autostartLabel) + "</string>\n")
	plist.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	plist.WriteString("\t\t<string>" + escapeXML(exe) + "</string>\n")
	plist.WriteString("\t\t<string>" + escapeXML(backgroundFlag) + "</string>\n")
	plist.WriteString("\t</array>\n")
	plist.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	// Aqua keeps the agent out of non-GUI (ssh/cron) sessions, where a
	// windowed app would fail to start.
	plist.WriteString("\t<key>LimitLoadToSessionType</key>\n\t<string>Aqua</string>\n")
	plist.WriteString("</dict>\n</plist>\n")

	if err := writeAutostartFile(path, plist.Bytes(), autostartFileMode); err != nil {
		return fmt.Errorf("write login item: %w", err)
	}
	return nil
}

// autostartEnabled reports whether the LaunchAgent plist exists.
func autostartEnabled() bool {
	path, err := autostartPlistPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func escapeXML(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}
