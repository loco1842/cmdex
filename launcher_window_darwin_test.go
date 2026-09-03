//go:build darwin

package main

import (
	"testing"
	"unsafe"
)

func TestPositionLauncherWindowNativeMissingWindowIsSafe(t *testing.T) {
	if positionLauncherWindowNative(unsafe.Pointer(nil), launcherWidth, launcherHeight, launcherTopFraction, 0) {
		t.Fatal("native positioner reported success for a missing window")
	}
}
