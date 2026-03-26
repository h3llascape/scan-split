// cmd/debug is a CLI tool for diagnosing grouping issues.
// Usage: go run ./cmd/debug <input.pdf>
//
// For every page it prints:
//   - the raw OCR text
//   - the extracted name / group / confidence
//   - the grouping decision (which student bucket the page ends up in)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/h3llascape/scan-split/internal/models"
	"github.com/h3llascape/scan-split/internal/ocr"
	"github.com/h3llascape/scan-split/internal/pdf"
	"github.com/h3llascape/scan-split/internal/pipeline"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/debug <input.pdf>")
		os.Exit(1)
	}
	inputPath := os.Args[1]

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ocrProvider, err := ocr.NewTesseractProvider(logger, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tesseract init: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pageCount, err := pdf.PageCount(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "page count: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("PDF: %s\n", inputPath)
	fmt.Printf("Total pages: %d\n\n", pageCount)

	doc, err := pdf.OpenDocument(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open PDF: %v\n", err)
		os.Exit(1)
	}
	defer doc.Close()

	var parsedPages []models.ParsedPage

	for i := range pageCount {
		pg := models.Page{Number: i + 1, SourcePath: inputPath}

		imgData, renderErr := doc.RenderPage(i)
		if renderErr != nil {
			fmt.Printf("Page %2d: RENDER ERROR: %v\n\n", pg.Number, renderErr)
			parsedPages = append(parsedPages, models.ParsedPage{Page: pg, IsOrphan: true})
			continue
		}

		text, ocrErr := ocrProvider.RecognizeText(ctx, imgData)
		if ocrErr != nil {
			fmt.Printf("Page %2d: OCR ERROR: %v\n\n", pg.Number, ocrErr)
			parsedPages = append(parsedPages, models.ParsedPage{Page: pg, IsOrphan: true})
			continue
		}

		res := ocr.Parse(text)

		fmt.Printf("── Page %2d ─────────────────────────────────────────────\n", pg.Number)
		fmt.Printf("  Name:       %q\n", res.FullName)
		if len(res.AllNames) > 1 {
			fmt.Printf("  AllNames:   %q\n", res.AllNames)
		}
		fmt.Printf("  Group:      %q\n", res.Group)
		fmt.Printf("  Confidence: %.1f\n", res.Confidence)
		fmt.Printf("  OCR text (first 400 chars):\n%s\n\n",
			indent(truncate(text, 400), "    "))

		pg.OCRText = text
		parsedPages = append(parsedPages, models.ParsedPage{
			Page:       pg,
			FullName:   res.FullName,
			Group:      res.Group,
			Confidence: res.Confidence,
			IsOrphan:   res.Confidence == 0,
		})
	}

	fmt.Println("══ GROUPING (real pipeline) ══════════════════════════════")

	p := pipeline.New(pipeline.Config{}, nil, logger)
	students, orphans := p.GroupPages(parsedPages)

	fmt.Printf("Students: %d, Orphan pages: %d\n\n", len(students), len(orphans))
	for _, s := range students {
		pageNums := make([]int, len(s.Pages))
		for i, p := range s.Pages {
			pageNums[i] = p.Number
		}
		fmt.Printf("  %-40s [%s] → pages %v\n", s.FullName, s.Group, pageNums)
	}
	if len(orphans) > 0 {
		fmt.Println("\nOrphan pages (no student identified at all):")
		for _, o := range orphans {
			fmt.Printf("  page %d\n", o.Page.Number)
		}
	}
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(prefix)
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}
