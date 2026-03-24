// Package pipeline orchestrates the full PDF processing workflow:
// split → render → OCR → parse → group → merge & save.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

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
	parser *ocr.Parser
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
		parser: ocr.NewParser(),
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

	tmpDir, err := os.MkdirTemp("", "scansplit-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			p.logger.Warn("failed to remove temp dir", "path", tmpDir, "err", rmErr)
		}
	}()

	splitDir := filepath.Join(tmpDir, "split")
	imageDir := filepath.Join(tmpDir, "images")
	for _, d := range []string{splitDir, imageDir} {
		if mkErr := os.MkdirAll(d, 0o755); mkErr != nil {
			return nil, fmt.Errorf("failed to create dir %q: %w", d, mkErr)
		}
	}

	// ── Step 1: Split ──────────────────────────────────────────────────────────
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

	// ── Steps 2 & 3: Render + OCR (concurrent worker pool) ────────────────────
	parsedPages, errs := p.renderAndOCR(ctx, pages, imageDir, progress)

	result := &models.ProcessingResult{Errors: errs}

	// ── Step 4: Group pages by student ────────────────────────────────────────
	progress(models.ProcessingProgress{
		Stage:       "grouping",
		Current:     0,
		Total:       1,
		Description: "Группировка страниц по студентам…",
	})

	students, orphans := groupByStudent(parsedPages, p.logger)
	result.Orphans = orphans
	p.logger.Info("grouping complete", "students", len(students), "orphans", len(orphans))

	// ── Step 5: Merge & save ──────────────────────────────────────────────────
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

	// ── Step 6: Save orphan pages ─────────────────────────────────────────────
	// Orphans are pages where OCR found neither name nor group (confidence=0).
	// They are saved individually so the user can review them manually.
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

		fileName := fmt.Sprintf("неопознанная_стр_%02d.pdf", orphan.Page.Number)
		outPath := filepath.Join(outputDir, fileName)
		if cpErr := pdf.MergePages(ctx, []string{orphan.Page.PDFPath}, outPath); cpErr != nil {
			p.logger.Warn("failed to save orphan page", "page", orphan.Page.Number, "err", cpErr)
			result.Errors = append(result.Errors,
				fmt.Sprintf("не удалось сохранить стр. %d: %v", orphan.Page.Number, cpErr))
			continue
		}
		p.logger.Info("saved orphan page", "page", orphan.Page.Number, "path", outPath)
		result.OutputFiles = append(result.OutputFiles, models.OutputFile{
			Student: models.Student{
				FullName: fmt.Sprintf("Неопознанная страница %d", orphan.Page.Number),
				Pages:    []models.Page{orphan.Page},
			},
			FilePath: outPath,
			FileName: fileName,
		})
	}

	return result, nil
}

