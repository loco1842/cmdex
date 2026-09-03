//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const autostartRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// autostartValueName is the registry value holding the CmDex launch command.
const autostartValueName = "CmDex"

type autostartRegistryKey interface {
	Close() error
	DeleteValue(name string) error
	SetStringValue(name, value string) error
}

var openOrCreateAutostartKey = func() (autostartRegistryKey, error) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, autostartRunKey, registry.SET_VALUE)
	return key, err
}

// setAutostart adds or removes the HKCU Run entry. HKCU needs no elevation.
func setAutostart(enabled bool) error {
	// CreateKey is intentionally used instead of OpenKey: the per-user Run key
	// is not guaranteed to exist on a fresh Windows profile.
	key, err := openOrCreateAutostartKey()
	if err != nil {
		return fmt.Errorf("open or create Run registry key: %w", err)
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue(autostartValueName); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("remove login item: %w", err)
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	// Quote the path so spaces in Program Files do not split the command.
	value := fmt.Sprintf("%q %s", exe, backgroundFlag)
	if err := key.SetStringValue(autostartValueName, value); err != nil {
		return fmt.Errorf("write login item: %w", err)
	}
	return nil
}

// autostartEnabled reports whether the Run entry is present.
func autostartEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	value, _, err := key.GetStringValue(autostartValueName)
	return err == nil && value != ""
}
