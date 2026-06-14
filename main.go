package main

import (
	"embed"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:web/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Porque Minecraft Server Manager",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/servers/") && strings.HasSuffix(r.URL.Path, "/icon") {
					parts := strings.Split(r.URL.Path, "/")
					if len(parts) >= 4 {
						id, err := uuid.Parse(parts[3])
						if err == nil && app.life != nil {
							data, err := app.life.GetIcon(r.Context(), id)
							if err == nil {
								w.Header().Set("Content-Type", "image/png")
								w.Header().Set("Cache-Control", "no-cache")
								w.Write(data)
								return
							}
						}
					}
				}
				http.NotFound(w, r)
			}),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 59, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
