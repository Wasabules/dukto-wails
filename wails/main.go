package main

import (
	"embed"
	"net/http"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "dukto",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Fall-through HTTP handler: Wails only routes requests here when
			// the embedded SPA bundle has no matching asset. We use it to
			// serve files under the destination directory so the frontend
			// can render <img>/<video>/<audio> previews of received content.
			Handler: http.HandlerFunc(app.serveFile),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.onBeforeClose,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			// Arbitrary but stable UUID: the lock's job is to match sibling
			// instances, so the value must not change across releases.
			UniqueId: "f2a9c1b4-3d5e-4f0a-9b7c-6e1d8a2c3b4f-dukto",
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				wailsruntime.WindowShow(app.ctx)
				wailsruntime.WindowUnminimise(app.ctx)
			},
		},
		Menu: buildMenu(),
		Bind: []any{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// buildMenu returns the application menu. On macOS the standard App + Edit
// menus are mandatory for keyboard shortcuts (⌘C/⌘V/⌘Q) to work. On Windows
// and Linux a *menu.Menu attaches to the window frame — we skip it there so
// the frontend's title bar stays clean.
func buildMenu() *menu.Menu {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return menu.NewMenuFromItems(
		menu.AppMenu(),
		menu.EditMenu(),
		menu.WindowMenu(),
	)
}
