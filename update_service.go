package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// autoCheckInterval is how often the opt-in background update check runs.
// Checks are headless: a found update downloads in the background and waits
// as "ready" in the About dialog, so ticks with no update stay fully silent.
const autoCheckInterval = 12 * time.Hour

// updaterDisabled reports whether the updater must stay unconfigured.
// appVersion is only stamped on real builds (Taskfiles + release.yml pass
// -X main.appVersion=...); local `wails3 dev` builds keep "dev" and must not
// hit the GitHub API or offer to swap the dev binary.
func updaterDisabled() bool {
	return appVersion == "" || appVersion == "dev"
}

// channelProvider wraps the GitHub provider so the beta-channel toggle takes
// effect without re-running Updater.Init (the framework only allows Init
// once). Flipping the toggle rebuilds the inner provider with the matching
// Prerelease flag; in-flight checks keep the instance they started with.
type channelProvider struct {
	mu    sync.RWMutex
	inner updater.Provider
}

func newChannelProvider(beta bool) (*channelProvider, error) {
	p := &channelProvider{}
	if err := p.setBeta(beta); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *channelProvider) Name() string { return "github" }

func (p *channelProvider) current() updater.Provider {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.inner
}

func (p *channelProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	return p.current().Check(ctx, req)
}

func (p *channelProvider) Download(
	ctx context.Context,
	r *updater.Release,
	dst io.Writer,
	onProgress func(written, total int64),
) error {
	return p.current().Download(ctx, r, dst, onProgress)
}

// setBeta rebuilds the inner provider: beta on walks /releases (prereleases
// included, e.g. v0.3.5-rc1), beta off uses /releases/latest (stable only).
func (p *channelProvider) setBeta(beta bool) error {
	gh, err := github.New(github.Config{
		Repository:    updaterGitHubRepo,
		ChecksumAsset: "SHA256SUMS",
		AssetMatcher:  matchUpdaterAsset,
		Prerelease:    beta,
	})
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inner = gh
	return nil
}

// updateChannelProvider is the single instance handed to Updater.Init;
// SetBetaChannel swaps its inner provider when the toggle flips.
var updateChannelProvider *channelProvider

// updateCheckMu serializes manual and background checks: the framework drops
// overlapping flows, so a trigger arriving while one is in flight is a no-op.
var updateCheckMu sync.Mutex

// pendingUpdateVersion tracks the release found by the last check so the
// About dialog can name it. Guarded by updateCheckMu.
var pendingUpdateVersion string

