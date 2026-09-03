package main

import (
	"reflect"
	"testing"
)

func TestPresentLauncherWindowUsesNativePlacementBeforeFocus(t *testing.T) {
	var calls []string
	presentLauncherWindow(
		func() { calls = append(calls, "prepare") },
		func() { calls = append(calls, "show") },
		func() bool { calls = append(calls, "native-place"); return true },
		func() { calls = append(calls, "fallback-place") },
		func() { calls = append(calls, "focus") },
	)

	want := []string{"prepare", "show", "native-place", "focus"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("presentation sequence = %v, want %v", calls, want)
	}
}

func TestPresentLauncherWindowNativePlacementIsFinal(t *testing.T) {
	var calls []string
	presentLauncherWindow(
		func() { calls = append(calls, "prepare") },
		func() { calls = append(calls, "show") },
		func() bool { calls = append(calls, "native-place"); return true },
		func() { calls = append(calls, "fallback-place") },
		func() { calls = append(calls, "wails-focus") },
	)

	for _, call := range calls[2:] {
		if call == "fallback-place" {
			t.Fatalf("native presentation was followed by %q: %v", call, calls)
		}
	}
}

func TestPresentLauncherWindowFallsBackWhenNativePlacementUnavailable(t *testing.T) {
	var calls []string
	presentLauncherWindow(
		func() { calls = append(calls, "prepare") },
		func() { calls = append(calls, "show") },
		func() bool { calls = append(calls, "native-place"); return false },
		func() { calls = append(calls, "fallback-place") },
		func() { calls = append(calls, "focus") },
	)

	want := []string{"prepare", "show", "native-place", "fallback-place", "focus"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("presentation sequence = %v, want %v", calls, want)
	}
}

func TestCenteredLauncherFrameOnPrimaryScreen(t *testing.T) {
	got := centeredLauncherFrame(launcherScreenBounds{Width: 1440, Height: 900}, 720, 460, 0.16)
	want := launcherFrame{X: 360, Y: 296, Width: 720, Height: 460}
	if got != want {
		t.Fatalf("frame = %+v, want %+v", got, want)
	}
}

func TestCenteredLauncherFrameOnSecondaryScreen(t *testing.T) {
	got := centeredLauncherFrame(launcherScreenBounds{X: 1440, Y: 0, Width: 1920, Height: 1080}, 720, 460, 0.16)
	want := launcherFrame{X: 2040, Y: 448, Width: 720, Height: 460}
	if got != want {
		t.Fatalf("frame = %+v, want %+v", got, want)
	}
}

func TestCenteredLauncherFrameOnNegativeOriginScreen(t *testing.T) {
	got := centeredLauncherFrame(launcherScreenBounds{X: -1280, Y: -50, Width: 1280, Height: 800}, 720, 460, 0.16)
	want := launcherFrame{X: -1000, Y: 162, Width: 720, Height: 460}
	if got != want {
		t.Fatalf("frame = %+v, want %+v", got, want)
	}
}
