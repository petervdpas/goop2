// main.go
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Goop²  ·  ephemeral web",
		Width:  1200,
		Height: 800,

		AssetServer: &assetserver.Options{
			Assets: assets,
		},

		// Linux runtime window icon (closest to Electron behaviour)
		Linux: &linux.Options{
			Icon: appIcon,
		},

		OnStartup: app.startup,
		Bind:      []any{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
