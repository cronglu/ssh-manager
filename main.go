package main

import (
	"embed"
	"ssh-manager-wails/logger"

	"go.uber.org/zap"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Initialize logger
	cfg := &logger.Config{
		LogPath: "ssh-manager.log",
		Debug:   true,
		Console: true,
	}
	logger.Init(cfg)
	defer zap.L().Sync() // Flushes buffer, if any

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "ssh-manager",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		zap.L().Error("Application error", zap.Error(err))
	}
}
