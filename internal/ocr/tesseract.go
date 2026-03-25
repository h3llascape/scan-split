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
// A pool of gosseract clients is created at init (one per concurrency worker),
// avoiding re-initialisation on every page while keeping calls parallel.
type TesseractProvider struct {
	logger *slog.Logger
	pool   chan *gosseract.Client
}

// NewTesseractProvider creates a TesseractProvider and extracts the embedded
// tessdata to os.UserCacheDir()/scansplit/tessdata if not already present.
// poolSize controls how many Tesseract clients are created (should match pipeline concurrency).
// Returns an error if tessdata was not embedded at build time (see Makefile: make tessdata).
func NewTesseractProvider(logger *slog.Logger, poolSize int) (*TesseractProvider, error) {
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

	if poolSize <= 0 {
		poolSize = 1
	}
	pool := make(chan *gosseract.Client, poolSize)
	for range poolSize {
		c := gosseract.NewClient()
		c.SetTessdataPrefix(tessdataDir) //nolint:errcheck // no meaningful recovery
		if err := c.SetLanguage("rus"); err != nil {
			c.Close() //nolint:errcheck // best-effort cleanup
			return nil, fmt.Errorf("tesseract init failed (language 'rus'): %w", err)
		}
		pool <- c
	}

	logger.Info("tesseract provider ready", "tessdata", tessdataDir, "pool", poolSize)
	return &TesseractProvider{
		logger: logger,
		pool:   pool,
	}, nil
}

// RecognizeText runs Tesseract OCR on the supplied PNG/JPEG image bytes.
// A client is borrowed from the pool for the duration of the call and returned
// afterwards, so up to poolSize calls can run in parallel.
func (t *TesseractProvider) RecognizeText(ctx context.Context, imageData []byte) (string, error) {
	var client *gosseract.Client
	select {
	case client = <-t.pool:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { t.pool <- client }()

	if err := client.SetImageFromBytes(imageData); err != nil {
		return "", fmt.Errorf("set image failed: %w", err)
	}

	text, err := client.Text()
	if err != nil {
		return "", fmt.Errorf("tesseract OCR failed: %w", err)
	}

	return text, nil
}
