package main

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// EventNames holds all Wails event name constants, exposed to frontend via GetEventNames().
type EventNames struct {
	OpenSettings          string `json:"openSettings"`
	OpenShortcuts         string `json:"openShortcuts"`
	SettingsChanged       string `json:"settingsChanged"`
	SettingsWindowClosing string `json:"settingsWindowClosing"`
	DataReset             string `json:"dataReset"`
	// LauncherShown fires each time the quick launcher is revealed, so its UI
	// can refocus the search field and select any existing query text.
	LauncherShown  string `json:"launcherShown"`
	LauncherHidden string `json:"launcherHidden"`
}

var eventNames = EventNames{
	OpenSettings:          "open-settings",
	OpenShortcuts:         "open-shortcuts",
	SettingsChanged:       "settings-changed",
	SettingsWindowClosing: "settings-window-closing",
	DataReset:             "data-reset",
	LauncherShown:         "launcher-shown",
	LauncherHidden:        "launcher-hidden",
}

// EventService exposes event name constants to the frontend.
type EventService struct{}

func (s *EventService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// GetEventNames returns all event name constants so the frontend can use
// them via Wails bindings instead of hardcoded strings.
func (s *EventService) GetEventNames() EventNames {
	return eventNames
}
