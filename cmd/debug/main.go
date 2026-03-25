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

	"github.com/hellascape/scansplit/internal/models"
	"github.com/hellascape/scansplit/internal/ocr"
	"github.com/hellascape/scansplit/internal/pdf"
	"github.com/hellascape/scansplit/internal/pipeline"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/debug <input.pdf>")
		os.Exit(1)
	}
	inputPath := os.Args[1]

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// ── Init OCR ──────────────────────────────────────────────────────────────
	ocrProvider, err := ocr.NewTesseractProvider(logger, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tesseract init: %v\n", err)
		os.Exit(1)
	}

	// ── Split ─────────────────────────────────────────────────────────────────
	tmpDir, err := os.MkdirTemp("", "scansplit-debug-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mktemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	splitDir := tmpDir + "/split"
	if err := os.MkdirAll(splitDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pages, err := pdf.SplitPages(ctx, inputPath, splitDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "split: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("PDF: %s\n", inputPath)
	fmt.Printf("Total pages: %d\n\n", len(pages))

	// ── Per-page OCR + parse ──────────────────────────────────────────────────
	var parsedPages []models.ParsedPage

	for _, pg := range pages {
		imgData, renderErr := pdf.RenderPage(ctx, pg.PDFPath)
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

	// ── Grouping using the real pipeline logic ────────────────────────────────
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

// ── helpers ───────────────────────────────────────────────────────────────────

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
