// Package app contains the Wails application struct bound to the frontend.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	goruntime "runtime"
	"strings"
	"sync"

	"github.com/hellascape/scansplit/internal/models"
	"github.com/hellascape/scansplit/internal/ocr"
	"github.com/hellascape/scansplit/internal/pipeline"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main application struct exposed to the Wails frontend.
type App struct {
	ctx         context.Context
	logger      *slog.Logger
	pipeline    *pipeline.Pipeline
	ocrProvider string // human-readable name shown in UI

	mu         sync.Mutex
	cancelFunc context.CancelFunc
}

// New creates a new App. It tries to initialise Tesseract; if unavailable
// it falls back to the mock provider so the app remains usable for development.
func New(logger *slog.Logger) *App {
	var provider ocr.Provider
	var providerName string

	const concurrency = 4

	tp, err := ocr.NewTesseractProvider(logger, concurrency)
	if err != nil {
		logger.Warn("tesseract unavailable, using mock OCR", "err", err)
		provider = ocr.NewMockProvider(logger)
		providerName = "Mock (тест)"
	} else {
		provider = tp
		providerName = "Tesseract"
	}

	p := pipeline.New(pipeline.Config{Concurrency: concurrency}, provider, logger)
	return &App{
		logger:      logger,
		pipeline:    p,
		ocrProvider: providerName,
	}
}

// Startup is called by Wails when the application starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.logger.Info("application started", "ocr", a.ocrProvider)

	// Register native file-drop handler. When the user drags a file onto the
	// window, Wails emits paths via OnFileDrop (WebKit doesn't expose file.path
	// in DataTransfer, so the standard HTML5 DnD API can't provide the full path).
	wailsruntime.OnFileDrop(ctx, func(x, y int, paths []string) {
		for _, p := range paths {
			if strings.HasSuffix(strings.ToLower(p), ".pdf") {
				wailsruntime.EventsEmit(ctx, "file:drop", p)
				return
			}
		}
		if len(paths) > 0 {
			wailsruntime.EventsEmit(ctx, "file:drop:error", "Поддерживаются только PDF-файлы")
		}
	})
}

// Shutdown is called by Wails when the application is closing.
func (a *App) Shutdown(_ context.Context) {
	a.logger.Info("application shutting down")
	a.mu.Lock()
	if a.cancelFunc != nil {
		a.cancelFunc()
	}
	a.mu.Unlock()
}

// GetOCRProvider returns the name of the active OCR provider for display in the UI.
func (a *App) GetOCRProvider() string {
	return a.ocrProvider
}

// SelectInputFile opens a native file-picker dialog and returns the chosen PDF path.
func (a *App) SelectInputFile() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Выберите PDF-файл",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "PDF Files (*.pdf)", Pattern: "*.pdf"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("file dialog failed: %w", err)
	}
	return path, nil
}

// SelectOutputDir opens a native directory-picker dialog and returns the chosen path.
func (a *App) SelectOutputDir() (string, error) {
	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Выберите папку для результатов",
	})
	if err != nil {
		return "", fmt.Errorf("directory dialog failed: %w", err)
	}
	return path, nil
}

// ProcessFile starts the processing pipeline asynchronously.
// Progress is reported via Wails events:
//   - "processing:progress" → models.ProcessingProgress
//   - "processing:complete" → models.ProcessingResult
//   - "processing:error"    → string
func (a *App) ProcessFile(inputPath, outputDir string) error {
	a.mu.Lock()
	if a.cancelFunc != nil {
		a.cancelFunc()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelFunc = cancel
	a.mu.Unlock()

	go func() {
		result, err := a.pipeline.Run(ctx, inputPath, outputDir, func(p models.ProcessingProgress) {
			wailsruntime.EventsEmit(a.ctx, "processing:progress", p)
		})
		if err != nil {
			a.logger.Error("pipeline error", "err", err)
			wailsruntime.EventsEmit(a.ctx, "processing:error", err.Error())
			return
		}
		wailsruntime.EventsEmit(a.ctx, "processing:complete", result)
	}()

	return nil
}

// CancelProcessing cancels any ongoing processing.
func (a *App) CancelProcessing() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancelFunc != nil {
		a.cancelFunc()
		a.cancelFunc = nil
	}
	return nil
}

// OpenResultsFolder opens dir in the OS file manager (Explorer / Finder / Nautilus).
func (a *App) OpenResultsFolder(dir string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open folder %q: %w", dir, err)
	}
	return nil
}
