package main

import (
	"embed"
	"log"

	"displaycontrol/internal/display"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	err := wails.Run(&options.App{
		Title:     "屏幕管理工具",
		Width:     850,
		Height:    620,
		MinWidth:  480,
		MinHeight: 430,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: options.NewRGB(244, 246, 243),
		Bind:             []interface{}{display.New()},
	})
	if err != nil {
		log.Fatal(err)
	}
}
