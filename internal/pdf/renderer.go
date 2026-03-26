package pdf

import (
	"bytes"
	"fmt"
	"image/png"

	"github.com/gen2brain/go-fitz"
)

// Document wraps a go-fitz document handle for rendering multiple pages
// without re-opening and re-parsing the PDF each time.
// Not thread-safe — each goroutine must use its own Document.
type Document struct {
	doc *fitz.Document
}

// OpenDocument opens a PDF for rendering. Call Close when done.
func OpenDocument(pdfPath string) (*Document, error) {
	doc, err := fitz.New(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %q: %w", pdfPath, err)
	}
	return &Document{doc: doc}, nil
}

// Close releases the underlying MuPDF resources.
func (d *Document) Close() {
	d.doc.Close() //nolint:errcheck // best-effort cleanup
}

var pngEncoder = png.Encoder{CompressionLevel: png.BestSpeed}

// RenderPage renders the given 0-based page to PNG bytes in memory.
// Uses BestSpeed compression: ~2x faster encode than default, compact
// output (~3MB vs ~12MB raw) — important because gosseract writes image
// bytes to a temp file before OCR.
// Resolution is 300 DPI — higher than 200 DPI noticeably improves Tesseract
// accuracy on scanned (as opposed to born-digital) PDFs.
func (d *Document) RenderPage(pageIndex int) ([]byte, error) {
	img, err := d.doc.ImageDPI(pageIndex, 300)
	if err != nil {
		return nil, fmt.Errorf("failed to render page %d: %w", pageIndex, err)
	}

	var buf bytes.Buffer
	if err := pngEncoder.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("failed to encode PNG: %w", err)
	}

	return buf.Bytes(), nil
}
