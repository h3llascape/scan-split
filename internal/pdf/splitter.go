// Package pdf wraps pdfcpu operations used by the pipeline.
package pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hellascape/scansplit/internal/models"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// pageNumRe extracts the trailing integer from pdfcpu split output names,
// e.g. "document_3.pdf" → "3", "document_12.pdf" → "12".
var pageNumRe = regexp.MustCompile(`_(\d+)\.pdf$`)

// SplitPages splits inputPath into individual single-page PDFs inside destDir.
// Returns a slice of Page values with Number and PDFPath filled in,
// sorted in correct numeric page order (not lexicographic).
func SplitPages(_ context.Context, inputPath, destDir string) ([]models.Page, error) {
	conf := pdfmodel.NewDefaultConfiguration()
	conf.ValidationMode = pdfmodel.ValidationRelaxed

	if err := api.SplitFile(inputPath, destDir, 1, conf); err != nil {
		return nil, fmt.Errorf("failed to split %q: %w", inputPath, err)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read split dir %q: %w", destDir, err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			names = append(names, e.Name())
		}
	}

	// Natural numeric sort: "doc_2.pdf" < "doc_10.pdf" (not "doc_10" < "doc_2").
	sort.Slice(names, func(i, j int) bool {
		return pageNumLess(names[i], names[j])
	})

	if len(names) == 0 {
		return nil, fmt.Errorf("no pages produced after splitting %q", inputPath)
	}

	pages := make([]models.Page, len(names))
	for i, name := range names {
		pages[i] = models.Page{
			Number:  i + 1,
			PDFPath: filepath.Join(destDir, name),
		}
	}

	return pages, nil
}

// pageNumLess reports whether file name a should sort before b by their
// trailing page number. Falls back to lexicographic order if no number found.
func pageNumLess(a, b string) bool {
	ma := pageNumRe.FindStringSubmatch(a)
	mb := pageNumRe.FindStringSubmatch(b)
	if len(ma) < 2 || len(mb) < 2 {
		return a < b
	}
	na, err1 := strconv.Atoi(ma[1])
	nb, err2 := strconv.Atoi(mb[1])
	if err1 != nil || err2 != nil {
		return a < b
	}
	return na < nb
}
