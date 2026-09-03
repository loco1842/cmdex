package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// autoCheckInterval is how often the opt-in background update check runs.
// The default updater window only surfaces when an update is actually found.
const autoCheckInterval = 12 * time.Hour

// updaterWindowWidth/Height is the full content size of the updater window.
const (
	updaterWindowWidth  = 520
	updaterWindowHeight = 540
)

// updaterThemeCSS layers Cmdex's dark palette over the updater's default
// window so it doesn't flash the framework's light theme. Only the variables
// the updater stylesheet exposes are overridden (see the Updater guide).
const updaterThemeCSS = `:root {
  --bg: #0f0f14;
  --surface: #16161e;
  --surface-2: #1e1e26;
  --fg: #e8e8f0;
  --fg-dim: #a0a0b0;
  --fg-faint: #6b6b78;
  --border: rgba(255, 255, 255, 0.1);
  --accent: #7c6aef;
  --accent-fg: #ffffff;
}`

// updaterDisabled reports whether the updater must stay unconfigured.
// appVersion is only stamped on real builds (Taskfiles + release.yml pass
// -X main.appVersion=...); local `wails3 dev` builds keep "dev" and must not
// hit the GitHub API or offer to swap the dev binary.
func updaterDisabled() bool {
	return appVersion == "" || appVersion == "dev"
}

// initUpdater configures app.Updater with the GitHub Releases provider.
// Must be called after application.New and before app.Run so the helper-mode
// swap path (Restart re-execs into application.New) is armed.
func initUpdater(app *application.App) {
	if updaterDisabled() {
		return
	}
	gh, err := github.New(github.Config{
		Repository:    updaterGitHubRepo,
		ChecksumAsset: "SHA256SUMS",
		AssetMatcher:  matchUpdaterAsset,
	})
	if err != nil {
		app.Logger.Error("updater: github provider", "error", err)
		return
	}
	if err := app.Updater.Init(updater.Config{
		CurrentVersion: appVersion,
		Providers:      []updater.Provider{gh},
		Window: &updater.BuiltinWindow{
			CSS: updaterThemeCSS,
			// Open at the full content size up front. The framework default
			// opens small (348x161) and grows to fit via WindowSizer, but the
			// grow step doesn't fire for late-arriving states (e.g. the error
			// panel), leaving content clipped.
			Options: updater.WindowOptions{
				Title:  "CmDex Updater",
				Width:  updaterWindowWidth,
				Height: updaterWindowHeight,
			},
		},
	}); err != nil {
		app.Logger.Error("updater: init", "error", err)
	}
}

// matchUpdaterAsset picks the release asset for the running platform. On top
// of the default platform+arch substring match it accepts a "universal" macOS
// artifact (cmdex-darwin-universal.zip), which runs on both arm64 and amd64,
// so the release pipeline doesn't need per-arch macOS zips.
func matchUpdaterAsset(req updater.CheckRequest, assets []github.ReleaseAsset) int {
	platform, arch := strings.ToLower(req.Platform), strings.ToLower(req.Arch)
	for i, a := range assets {
		name := strings.ToLower(a.Name)
		if strings.HasSuffix(name, ".sig") {
			continue
		}
		if !strings.Contains(name, platform) {
			continue
		}
		if strings.Contains(name, arch) || strings.Contains(name, "universal") {
			return i
		}
	}
	return -1
}

// UpdateService exposes the in-app updater to the frontend and runs the
// opt-in periodic background check.
type UpdateService struct{}

// ServiceStartup starts the background auto-check loop. It only ever fires
// when the user enabled it in Settings (default off) and this is a stamped
// release build; ticks with no update stay silent (Window only opens when the
// updater itself finds a release via CheckAndInstall).
func (s *UpdateService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	go s.autoCheckLoop(context.WithoutCancel(ctx))
	return nil
}

func (s *UpdateService) autoCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(autoCheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		if db == nil || wailsApp == nil || updaterDisabled() {
			continue
		}
		settings, err := db.GetSettings()
		if err != nil {
			continue
		}
		if settings.AutoUpdateCheck == nil || !*settings.AutoUpdateCheck {
			continue
		}
		if err := wailsApp.Updater.CheckAndInstall(ctx); err != nil {
			fmt.Println("auto update check error:", err)
		}
	}
}

// checkForUpdatesFromMenu runs the update flow from the native menu, which
// has no toast surface: dev builds (updater unconfigured) get a native
// dialog instead of silent nothing, while configured builds open the
// updater window asynchronously (it renders progress, errors, and the
// up-to-date state itself).
func checkForUpdatesFromMenu() {
	if updaterDisabled() || wailsApp == nil {
		wailsApp.Dialog.Info().
			SetTitle("CmDex Updater").
			SetMessage("Updates are not available in development builds. Release builds check GitHub releases for new versions.").
			Show()
		return
	}
	go func() {
		if err := wailsApp.Updater.CheckAndInstall(context.Background()); err != nil {
			wailsApp.Logger.Error("update check failed", "error", err)
		}
	}()
}

// GetAppVersion returns the build-time version, or "dev" for local builds.
func (s *UpdateService) GetAppVersion() string {
	return appVersion
}

// UpdatesEnabled reports whether the updater was configured. It is false for
// local dev builds, where the update UI must stay hidden/disabled.
func (s *UpdateService) UpdatesEnabled() bool {
	return !updaterDisabled()
}

// CheckForUpdates opens the update window and runs check + download/install.
// It returns immediately; the window itself reflects progress, errors, and
// the up-to-date state. Dev builds get an error instead of silent nothing.
func (s *UpdateService) CheckForUpdates() error {
	if updaterDisabled() || wailsApp == nil {
		return errors.New("updates are not available in dev builds")
	}
	go func() {
		if err := wailsApp.Updater.CheckAndInstall(context.Background()); err != nil {
			fmt.Println("CheckForUpdates error:", err)
		}
	}()
	return nil
}

// GetUpdateState returns the updater lifecycle state
// (unconfigured/idle/checking/up-to-date/available/downloading/verifying/
// installing/ready/error), or "unconfigured" for dev builds.
func (s *UpdateService) GetUpdateState() string {
	if updaterDisabled() || wailsApp == nil {
		return string(updater.StateUnconfigured)
	}
	return string(wailsApp.Updater.State())
}

// GetSkippedVersion returns the version the user dismissed via
// "Skip This Version", or "" if none.
func (s *UpdateService) GetSkippedVersion() string {
	if updaterDisabled() || wailsApp == nil {
		return ""
	}
	return wailsApp.Updater.SkippedVersion()
}