// renderAndOCR processes pages concurrently: render to image, then OCR, then parse.
func (p *Pipeline) renderAndOCR(
	ctx context.Context,
	pages []models.Page,
	imageDir string,
	progress ProgressCallback,
) ([]models.ParsedPage, []string) {
	type workItem struct {
		index int
		page  models.Page
	}

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
	)

	for range p.cfg.Concurrency {
		wg.Go(func() {
			for item := range work {
				if ctx.Err() != nil {
					return
				}

				parsed, procErr := p.processPage(ctx, item.page, imageDir)
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
	return results, errs
}

// processPage renders one page to an image and runs OCR + parse on it.
func (p *Pipeline) processPage(ctx context.Context, page models.Page, imageDir string) (models.ParsedPage, error) {
	imagePath, renderErr := pdf.RenderPage(ctx, page.PDFPath, imageDir)
	if renderErr != nil {
		// Warn but continue — mock OCR does not need an actual image.
		p.logger.Warn("render failed, continuing with empty image", "page", page.Number, "err", renderErr)
	}
	page.ImagePath = imagePath

	var imageData []byte
	if imagePath != "" {
		var readErr error
		imageData, readErr = os.ReadFile(imagePath)
		if readErr != nil {
			p.logger.Warn("failed to read rendered image", "path", imagePath, "err", readErr)
		}
	}

	ocrText, err := p.ocr.RecognizeText(ctx, imageData)
	if err != nil {
		return models.ParsedPage{}, fmt.Errorf("OCR failed for page %d: %w", page.Number, err)
	}
	page.OCRText = ocrText

	res := p.parser.Parse(ocrText)
	return models.ParsedPage{
		Page:       page,
		FullName:   res.FullName,
		Group:      res.Group,
		Confidence: res.Confidence,
		IsOrphan:   res.Confidence == 0,
	}, nil
}

// groupByStudent groups pages by recognised student name.
//
// Two-pass algorithm:
//  1. Collect all pages that carry a recognised name ("anchors").
//  2. For every page (named or not) find the nearest anchor by index distance.
//     On a tie (equidistant between two anchors) the preceding one wins.
//  3. Anchor names are fuzzy-matched so that declension variants
//     (Малышев / Малышева) collapse into one canonical bucket.
//
// This beats the old "attach to last seen" approach for documents where
// unnamed pages appear BEFORE the student's title page (e.g. a signature /
// conclusion page printed before the cover sheet).
// Only if there are no anchors at all do pages become orphans.
func groupByStudent(pages []models.ParsedPage, logger *slog.Logger) (students []models.Student, orphans []models.ParsedPage) {
	// ── Step 1: collect anchor positions ─────────────────────────────────────
	type anchor struct {
		idx   int
		name  string
		group string
	}
	var anchors []anchor
	for i, pp := range pages {
		if pp.FullName != "" {
			anchors = append(anchors, anchor{i, pp.FullName, pp.Group})
			logger.Debug("anchor found", "page", pp.Page.Number, "name", pp.FullName, "group", pp.Group, "confidence", pp.Confidence)
		}
	}

	if len(anchors) == 0 {
		for _, pp := range pages {
			orphans = append(orphans, pp)
		}
		logger.Debug("no anchors — all pages are orphans", "count", len(pages))
		return
	}

	// ── Step 2: assign every page to the nearest anchor ──────────────────────
	type bucket struct {
		canonical string // shortest (likely nominative) name seen
		group     string
		pages     []models.Page
	}
	buckets := make(map[string]*bucket)
	var order []string // first-appearance order

	getKey := func(name, group string) string {
		for _, k := range order {
			if namesSimilar(name, buckets[k].canonical) {
				return k
			}
		}
		buckets[name] = &bucket{canonical: name, group: group}
		order = append(order, name)
		return name
	}

	for i, pp := range pages {
		name, group := pp.FullName, pp.Group
		reason := "own name"

		if name == "" {
			// Cover-page heuristic: if the very next page carries a FULL name this
			// is almost certainly the cover page of that student's section — assign
			// it forward even though the preceding student is equally close.
			// Only fires for full names (not abbreviated forms like "Трошов И.В.")
			// to avoid stealing trailing pages from the preceding student.
			if i+1 < len(pages) && pages[i+1].FullName != "" && !hasInitials(pages[i+1].FullName) {
				name = pages[i+1].FullName
				group = pages[i+1].Group
				reason = "cover-page look-ahead"
			} else {
				// General case: nearest anchor; ties go to the preceding student
				// (anchors are iterated in order → first ≤-distance match wins).
				best := anchors[0]
				bestDist := intAbs(i - best.idx)
				for _, a := range anchors[1:] {
					if d := intAbs(i - a.idx); d < bestDist {
						bestDist = d
						best = a
					}
				}
				name, group = best.name, best.group
				reason = fmt.Sprintf("nearest-anchor (dist=%d)", bestDist)
			}
		}

		logger.Debug("page assigned",
			"page", pp.Page.Number,
			"reason", reason,
			"student", name,
			"ocr_name", pp.FullName,
			"ocr_group", pp.Group,
			"confidence", pp.Confidence,
		)

		key := getKey(name, group)
		b := buckets[key]
		b.pages = append(b.pages, pp.Page)
		// Always use the current page's group for back-fill, even if the page
		// is unnamed (e.g. a page with only a group code but no readable name).
		if pp.Group != "" && b.group == "" {
			b.group = pp.Group
		}
		// Prefer the full 3-word form over abbreviated; among same-type names
		// prefer the shorter string (likely nominative case).
		existingAbbrev := hasInitials(b.canonical)
		newAbbrev := hasInitials(name)
		if (!newAbbrev && existingAbbrev) ||
			(newAbbrev == existingAbbrev && len(name) < len(b.canonical)) {
			b.canonical = name
		}
	}

	// ── Step 3: emit students in first-appearance order ───────────────────────
	for _, key := range order {
		b := buckets[key]
		students = append(students, models.Student{
			FullName: b.canonical,
			Group:    b.group,
			Pages:    b.pages,
		})
	}
	return
}

// GroupPages is an exported shim used by the debug CLI tool.
// It exposes the internal groupByStudent logic for diagnostic purposes.
func GroupPages(pages []models.ParsedPage) ([]models.Student, []models.ParsedPage) {
	return groupByStudent(pages, slog.Default())
}

func intAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// namesSimilar reports whether a and b likely refer to the same person.
// For each corresponding word-pair, one must be a prefix of the other and
// the suffix must be ≤ 3 runes — this covers typical Russian noun endings
// (а, у, е, ой, ого, ому, ем, ях, …) without conflating unrelated names.
//
// 'ё' is normalised to 'е' before comparison because OCR often confuses them
// and Russian declension changes ё→е (Пётр→Петра, Фёдоров→Фёдорова).
func namesSimilar(a, b string) bool {
	if a == b {
		return true
	}
	wa := strings.Fields(normaliseYo(strings.ToLower(a)))
	wb := strings.Fields(normaliseYo(strings.ToLower(b)))
	if len(wa) != len(wb) || len(wa) == 0 {
		// Special case: 2-word abbreviated form (Фамилия И.) vs 3-word full name.
		// E.g. "Котляров Н." merges with "Котляров Николай Алексеевич".
		short, long := wa, wb
		if len(wa) > len(wb) {
			short, long = wb, wa
		}
		if len(short) == 2 && len(long) == 3 && isLowercaseInitial(short[1]) {
			ra, rb := []rune(short[0]), []rune(long[0])
			s, l := ra, rb
			if len(s) > len(l) {
				s, l = l, s
			}
			surnameOK := (string(l[:len(s)]) == string(s) && len(l)-len(s) <= 3) ||
				(len(ra) == len(rb) && len(ra) > 2 && string(ra[:len(ra)-1]) == string(rb[:len(rb)-1]))
			initRunes := []rune(short[1])
			nameRunes := []rune(long[1])
			if surnameOK && len(nameRunes) > 0 && initRunes[0] == nameRunes[0] {
				return true
			}
		}
		return false
	}
	for i := range wa {
		if wa[i] == wb[i] {
			continue
		}
		ra, rb := []rune(wa[i]), []rune(wb[i])

		// Case 1: one word is a prefix of the other, suffix ≤ 3 runes.
		// Handles Малышев→Малышева, Евгеньевич→Евгеньевичу, etc.
		short, long := ra, rb
		if len(short) > len(long) {
			short, long = long, short
		}
		if string(long[:len(short)]) == string(short) && len(long)-len(short) <= 3 {
			continue
		}

		// Case 2: same-length words differing only in the last rune.
		// Handles Тимофей→Тимофею, Тимофей→Тимофея (й→ю/я declension).
		if len(ra) == len(rb) && len(ra) > 2 && string(ra[:len(ra)-1]) == string(rb[:len(rb)-1]) {
			continue
		}

		// Case 3: one word is a single-letter initial matching the first letter of the
		// other word.  Handles "А. Е." form vs "Алексей Евгеньевич" in same-length
		// abbreviated names like "Задевалов А. Е." vs "Задевалов Алексей Евгеньевич".
		if isLowercaseInitial(wa[i]) && len(rb) > 0 && []rune(wa[i])[0] == rb[0] {
			continue
		}
		if isLowercaseInitial(wb[i]) && len(ra) > 0 && []rune(wb[i])[0] == ra[0] {
			continue
		}

		return false
	}
	return true
}

// isLowercaseInitial returns true if s (already lower-cased) looks like a single
// Cyrillic initial: "а", "а.", or "а.б." (combined initials as one token).
func isLowercaseInitial(s string) bool {
	runes := []rune(s)
	switch len(runes) {
	case 1:
		return isCyrillicLower(runes[0])
	case 2:
		return isCyrillicLower(runes[0]) && runes[1] == '.'
	case 4:
		return isCyrillicLower(runes[0]) && runes[1] == '.' && isCyrillicLower(runes[2]) && runes[3] == '.'
	}
	return false
}

func isCyrillicLower(r rune) bool {
	return (r >= 'а' && r <= 'я') || r == 'ё'
}

// hasInitials reports whether any non-surname word in the name is an initial
// (e.g. "А.", "И.В.").  Used to prefer full names over abbreviated forms.
func hasInitials(name string) bool {
	parts := strings.Fields(normaliseYo(strings.ToLower(name)))
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts[1:] {
		if isLowercaseInitial(p) {
			return true
		}
	}
	return false
}

// yoReplacer is compiled once and reused across all namesSimilar calls.
var yoReplacer = strings.NewReplacer("ё", "е", "Ё", "Е")

// normaliseYo replaces 'ё'/'Ё' with 'е'/'Е' so that OCR variants and
// declension-driven vowel shifts (Пётр→Петра) don't break prefix matching.
func normaliseYo(s string) string {
	return yoReplacer.Replace(s)
}

// saveStudent merges the student's pages into a single PDF and writes it to outputDir.
func (p *Pipeline) saveStudent(ctx context.Context, student models.Student, outputDir string) (models.OutputFile, error) {
	if len(student.Pages) == 0 {
		return models.OutputFile{}, fmt.Errorf("student %q has no pages", student.FullName)
	}

	fileName := safeFileName(student.FullName, student.Group) + ".pdf"
	outputPath := filepath.Join(outputDir, fileName)

	pagePaths := make([]string, len(student.Pages))
	for i, pg := range student.Pages {
		pagePaths[i] = pg.PDFPath
	}

	if err := pdf.MergePages(ctx, pagePaths, outputPath); err != nil {
		return models.OutputFile{}, fmt.Errorf("merge failed: %w", err)
	}

	return models.OutputFile{
		Student:  student,
		FilePath: outputPath,
		FileName: fileName,
	}, nil
}

// safeFileName builds a filesystem-safe name: "Фамилия_Имя_Отчество_Группа".
// Characters illegal in Windows/macOS filenames are replaced with underscores.
func safeFileName(fullName, group string) string {
	parts := append(strings.Fields(fullName), group)
	var b strings.Builder
	for i, seg := range parts {
		if i > 0 {
			b.WriteByte('_')
		}
		for _, r := range seg {
			switch r {
			case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
				b.WriteByte('_')
			default:
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}
