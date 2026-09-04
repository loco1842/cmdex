package main

import (
	"embed"
	"os"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// appVersion is the release version without a "v" prefix (e.g. "0.4.0").
// It is injected at build time via -ldflags "-X main.appVersion=..." (see the
// per-OS Taskfiles and release.yml). Local builds keep the "dev" default, in
// which case the in-app updater stays unconfigured (see updaterDisabled).
var appVersion = "dev"

// updaterGitHubRepo is the owner/repo the updater polls for releases.
const updaterGitHubRepo = "loco1842/cmdex"

//go:embed all:frontend/dist
var assets embed.FS

// mainWindowName identifies the primary window so other services can find it.
const mainWindowName = "main"

const (
	mainWindowWidth                            = 1200
	mainWindowHeight                           = 800
	mainWindowMinWidth                         = 900
	mainWindowMinHeight                        = 600
	windowBgR, windowBgG, windowBgB, windowBgA = 15, 15, 20, 255
)

func main() {
	appService := &App{}
	startHidden := slices.Contains(os.Args[1:], backgroundFlag)

	app := application.New(application.Options{
		Name: "CmDex",
		Services: []application.Service{
			application.NewService(appService),
			application.NewService(&TerminalService{}),
			application.NewService(&CommandService{}),
			application.NewService(&ExecutionService{}),
			application.NewService(&SettingsService{}),
			application.NewService(&ImportExportService{}),
			application.NewService(&EventService{}),
			application.NewService(&LauncherService{}),
			application.NewService(&UpdateService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	menu := app.NewMenu()

	cmdexMenu := menu.AddSubmenu("CmDex")
	// Custom About (not the role default): the in-app About dialog doubles as
	// the update UI, mirroring GitHub Desktop.
	cmdexMenu.Add("About CmDex").OnClick(func(ctx *application.Context) {
		wailsApp.Event.Emit(eventNames.OpenAbout)
	})
	cmdexMenu.AddSeparator()
	cmdexMenu.Add("Settings...").SetAccelerator("CmdOrCtrl+,").OnClick(func(ctx *application.Context) {
		appService.ShowSettingsWindow()
	})
	cmdexMenu.Add("Check for Updates...").OnClick(func(ctx *application.Context) {
		checkForUpdatesFromMenu()
	})
	cmdexMenu.AddSeparator()
	cmdexMenu.AddRole(application.Hide)
	cmdexMenu.AddRole(application.HideOthers)
	cmdexMenu.AddSeparator()
	cmdexMenu.AddRole(application.Reload)
	cmdexMenu.AddRole(application.Quit)

	menu.AddRole(application.EditMenu)

	helpMenu := menu.AddSubmenu("Help")
	helpMenu.Add("Keyboard Shortcuts...").SetAccelerator("CmdOrCtrl+?").OnClick(func(ctx *application.Context) {
		wailsApp.Event.Emit(eventNames.OpenShortcuts)
	})
	helpMenu.Add("Open Dev Tools").OnClick(func(ctx *application.Context) {
		w := app.Window.Current()
		if w != nil {
			w.OpenDevTools()
		}
	})

	app.Menu.Set(menu)

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:              "CmDex",
		Name:               mainWindowName,
		Width:              mainWindowWidth,
		Height:             mainWindowHeight,
		MinWidth:           mainWindowMinWidth,
		MinHeight:          mainWindowMinHeight,
		Hidden:             startHidden,
		UseApplicationMenu: true,
		BackgroundColour:   application.NewRGBA(windowBgR, windowBgG, windowBgB, windowBgA),
	})

	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		app.Quit()
	})

	initUpdater(app)

	if err := app.Run(); err != nil {
		println("Error:", err.Error())
	}
}
