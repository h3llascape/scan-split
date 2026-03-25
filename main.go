package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/h3llascape/scan-split/cmd/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	a := app.New(logger)

	if err := wails.Run(&options.App{
		Title:     "ScanSplit",
		Width:     800,
		Height:    600,
		MinWidth:  700,
		MinHeight: 500,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 18, B: 18, A: 1},
		OnStartup:        a.Startup,
		OnShutdown:       a.Shutdown,
		Bind:             []any{a},
		// EnableFileDrop activates runtime.OnFileDrop so we get the full native
		// file path when the user drags a PDF onto the window.
		// DisableWebViewDrop is intentionally NOT set: the frontend's blockNativeDrop
		// already calls preventDefault() on drop, preventing WebView from opening
		// the file as a document. Leaving WebView drop enabled allows HTML5
		// dragover/dragleave events to fire so the drop zone highlights correctly.
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
	}); err != nil {
		logger.Error("application error", "err", err)
		os.Exit(1)
	}
}
