package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// SettingsService handles user preferences and settings window.
type SettingsService struct{}

// ServiceStartup implements the Wails v3 service lifecycle hook for startup.
// Currently a no-op as settings are initialized on-demand.
func (s *SettingsService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// GetSettings returns the current app settings.
func (s *SettingsService) GetSettings() AppSettings {
	settings, err := db.GetSettings()
	if err != nil {
		fmt.Println("GetSettings error:", err)
		return AppSettings{Locale: "en"}
	}
	return settings
}

// SetSettings updates the app settings from a JSON string.
func (s *SettingsService) SetSettings(jsonStr string) error {
	var settings AppSettings
	if err := json.Unmarshal([]byte(jsonStr), &settings); err != nil {
		return fmt.Errorf("invalid settings JSON: %w", err)
	}
	return db.SetSettings(settings)
}
