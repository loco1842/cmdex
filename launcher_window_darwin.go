//go:build darwin

package main

/*
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <AppKit/AppKit.h>
#import <Cocoa/Cocoa.h>
#include <math.h>
#include <stdbool.h>

static NSPanel* launcherPanel(void *window) {
	if (window == NULL) {
		return nil;
	}
	NSWindow *nativeWindow = (NSWindow *)window;
	if (![nativeWindow isKindOfClass:[NSPanel class]]) {
		return nil;
	}
	return (NSPanel *)nativeWindow;
}

static NSScreen* screenContainingMouse(void) {
	NSPoint mouse = [NSEvent mouseLocation];
	for (NSScreen *screen in [NSScreen screens]) {
		if (NSPointInRect(mouse, screen.frame)) {
			return screen;
		}
	}
	return nil;
}

static unsigned int displayIDForScreen(NSScreen *screen) {
	if (screen == nil) {
		return 0;
	}
	NSNumber *number = [[screen deviceDescription] objectForKey:@"NSScreenNumber"];
	return number == nil ? 0 : [number unsignedIntValue];
}

static unsigned int launcherDisplayUnderMouse(void) {
	return displayIDForScreen(screenContainingMouse());
}

static NSScreen* screenForDisplayID(unsigned int displayID) {
	if (displayID == 0) {
		return nil;
	}
	for (NSScreen *screen in [NSScreen screens]) {
		if (displayIDForScreen(screen) == displayID) {
			return screen;
		}
	}
	return nil;
}

// Wails does not expose the pointer's target screen. NativeWindow does expose
// the actual Wails NSPanel, so this helper only computes and applies its frame;
// Show and Focus remain Wails' non-activating panel operations.
static bool positionLauncherPanel(void *window, int width, int height, double topFraction, unsigned int displayID) {
	NSPanel *panel = launcherPanel(window);
	if (panel == nil) {
		return false;
	}

	NSScreen *screen = screenForDisplayID(displayID);
	if (screen == nil) {
		screen = panel.screen;
	}
	if (screen == nil) {
		screen = [NSScreen mainScreen];
	}
	if (screen == nil) {
		return false;
	}

	// AppKit's frame coordinates are global and support negative origins for
	// displays arranged above or to the left of the primary display.
	NSRect visibleFrame = screen.visibleFrame;
	CGFloat x = NSMidX(visibleFrame) - ((CGFloat)width / 2.0);
	CGFloat y = NSMaxY(visibleFrame) - visibleFrame.size.height * (CGFloat)topFraction - (CGFloat)height;
	if (y < NSMinY(visibleFrame)) {
		y = NSMinY(visibleFrame);
	}
	NSRect intendedFrame = NSMakeRect(x, y, width, height);
	[panel setFrame:intendedFrame display:YES animate:NO];

	NSRect committedFrame = panel.frame;
	return fabs(committedFrame.origin.x - x) <= 1.0 &&
		fabs(committedFrame.origin.y - y) <= 1.0 &&
		fabs(committedFrame.size.width - width) <= 1.0 &&
		fabs(committedFrame.size.height - height) <= 1.0;
}
*/
import "C"
import "unsafe"

// positionLauncherWindowNative operates on the actual Wails NSPanel. It never
// orders, focuses, activates, or hides the window; those operations must stay
// on Wails' supported non-activating panel path.
func launcherDisplayUnderMouseNative() uint32 {
	return uint32(C.launcherDisplayUnderMouse())
}

func positionLauncherWindowNative(
	window unsafe.Pointer,
	width int,
	height int,
	topFraction float64,
	displayID uint32,
) bool {
	return bool(C.positionLauncherPanel(
		window,
		C.int(width),
		C.int(height),
		C.double(topFraction),
		C.uint(displayID),
	))
}
