//go:build darwin || linux

package main

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// writeAutostartFile replaces a per-user autostart file without following a
// symlink planted at the destination. Removing first handles an existing
// symlink, while O_NOFOLLOW closes the check/use gap if another same-user
// process swaps the path before the open.
func writeAutostartFile(path string, data []byte, mode fs.FileMode) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
