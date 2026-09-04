package main

import (
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func updaterTestAssets() []github.ReleaseAsset {
	return []github.ReleaseAsset{
		{Name: "cmdex-darwin-universal.zip"},
		{Name: "cmdex-linux-amd64"},
		{Name: "cmdex-windows-amd64.exe"},
		{Name: "SHA256SUMS"},
		{Name: "cmdex-1.0-installer.exe"},
	}
}

func TestMatchUpdaterAsset(t *testing.T) {
	assets := updaterTestAssets()
	cases := []struct {
		platform, arch string
		want           string
	}{
		{"darwin", "arm64", "cmdex-darwin-universal.zip"},
		{"darwin", "amd64", "cmdex-darwin-universal.zip"},
		{"linux", "amd64", "cmdex-linux-amd64"},
		{"windows", "amd64", "cmdex-windows-amd64.exe"},
	}
	for _, tc := range cases {
		got := matchUpdaterAsset(updater.CheckRequest{Platform: tc.platform, Arch: tc.arch}, assets)
		if got < 0 || assets[got].Name != tc.want {
			name := "<no match>"
			if got >= 0 {
				name = assets[got].Name
			}
			t.Errorf("match(%s/%s) = %s, want %s", tc.platform, tc.arch, name, tc.want)
		}
	}
}

func TestMatchUpdaterAssetSkipsSidecars(t *testing.T) {
	assets := []github.ReleaseAsset{
		{Name: "cmdex-linux-amd64.sig"},
		{Name: "cmdex-linux-amd64"},
	}
	got := matchUpdaterAsset(updater.CheckRequest{Platform: "linux", Arch: "amd64"}, assets)
	if got != 1 {
		t.Errorf("match = %d, want 1 (sidecar .sig must be skipped)", got)
	}
}

func TestMatchUpdaterAssetNoMatch(t *testing.T) {
	got := matchUpdaterAsset(
		updater.CheckRequest{Platform: "freebsd", Arch: "riscv64"},
		updaterTestAssets(),
	)
	if got != -1 {
		t.Errorf("match = %d, want -1", got)
	}
}

func TestMatchUpdaterAssetPrefersExactArchOverUniversal(t *testing.T) {
	universalFirst := []github.ReleaseAsset{
		{Name: "cmdex-darwin-universal.zip"},
		{Name: "cmdex-darwin-arm64.zip"},
	}
	got := matchUpdaterAsset(updater.CheckRequest{Platform: "darwin", Arch: "arm64"}, universalFirst)
	if got != 1 || universalFirst[got].Name != "cmdex-darwin-arm64.zip" {
		name := "<no match>"
		if got >= 0 {
			name = universalFirst[got].Name
		}
		t.Errorf("universal-first match = %s, want cmdex-darwin-arm64.zip", name)
	}

	archFirst := []github.ReleaseAsset{
		{Name: "cmdex-darwin-arm64.zip"},
		{Name: "cmdex-darwin-universal.zip"},
	}
	got = matchUpdaterAsset(updater.CheckRequest{Platform: "darwin", Arch: "arm64"}, archFirst)
	if got != 0 {
		t.Errorf("arch-first match = %d, want 0", got)
	}
}

func TestUpdaterDisabledDevBuild(t *testing.T) {
	old := appVersion
	defer func() { appVersion = old }()

	appVersion = "dev"
	if !updaterDisabled() {
		t.Error("updaterDisabled() = false for \"dev\", want true")
	}
	appVersion = ""
	if !updaterDisabled() {
		t.Error("updaterDisabled() = false for \"\", want true")
	}
	appVersion = "0.4.0"
	if updaterDisabled() {
		t.Error("updaterDisabled() = true for \"0.4.0\", want false")
	}
	// Tags carry a "v" prefix; release.yml strips it before stamping. A
	// v-prefixed value must still enable the updater (it is neither "" nor
	// "dev") so a stamping slip fails loudly at the version comparison,
	// not by silently disabling updates.
	appVersion = "v0.4.0"
	if updaterDisabled() {
		t.Error("updaterDisabled() = true for \"v0.4.0\", want false")
	}
}

func TestAutoUpdateCheckSettingsRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	if err := db.runMigrations(); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	settings, err := db.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if settings.AutoUpdateCheck == nil || *settings.AutoUpdateCheck {
		t.Errorf("default AutoUpdateCheck = %v, want non-nil false", settings.AutoUpdateCheck)
	}

	enabled := true
	if err := db.SetSettings(AppSettings{AutoUpdateCheck: &enabled}); err != nil {
		t.Fatalf("SetSettings failed: %v", err)
	}
	// A partial write for an unrelated field must not clobber the flag.
	if err := db.SetSettings(AppSettings{Locale: "en"}); err != nil {
		t.Fatalf("SetSettings failed: %v", err)
	}
	settings, err = db.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if settings.AutoUpdateCheck == nil || !*settings.AutoUpdateCheck {
		t.Errorf("AutoUpdateCheck after partial write = %v, want non-nil true", settings.AutoUpdateCheck)
	}
}

func TestBetaChannelSettingsRoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	if err := db.runMigrations(); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	settings, err := db.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if settings.BetaChannel == nil || *settings.BetaChannel {
		t.Errorf("default BetaChannel = %v, want non-nil false", settings.BetaChannel)
	}
	if settings.LastUpdateCheck != nil {
		t.Errorf("default LastUpdateCheck = %v, want nil", *settings.LastUpdateCheck)
	}

	enabled := true
	if err := db.SetSettings(AppSettings{BetaChannel: &enabled}); err != nil {
		t.Fatalf("SetSettings failed: %v", err)
	}
	stamp := "2026-09-04T12:00:00Z"
	if err := db.SetSettings(AppSettings{LastUpdateCheck: &stamp}); err != nil {
		t.Fatalf("SetSettings failed: %v", err)
	}
	// A partial write for an unrelated field must not clobber either value.
	if err := db.SetSettings(AppSettings{Locale: "en"}); err != nil {
		t.Fatalf("SetSettings failed: %v", err)
	}
	settings, err = db.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if settings.BetaChannel == nil || !*settings.BetaChannel {
		t.Errorf("BetaChannel after partial write = %v, want non-nil true", settings.BetaChannel)
	}
	if settings.LastUpdateCheck == nil || *settings.LastUpdateCheck != stamp {
		t.Errorf("LastUpdateCheck after partial write = %v, want %q", settings.LastUpdateCheck, stamp)
	}
}

func TestChannelProviderSetBeta(t *testing.T) {
	p, err := newChannelProvider(false)
	if err != nil {
		t.Fatalf("newChannelProvider failed: %v", err)
	}
	if p.Name() != "github" {
		t.Errorf("Name() = %q, want github", p.Name())
	}
	if p.current() == nil {
		t.Fatal("inner provider is nil after construction")
	}
	first := p.current()
	if err := p.setBeta(true); err != nil {
		t.Fatalf("setBeta(true) failed: %v", err)
	}
	if p.current() == first {
		t.Error("setBeta(true) did not rebuild the inner provider")
	}
	if err := p.setBeta(false); err != nil {
		t.Fatalf("setBeta(false) failed: %v", err)
	}
	if p.current() == nil {
		t.Error("inner provider is nil after setBeta(false)")
	}
}

func TestGetAppInfoDevBuild(t *testing.T) {
	oldVersion, oldDB := appVersion, db
	defer func() { appVersion, db = oldVersion, oldDB }()

	appVersion = "dev"
	db = nil
	info := (&UpdateService{}).GetAppInfo()
	if info.UpdatesEnabled {
		t.Error("UpdatesEnabled = true for dev build, want false")
	}
	if info.Version != "dev" {
		t.Errorf("Version = %q, want dev", info.Version)
	}
	if info.Arch == "" {
		t.Error("Arch is empty, want runtime.GOARCH")
	}
	if info.State != string(updater.StateUnconfigured) {
		t.Errorf("State = %q, want unconfigured", info.State)
	}
}

// stubUpdaterHost satisfies updater.Host (updater.New is documented for test
// use) so GetAppInfo can run against a real Updater without an application.
type stubUpdaterHost struct{}

func (stubUpdaterHost) Emit(name string, data ...any) bool { return false }

func (stubUpdaterHost) OnEvent(name string, callback func(payload any)) func() {
	return func() {}
}

func (stubUpdaterHost) OpenWindow(opts updater.WindowOptions) updater.WindowHandle {
	return nil
}

func (stubUpdaterHost) Quit() {}

// TestGetAppInfoDoesNotBlockDuringCheck guards against the snapshot waiting
// on updateCheckMu: with a check in flight (mutex held for the whole remote
// check + download), About must still get its release-build snapshot instead
// of hanging on a null one rendered as a dev build.
func TestGetAppInfoDoesNotBlockDuringCheck(t *testing.T) {
	oldVersion, oldDB, oldApp := appVersion, db, wailsApp
	defer func() { appVersion, db, wailsApp = oldVersion, oldDB, oldApp }()

	appVersion = "0.4.0"
	db = nil
	wailsApp = &application.App{Updater: updater.New(stubUpdaterHost{})}

	updateCheckMu.Lock()
	defer updateCheckMu.Unlock()

	done := make(chan AppInfo, 1)
	go func() { done <- (&UpdateService{}).GetAppInfo() }()
	select {
	case info := <-done:
		if !info.UpdatesEnabled {
			t.Error("UpdatesEnabled = false while a check is in flight, want true")
		}
		if info.Version != "0.4.0" {
			t.Errorf("Version = %q while a check is in flight, want 0.4.0", info.Version)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GetAppInfo blocked while a check held updateCheckMu")
	}
}
