package ocr

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/otiai10/gosseract/v2"
)

//go:embed tessdata
var tessdataFS embed.FS

// TesseractProvider implements Provider using the local Tesseract OCR engine.
// The Russian language model (rus.traineddata) is embedded at build time and
// extracted to the OS user-cache directory on first use — no external files needed.
type TesseractProvider struct {
	logger      *slog.Logger
	tessdataDir string // path to the extracted tessdata directory
}

// NewTesseractProvider creates a TesseractProvider and extracts the embedded
// tessdata to os.UserCacheDir()/scansplit/tessdata if not already present.
// Returns an error if tessdata was not embedded at build time (see Makefile: make tessdata).
func NewTesseractProvider(logger *slog.Logger) (*TesseractProvider, error) {
	data, err := tessdataFS.ReadFile("tessdata/rus.traineddata")
	if err != nil {
		return nil, fmt.Errorf(
			"tessdata/rus.traineddata is not embedded — run 'make tessdata' before building: %w", err,
		)
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to locate user cache dir: %w", err)
	}

	tessdataDir := filepath.Join(cacheDir, "scansplit", "tessdata")
	if err := os.MkdirAll(tessdataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create tessdata dir %q: %w", tessdataDir, err)
	}

	dest := filepath.Join(tessdataDir, "rus.traineddata")

	// Only write if not already present (avoid re-writing on every startup).
	if info, statErr := os.Stat(dest); statErr != nil || info.Size() != int64(len(data)) {
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return nil, fmt.Errorf("failed to write tessdata to %q: %w", dest, err)
		}
		logger.Debug("tessdata extracted", "path", dest, "bytes", len(data))
	}

	// Smoke-test: verify tesseract can initialise.
	client := gosseract.NewClient()
	client.SetTessdataPrefix(tessdataDir)
	if err := client.SetLanguage("rus"); err != nil {
		client.Close()
		return nil, fmt.Errorf("tesseract init failed (language 'rus'): %w", err)
	}
	client.Close()

	logger.Info("tesseract provider ready", "tessdata", tessdataDir)
	return &TesseractProvider{
		logger:      logger,
		tessdataDir: tessdataDir,
	}, nil
}

// RecognizeText runs Tesseract OCR on the supplied PNG/JPEG image bytes.
// A fresh client is created per call so the provider is safe for concurrent use.
func (t *TesseractProvider) RecognizeText(_ context.Context, imageData []byte) (string, error) {
	client := gosseract.NewClient()
	defer client.Close()

	client.SetTessdataPrefix(t.tessdataDir)
	if err := client.SetLanguage("rus"); err != nil {
		return "", fmt.Errorf("set language failed: %w", err)
	}

	if err := client.SetImageFromBytes(imageData); err != nil {
		return "", fmt.Errorf("set image failed: %w", err)
	}

	text, err := client.Text()
	if err != nil {
		return "", fmt.Errorf("tesseract OCR failed: %w", err)
	}

	return text, nil
}
