package main

import (
	"embed"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

const (
	mainWindowWidth                            = 1200
	mainWindowHeight                           = 800
	mainWindowMinWidth                         = 900
	mainWindowMinHeight                        = 600
	windowBgR, windowBgG, windowBgB, windowBgA = 15, 15, 20, 255
)

func main() {
	appService := &App{}

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
	cmdexMenu.AddRole(application.About)
	cmdexMenu.AddSeparator()
	cmdexMenu.Add("Settings...").SetAccelerator("CmdOrCtrl+,").OnClick(func(ctx *application.Context) {
		appService.ShowSettingsWindow()
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
		Width:              mainWindowWidth,
		Height:             mainWindowHeight,
		MinWidth:           mainWindowMinWidth,
		MinHeight:          mainWindowMinHeight,
		UseApplicationMenu: true,
		BackgroundColour:   application.NewRGBA(windowBgR, windowBgG, windowBgB, windowBgA),
	})

	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		app.Quit()
	})

	if err := app.Run(); err != nil {
		println("Error:", err.Error())
	}
}
