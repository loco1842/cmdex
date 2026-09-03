package main

// launcherScreenBounds and launcherFrame are the platform-neutral part of
// launcher placement. Keeping the calculation here makes the AppKit geometry
// policy testable without requiring a running NSWindow or a particular monitor
// layout.
type launcherScreenBounds struct {
	X      int
	Y      int
	Width  int
	Height int
}

type launcherFrame struct {
	X      int
	Y      int
	Width  int
	Height int
}

// presentLauncherWindow keeps the show/placement/focus sequence injectable so
// the global-shortcut path can be tested without starting a desktop app. The
// native callback only places the panel; Wails remains responsible for its
// non-activating Show and Focus operations.
func presentLauncherWindow(prepare func(), show func(), nativePlace func() bool, fallbackPlace func(), focus func()) {
	prepare()
	show()
	if !nativePlace() {
		fallbackPlace()
	}
	focus()
}

func centeredLauncherFrame(screen launcherScreenBounds, width, height int, topFraction float64) launcherFrame {
	x := screen.X + (screen.Width-width)/launcherCenterDivisor
	y := screen.Y + screen.Height - int(float64(screen.Height)*topFraction) - height
	y = max(y, screen.Y)
	return launcherFrame{X: x, Y: y, Width: width, Height: height}
}
