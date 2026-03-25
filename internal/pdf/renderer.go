package pdf

import (
	"bytes"
	"context"
	"fmt"
	"image/png"

	"github.com/gen2brain/go-fitz"
)

// RenderPage renders page 0 of a single-page PDF to PNG bytes in memory.
// Uses MuPDF via go-fitz (statically bundled). Resolution is 200 DPI —
// sufficient for printed text OCR and ~2x faster than 300 DPI.
// PNG (lossless) is used instead of JPEG to avoid compression artefacts
// on text edges, which hurt Tesseract accuracy.
func RenderPage(_ context.Context, pagePDFPath string) ([]byte, error) {
	doc, err := fitz.New(pagePDFPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %q: %w", pagePDFPath, err)
	}
	defer doc.Close()

	img, err := doc.ImageDPI(0, 200)
	if err != nil {
		return nil, fmt.Errorf("failed to render page 0 of %q: %w", pagePDFPath, err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("failed to encode PNG: %w", err)
	}

	return buf.Bytes(), nil
}
