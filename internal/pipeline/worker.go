package pipeline

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hellascape/scansplit/internal/models"
	"github.com/hellascape/scansplit/internal/ocr"
	"github.com/hellascape/scansplit/internal/pdf"
)

type workItem struct {
	index     int
	page      models.Page
	imageData []byte
}

// renderAndOCR renders all pages parallel,
// then fans out OCR+parse to a concurrent worker pool.
func (p *Pipeline) renderAndOCR(
	ctx context.Context,
	pages []models.Page,
	progress ProgressCallback,
) ([]models.ParsedPage, []string, int64) {
	var (
		processWG = &sync.WaitGroup{}
		work      = make(chan workItem, p.cfg.Concurrency)
	)

	doc, openErr := pdf.OpenDocument(pages[0].SourcePath)
	if openErr != nil {
		return nil, []string{fmt.Sprintf("failed to open PDF: %v", openErr)}, 0
	}

	go func() {
		defer doc.Close()
		defer close(work)

		for i, pg := range pages {
			if ctx.Err() != nil {
				p.logger.Warn("processing cancelled")
				return
			}
			data, err := doc.RenderPage(pg.Number - 1)
			if err != nil {
				p.logger.Warn("render failed, continuing with empty image", "page", pg.Number, "err", err)
			}

			work <- workItem{index: i, page: pg, imageData: data}
		}
	}()

	results := make([]models.ParsedPage, len(pages))
	var (
		errsMu    sync.Mutex
		errs      []string
		completed atomic.Int64
		totalMs   atomic.Int64
	)

	for range p.cfg.Concurrency {
		processWG.Go(func() {
			for item := range work {
				if ctx.Err() != nil {
					return
				}

				t0 := time.Now()
				parsed, procErr := p.processPage(ctx, item.page, item.imageData)
				totalMs.Add(time.Since(t0).Milliseconds())
				if procErr != nil {
					p.logger.Error("page processing failed", "page", item.page.Number, "err", procErr)
					errsMu.Lock()
					errs = append(errs, fmt.Sprintf("страница %d: %v", item.page.Number, procErr))
					errsMu.Unlock()
					results[item.index] = models.ParsedPage{Page: item.page, IsOrphan: true}
				} else {
					results[item.index] = parsed
				}

				done := completed.Add(1)
				progress(models.ProcessingProgress{
					Stage:       "ocr",
					Current:     done,
					Total:       int64(len(pages)),
					Description: fmt.Sprintf("Распознавание страницы %d из %d…", done, len(pages)),
				})
			}
		})
	}

	processWG.Wait()
	var avgMs int64
	if n := int64(len(pages)); n > 0 {
		avgMs = totalMs.Load() / n
	}
	return results, errs, avgMs
}

// processPage runs OCR + parse on pre-rendered image data.
func (p *Pipeline) processPage(ctx context.Context, page models.Page, imageData []byte) (models.ParsedPage, error) {
	ocrText, err := p.ocr.RecognizeText(ctx, imageData)
	if err != nil {
		return models.ParsedPage{}, fmt.Errorf("OCR failed for page %d: %w", page.Number, err)
	}
	page.OCRText = ocrText

	res := ocr.Parse(ocrText)
	return models.ParsedPage{
		Page:       page,
		FullName:   res.FullName,
		Group:      res.Group,
		Confidence: res.Confidence,
		IsOrphan:   res.Confidence == 0,
	}, nil
}
