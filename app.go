package main

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

var (
	db          *DB
	executor    *Executor
	wailsApp    *application.App
	terminalSvc *TerminalService
	launcherSvc *LauncherService
)

const (
	settingsWindowWidth  = 640
	settingsWindowHeight = 520
	settingsWindowBgR    = 15
	settingsWindowBgG    = 15
	settingsWindowBgB    = 20
	settingsWindowBgA    = 255
)

// App handles application lifecycle and settings window management.
type App struct {
	settingsWindowMu sync.Mutex
	settingsWindow   *application.WebviewWindow
}

// ServiceStartup initializes the Wails application, database, and executor.
// Returns an error if database initialization fails.
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	wailsApp = application.Get()
	var err error
	db, err = NewDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	executor = NewExecutor()
	return nil
}

// ServiceShutdown closes the database connection if non-nil.
func (a *App) ServiceShutdown() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// ShowSettingsWindow opens the settings window, creating it if needed.
func (a *App) ShowSettingsWindow() {
	a.settingsWindowMu.Lock()
	if a.settingsWindow == nil {
		a.createSettingsWindowLocked()
	}
	w := a.settingsWindow
	a.settingsWindowMu.Unlock()

	if w != nil {
		w.Show()
		w.Focus()
	}
}

// GetOS returns the current operating system identifier (darwin, windows, or linux).
// Used by the frontend to read/write OS-specific paths in OSPathMap.
func (a *App) GetOS() string {
	return runtime.GOOS
}

// PickDirectory opens a native directory picker dialog and returns the selected path.
// Returns an empty string if the user cancels the dialog.
func (a *App) PickDirectory(currentPath string) (string, error) {
	dialog := wailsApp.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false)

	if currentPath != "" {
		dialog.SetDirectory(currentPath)
	}

	result, err := dialog.PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	return result, nil
}

// createSettingsWindowLocked creates the settings window. Caller must hold settingsWindowMu.
func (a *App) createSettingsWindowLocked() {
	if a.settingsWindow != nil {
		return
	}

	windowOptions := application.WebviewWindowOptions{
		Title:              "Settings",
		UseApplicationMenu: false,
		BackgroundColour: application.NewRGBA(
			settingsWindowBgR,
			settingsWindowBgG,
			settingsWindowBgB,
			settingsWindowBgA,
		),
		HideOnEscape:        true,
		DisableResize:       true,
		Width:               settingsWindowWidth,
		Height:              settingsWindowHeight,
		MinimiseButtonState: application.ButtonDisabled,
		MaximiseButtonState: application.ButtonDisabled,
		URL:                 "/?window=settings",
		Name:                "settings",
		InitialPosition:     application.WindowCentered,
	}

	sw := wailsApp.Window.NewWithOptions(windowOptions)

	sw.OnWindowEvent(events.Common.WindowClosing, func(event *application.WindowEvent) {
		a.settingsWindowMu.Lock()
		a.settingsWindow = nil
		a.settingsWindowMu.Unlock()

		wailsApp.Event.Emit(eventNames.SettingsWindowClosing)
	})

	a.settingsWindow = sw
}
