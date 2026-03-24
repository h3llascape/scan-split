package pdf

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/gen2brain/go-fitz"
)

// RenderPage renders page 0 of a single-page PDF to a PNG file and returns
// the path to the created image. Uses MuPDF via go-fitz (statically bundled,
// no system installation required). Resolution is 300 DPI for good OCR quality.
func RenderPage(_ context.Context, pagePDFPath, imageDir string) (string, error) {
	doc, err := fitz.New(pagePDFPath)
	if err != nil {
		return "", fmt.Errorf("failed to open %q: %w", pagePDFPath, err)
	}
	defer doc.Close()

	img, err := doc.ImageDPI(0, 300)
	if err != nil {
		return "", fmt.Errorf("failed to render page 0 of %q: %w", pagePDFPath, err)
	}

	stem := strings.TrimSuffix(filepath.Base(pagePDFPath), ".pdf")
	outPath := filepath.Join(imageDir, stem+".png")

	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("failed to create image file %q: %w", outPath, err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return "", fmt.Errorf("failed to encode PNG: %w", err)
	}

	return outPath, nil
}
