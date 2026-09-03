package main

// Launch-at-login support.
//
// Each platform gets its own implementation of setAutostart/autostartEnabled:
//
//	darwin  — a per-user LaunchAgent plist in ~/Library/LaunchAgents
//	windows — an HKCU "Run" registry value
//	linux   — an XDG autostart .desktop file in ~/.config/autostart
//
// All three install a user-level entry only, so none of them require elevated
// privileges. The installed entry passes backgroundFlag so CmDex starts with
// its main window hidden and only the global launcher active.

// autostartLabel is the reverse-DNS identifier used for the login item. It
// matches info.productIdentifier in build/config.yml.
const autostartLabel = "com.fenv.cmdex"

// backgroundFlag makes CmDex start without showing the main window. The
// launch-at-login entry always passes it; see main.go.
const backgroundFlag = "--background"
