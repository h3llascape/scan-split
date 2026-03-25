// Package pdf wraps PDF operations used by the pipeline.
package pdf

import (
	"fmt"

	"github.com/gen2brain/go-fitz"
)

// PageCount returns the number of pages in the given PDF file.
func PageCount(pdfPath string) (int, error) {
	doc, err := fitz.New(pdfPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open %q: %w", pdfPath, err)
	}
	defer doc.Close()
	return doc.NumPage(), nil
}
