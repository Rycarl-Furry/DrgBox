package main

import (
	"embed"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:             "DRGBOX · 本地分类工具箱",
		Width:             1440,
		Height:            900,
		Frameless:         true,
		HideWindowOnClose: hideWindowOnClose(),
		AssetServer: &assetserver.Options{
			Assets: assets,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/wallpaper" {
					app.ServeWallpaper(w, r)
					return
				}
				if r.URL.Path == "/avatar" {
					app.ServeAvatar(w, r)
					return
				}
				if r.URL.Path == "/tool-icon" {
					app.ServeToolIcon(w, r)
					return
				}
				http.NotFound(w, r)
			}),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
