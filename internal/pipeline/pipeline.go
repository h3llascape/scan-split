// Package pipeline orchestrates the full PDF processing workflow:
// render → OCR → parse → group → extract & save.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/h3llascape/scan-split/internal/models"
	"github.com/h3llascape/scan-split/internal/ocr"
	"github.com/h3llascape/scan-split/internal/pdf"
)

// Config holds tunable pipeline parameters.
type Config struct {
	// Concurrency is the number of parallel OCR workers. Defaults to 4.
	Concurrency int
	// Whitelist, if non-empty, restricts which OCR-detected names create
	// student anchors. Names not matching any entry are treated as unnamed.
	Whitelist []string
}

// ProgressCallback is invoked on each progress update from the pipeline.
type ProgressCallback func(models.ProcessingProgress)

// Pipeline orchestrates the full processing workflow.
type Pipeline struct {
	cfg    Config
	ocr    ocr.Provider
	logger *slog.Logger
}

// New creates a new Pipeline. cfg.Concurrency defaults to 4 if ≤ 0.
func New(cfg Config, ocrProvider ocr.Provider, logger *slog.Logger) *Pipeline {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	return &Pipeline{
		cfg:    cfg,
		ocr:    ocrProvider,
		logger: logger,
	}
}

// SetWhitelist updates the student name whitelist for the next Run.
func (p *Pipeline) SetWhitelist(names []string) {
	p.cfg.Whitelist = names
}

// GroupPages exposes groupByStudent for the debug CLI.
func (p *Pipeline) GroupPages(pages []models.ParsedPage) ([]models.Student, []models.ParsedPage) {
	return p.groupByStudent(pages)
}

// Run executes the full pipeline and returns the processing result.
// progress is called on every meaningful state change and may be nil.
func (p *Pipeline) Run(
	ctx context.Context,
	inputPath, outputDir string,
	progress ProgressCallback,
) (*models.ProcessingResult, error) {
	if progress == nil {
		progress = func(models.ProcessingProgress) {}
	}

	progress(models.ProcessingProgress{
		Stage:       "counting",
		Current:     0,
		Total:       1,
		Description: "Чтение PDF…",
	})

	pageCount, err := pdf.PageCount(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}
	if pageCount == 0 {
		return nil, fmt.Errorf("PDF has no pages: %s", inputPath)
	}

	pages := make([]models.Page, pageCount)
	for i := range pages {
		pages[i] = models.Page{Number: i + 1, SourcePath: inputPath}
	}
	p.logger.Info("PDF loaded", "pages", pageCount)

	parsedPages, errs, avgPageMs := p.renderAndOCR(ctx, pages, progress)

	result := &models.ProcessingResult{Errors: errs, AvgPageMs: avgPageMs}

	progress(models.ProcessingProgress{
		Stage:       "grouping",
		Current:     0,
		Total:       1,
		Description: "Группировка страниц по студентам…",
	})

	students, orphans := p.groupByStudent(parsedPages)
	result.Orphans = orphans
	p.logger.Info("grouping complete", "students", len(students), "orphans", len(orphans))

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output dir %q: %w", outputDir, err)
	}

	total := int64(len(students) + len(orphans))
	saved := int64(0)

	for _, student := range students {
		if ctx.Err() != nil {
			return result, fmt.Errorf("processing cancelled: %w", ctx.Err())
		}
		saved++
		progress(models.ProcessingProgress{
			Stage:       "saving",
			Current:     saved,
			Total:       total,
			Description: fmt.Sprintf("Сохранение: %s", student.FullName),
		})

		outFile, saveErr := p.saveStudent(ctx, student, outputDir)
		if saveErr != nil {
			p.logger.Error("failed to save student", "name", student.FullName, "err", saveErr)
			result.Errors = append(result.Errors,
				fmt.Sprintf("не удалось сохранить файл для %s: %v", student.FullName, saveErr))
			continue
		}
		result.OutputFiles = append(result.OutputFiles, outFile)
	}

	for _, orphan := range orphans {
		if ctx.Err() != nil {
			return result, fmt.Errorf("processing cancelled: %w", ctx.Err())
		}
		saved++
		progress(models.ProcessingProgress{
			Stage:       "saving",
			Current:     saved,
			Total:       total,
			Description: fmt.Sprintf("Сохранение неопознанной страницы %d…", orphan.Page.Number),
		})

		outFile, saveErr := p.saveOrphan(ctx, orphan.Page, outputDir)
		if saveErr != nil {
			p.logger.Warn("failed to save orphan page", "page", orphan.Page.Number, "err", saveErr)
			result.Errors = append(result.Errors,
				fmt.Sprintf("не удалось сохранить стр. %d: %v", orphan.Page.Number, saveErr))
			continue
		}
		result.OutputFiles = append(result.OutputFiles, outFile)
	}

	return result, nil
}