// initUpdater configures app.Updater with the channel-aware GitHub provider.
// No Window is configured: the in-app About dialog is the update UI and
// renders progress from the wails:updater:* events. Must be called after
// application.New and before app.Run so the helper-mode swap path (Restart
// re-execs into application.New) is armed.
func initUpdater(app *application.App) {
	if updaterDisabled() {
		return
	}
	// db isn't initialized yet at this point (services start on Run), so the
	// provider starts stable-only; ServiceStartup syncs the saved toggle.
	gh, err := newChannelProvider(false)
	if err != nil {
		app.Logger.Error("updater: github provider", "error", err)
		return
	}
	updateChannelProvider = gh
	if err := app.Updater.Init(updater.Config{
		CurrentVersion: appVersion,
		Providers:      []updater.Provider{gh},
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

// ServiceStartup syncs the saved beta-channel toggle (db is ready now, unlike
// at initUpdater time) and starts the background auto-check loop. It only
// ever fires when the user enabled it in Settings (default off) and this is
// a stamped release build.
func (s *UpdateService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	if !updaterDisabled() && updateChannelProvider != nil && db != nil {
		if settings, err := db.GetSettings(); err == nil && settings.BetaChannel != nil {
			if err := updateChannelProvider.setBeta(*settings.BetaChannel); err != nil {
				fmt.Println("updater: apply beta channel error:", err)
			}
		}
	}
	go s.autoCheckLoop()
	return nil
}

func (s *UpdateService) autoCheckLoop() {
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
		runUpdateCheck()
	}
}

// runUpdateCheck performs one headless check and auto-downloads any release
// found (GitHub Desktop behaviour); the About dialog renders progress from
// the wails:updater:* events. The completion time is recorded for the
// "last checked …" line. Overlapping triggers are dropped.
func runUpdateCheck() {
	if updaterDisabled() || wailsApp == nil {
		return
	}
	if !updateCheckMu.TryLock() {
		return
	}
	defer updateCheckMu.Unlock()
	pendingUpdateVersion = ""
	rel, err := wailsApp.Updater.Check(context.Background())
	recordUpdateCheckTime()
	if err != nil {
		wailsApp.Logger.Error("update check failed", "error", err)
		return
	}
	if rel == nil {
		return
	}
	pendingUpdateVersion = rel.Version
	if err := wailsApp.Updater.DownloadAndInstall(context.Background()); err != nil {
		wailsApp.Logger.Error("update download failed", "error", err)
	}
}

// recordUpdateCheckTime stamps the completion time shown in About as
// "last checked …". Failures are best-effort: a missed stamp just leaves the
// previous value.
func recordUpdateCheckTime() {
	if db == nil {
		return
	}
	settings, err := db.GetSettings()
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	settings.LastUpdateCheck = &now
	if err := db.SetSettings(settings); err != nil {
		fmt.Println("record update check time error:", err)
	}
}

// checkForUpdatesFromMenu opens About (the update UI) and kicks off a check
// whose progress About renders inline. Dev builds just open About, which
// shows the dev-build state with the button disabled.
func checkForUpdatesFromMenu() {
	if wailsApp == nil {
		return
	}
	wailsApp.Event.Emit(eventNames.OpenAbout)
	go runUpdateCheck()
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

// CheckForUpdates kicks off a headless check (+ auto-download when a release
// is found). It returns immediately; the About dialog renders progress from
// the wails:updater:* events. Dev builds get an error instead of silent
// nothing.
func (s *UpdateService) CheckForUpdates() error {
	if updaterDisabled() || wailsApp == nil {
		return errors.New("updates are not available in dev builds")
	}
	go runUpdateCheck()
	return nil
}

// RestartToUpdate restarts into the staged update after DownloadAndInstall
// reports ready. Returns an error when nothing is staged (ErrNotReady).
func (s *UpdateService) RestartToUpdate() error {
	if updaterDisabled() || wailsApp == nil {
		return errors.New("updates are not available in dev builds")
	}
	return wailsApp.Updater.Restart(context.Background())
}

// AppInfo is the About dialog snapshot: identity, channel, and live state in
// one call so About renders instantly without waiting for events.
type AppInfo struct {
	Version        string `json:"version"`
	Arch           string `json:"arch"`
	UpdatesEnabled bool   `json:"updatesEnabled"`
	BetaChannel    bool   `json:"betaChannel"`
	// LastCheck is the last completed check, RFC3339 UTC; "" = never.
	LastCheck      string `json:"lastCheck"`
	State          string `json:"state"`
	PendingVersion string `json:"pendingVersion"`
}

// GetAppInfo returns the About snapshot. Dev builds report updatesEnabled
// false with the unconfigured state; the dialog shows its dev-build copy.
func (s *UpdateService) GetAppInfo() AppInfo {
	info := AppInfo{Version: appVersion, Arch: runtime.GOARCH, State: string(updater.StateUnconfigured)}
	if db != nil {
		if settings, err := db.GetSettings(); err == nil {
			if settings.BetaChannel != nil {
				info.BetaChannel = *settings.BetaChannel
			}
			if settings.LastUpdateCheck != nil {
				info.LastCheck = *settings.LastUpdateCheck
			}
		}
	}
	if updaterDisabled() || wailsApp == nil {
		return info
	}
	info.UpdatesEnabled = true
	info.State = string(wailsApp.Updater.State())
	updateCheckMu.Lock()
	info.PendingVersion = pendingUpdateVersion
	updateCheckMu.Unlock()
	return info
}

// SetBetaChannel persists the prerelease toggle and rebuilds the provider so
// the next check walks /releases (beta on) or /releases/latest (beta off).
// It takes effect immediately, including for the running auto-check loop.
func (s *UpdateService) SetBetaChannel(enabled bool) error {
	if db == nil {
		return errors.New("settings store unavailable")
	}
	settings, err := db.GetSettings()
	if err != nil {
		return err
	}
	settings.BetaChannel = &enabled
	if err := db.SetSettings(settings); err != nil {
		return err
	}
	if updateChannelProvider != nil {
		if err := updateChannelProvider.setBeta(enabled); err != nil {
			return err
		}
	}
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
