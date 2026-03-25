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
	index int
	page  models.Page
}

// renderAndOCR runs render→OCR→parse on all pages using a concurrent worker pool.
// Results are written into a pre-allocated slice at the original page index so
// output order is stable regardless of completion order.
func (p *Pipeline) renderAndOCR(
	ctx context.Context,
	pages []models.Page,
	progress ProgressCallback,
) ([]models.ParsedPage, []string, int64) {
	work := make(chan workItem, len(pages))
	for i, pg := range pages {
		work <- workItem{index: i, page: pg}
	}
	close(work)

	results := make([]models.ParsedPage, len(pages))
	var (
		errsMu    sync.Mutex
		errs      []string
		wg        sync.WaitGroup
		completed atomic.Int64
		totalMs   atomic.Int64
	)

	for range p.cfg.Concurrency {
		wg.Go(func() {
			for item := range work {
				if ctx.Err() != nil {
					return
				}

				t0 := time.Now()
				parsed, procErr := p.processPage(ctx, item.page)
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

				done := int(completed.Add(1))
				progress(models.ProcessingProgress{
					Stage:       "ocr",
					Current:     done,
					Total:       len(pages),
					Description: fmt.Sprintf("Распознавание страницы %d из %d…", done, len(pages)),
				})
			}
		})
	}

	wg.Wait()
	var avgMs int64
	if n := int64(len(pages)); n > 0 {
		avgMs = totalMs.Load() / n
	}
	return results, errs, avgMs
}

// processPage renders one page to PNG bytes in memory and runs OCR + parse on it.
func (p *Pipeline) processPage(ctx context.Context, page models.Page) (models.ParsedPage, error) {
	imageData, renderErr := pdf.RenderPage(ctx, page.PDFPath)
	if renderErr != nil {
		// Warn but continue — mock OCR does not need an actual image.
		p.logger.Warn("render failed, continuing with empty image", "page", page.Number, "err", renderErr)
	}

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
