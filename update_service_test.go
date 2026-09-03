package main

import (
	"strings"
	"testing"

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
	// Tags carry a "v" prefix; release.yml strips it before stamping, so a
	// stamped value must never retain it.
	appVersion = "v0.4.0"
	if !strings.HasPrefix(appVersion, "v") {
		t.Error("test setup broken: expected v prefix")
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
