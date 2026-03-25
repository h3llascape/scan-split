// Package pipeline orchestrates the full PDF processing workflow:
// split → render → OCR → parse → group → merge & save.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hellascape/scansplit/internal/models"
	"github.com/hellascape/scansplit/internal/ocr"
	"github.com/hellascape/scansplit/internal/pdf"
)

// Config holds tunable pipeline parameters.
type Config struct {
	// Concurrency is the number of parallel OCR workers. Defaults to 4.
	Concurrency int
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

// Run executes the full pipeline and returns the processing result.
// progress is called on every meaningful state change and may be nil.
// Intermediate files are cleaned up via defer before Run returns.
func (p *Pipeline) Run(
	ctx context.Context,
	inputPath, outputDir string,
	progress ProgressCallback,
) (*models.ProcessingResult, error) {
	if progress == nil {
		progress = func(models.ProcessingProgress) {}
	}

	tmpDir, splitDir, err := makeTempDirs()
	if err != nil {
		return nil, err
	}
	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			p.logger.Warn("failed to remove temp dir", "path", tmpDir, "err", rmErr)
		}
	}()

	progress(models.ProcessingProgress{
		Stage:       "splitting",
		Current:     0,
		Total:       1,
		Description: "Разделение PDF на страницы…",
	})

	pages, err := pdf.SplitPages(ctx, inputPath, splitDir)
	if err != nil {
		return nil, fmt.Errorf("split stage failed: %w", err)
	}
	p.logger.Info("split complete", "pages", len(pages))

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

	total := len(students) + len(orphans)
	saved := 0

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

// makeTempDirs creates a temp root with a split/ subdirectory.
func makeTempDirs() (tmpDir, splitDir string, err error) {
	tmpDir, err = os.MkdirTemp("", "scansplit-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	splitDir = filepath.Join(tmpDir, "split")
	if mkErr := os.MkdirAll(splitDir, 0o755); mkErr != nil {
		os.RemoveAll(tmpDir)
		return "", "", fmt.Errorf("failed to create dir %q: %w", splitDir, mkErr)
	}
	return
}
