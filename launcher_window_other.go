//go:build !darwin

package main

import "unsafe"

// positionLauncherWindowNative is intentionally a no-op outside macOS. The
// caller supplies the Wails screen-based fallback for those platforms.
func launcherDisplayUnderMouseNative() uint32 {
	return 0
}

func positionLauncherWindowNative(_ unsafe.Pointer, _, _ int, _ float64, _ uint32) bool {
	return false
}
