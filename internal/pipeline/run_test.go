package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/h3llascape/scan-split/internal/models"
	"github.com/h3llascape/scan-split/internal/ocr/mocks"
)

// ── OCR text helpers ──────────────────────────────────────────────────────────

func studentOCR(name, group string) string {
	return fmt.Sprintf("Студент: %s\nГруппа: %s\n", name, group)
}

// unnamedOCR returns text with no recognisable name or group.
func unnamedOCR() string {
	return "Подпись: ___\nДата: ___\n"
}

// supervisorOCR returns text where the name is extracted (3 cap Cyrillic words)
// but belongs to a supervisor, not a student.
func supervisorOCR(name string) string {
	return fmt.Sprintf("ФИО: %s\nДолжность: Руководитель\n", name)
}

// ── PDF fixture ───────────────────────────────────────────────────────────────

// makeBlankPDF creates a minimal valid PDF with n blank pages in a temp dir
// and returns the file path. go-fitz (MuPDF) can render these pages; the
// mock OCR ignores the image bytes and returns whatever we configure.
func makeBlankPDF(t *testing.T, n int) string {
	t.Helper()

	var buf bytes.Buffer
	offsets := make([]int, 0, 2+n)

	w := func(s string) { buf.WriteString(s) }

	w("%PDF-1.4\n")

	offsets = append(offsets, buf.Len())
	w("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	kids := make([]string, n)
	for i := range n {
		kids[i] = fmt.Sprintf("%d 0 R", 3+i)
	}
	offsets = append(offsets, buf.Len())
	w(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n",
		strings.Join(kids, " "), n))

	for i := range n {
		offsets = append(offsets, buf.Len())
		w(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n", 3+i))
	}

	xrefPos := buf.Len()
	total := 2 + n
	w(fmt.Sprintf("xref\n0 %d\n", total+1))
	w("0000000000 65535 f \n")
	for _, off := range offsets {
		w(fmt.Sprintf("%010d 00000 n \n", off))
	}
	w(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\n", total+1))
	w(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefPos))

	f, err := os.CreateTemp(t.TempDir(), "blank_*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// ── Mock builder ──────────────────────────────────────────────────────────────

// ocrSequence configures the mock to return responses[i] on the i-th call,
// in page order. Works correctly when Concurrency=1.
func ocrSequence(t *testing.T, mc *minimock.Controller, responses []string) *mocks.ProviderMock {
	t.Helper()
	m := mocks.NewProviderMock(mc)
	i := 0
	m.RecognizeTextMock.Set(func(_ context.Context, _ []byte) (string, error) {
		if i >= len(responses) {
			t.Fatalf("RecognizeText called more times (%d) than responses provided (%d)", i+1, len(responses))
		}
		r := responses[i]
		i++
		return r, nil
	})
	return m
}

// newPipeline builds a pipeline with Concurrency=1 so OCR calls are
// deterministic and match ocrSequence order.
func newPipeline(provider *mocks.ProviderMock, whitelist ...string) *Pipeline {
	return New(Config{Concurrency: 1, Whitelist: whitelist}, provider, slog.Default())
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// Two students, each with their own pages. No unnamed pages.
// Expected: 2 output files, correct names.
func TestRun_TwoStudents(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 4)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		studentOCR("Иванов Иван Иванович", "РИ-330001"),
		studentOCR("Иванова Ивана Ивановича", ""), // declension of same person
		studentOCR("Петров Пётр Петрович", "РИ-330002"),
		studentOCR("Петрова Петра Петровича", ""), // declension
	})

	result, err := newPipeline(provider).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.OutputFiles) != 2 {
		t.Fatalf("expected 2 output files, got %d", len(result.OutputFiles))
	}

	names := outputNames(result)
	if !containsName(names, "Иванов Иван Иванович") {
		t.Errorf("Иванов not found in output: %v", names)
	}
	if !containsName(names, "Петров Пётр Петрович") {
		t.Errorf("Петров not found in output: %v", names)
	}

	assertPageCount(t, result, "Иванов Иван Иванович", 2)
	assertPageCount(t, result, "Петров Пётр Петрович", 2)
}

// Unnamed page between two students with adjacent preceding anchor: tie goes
// to preceding student (no cover-page look-ahead when prevDist ≤ 1).
func TestRun_TieBetweenStudents(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 4)
	outDir := t.TempDir()

	// Layout: [Иванов title] [unnamed ← preceding wins tie] [Петров title] [unnamed ← nearest Петров]
	provider := ocrSequence(t, mc, []string{
		studentOCR("Иванов Иван Иванович", "РИ-330001"),
		unnamedOCR(), // prevDist=1, tie → Иванов (preceding)
		studentOCR("Петров Пётр Петрович", "РИ-330002"),
		unnamedOCR(), // nearest-anchor Петров (dist=1)
	})

	result, err := newPipeline(provider).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	assertPageCount(t, result, "Иванов Иван Иванович", 2)
	assertPageCount(t, result, "Петров Пётр Петрович", 2)
}

// Cover-page look-ahead fires when no preceding anchor is adjacent (prevDist > 1).
func TestRun_CoverPageHeuristic(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 5)
	outDir := t.TempDir()

	// Layout: [Иванов title] [unnamed ← Иванов trailing] [unnamed ← cover of Петров] [Петров title] [unnamed ← Петров trailing]
	provider := ocrSequence(t, mc, []string{
		studentOCR("Иванов Иван Иванович", "РИ-330001"),
		unnamedOCR(), // prevDist=1, tie → Иванов
		unnamedOCR(), // prevDist=2, look-ahead → Петров
		studentOCR("Петров Пётр Петрович", "РИ-330002"),
		unnamedOCR(), // nearest-anchor Петров (dist=1)
	})

	result, err := newPipeline(provider).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	assertPageCount(t, result, "Иванов Иван Иванович", 2)
	assertPageCount(t, result, "Петров Пётр Петрович", 3)
}

// Unnamed pages before the first student's title are assigned to that student
// via nearest-anchor.
func TestRun_LeadingUnnamedPages(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 3)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		unnamedOCR(),
		unnamedOCR(),
		studentOCR("Иванов Иван Иванович", "РИ-330001"),
	})

	result, err := newPipeline(provider).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	assertPageCount(t, result, "Иванов Иван Иванович", 3)
}

// Nearest-anchor splits unnamed pages correctly between two students.
// idx0: Темирханова  idx1: unnamed(→Темирханова, dist=1<2)
// idx2: unnamed(→Трошов, dist=1<2)  idx3: Трошов
func TestRun_NearestAnchorSplit(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 4)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		studentOCR("Темирханова Зарина Кокеновна", "РИ-320930"),
		unnamedOCR(), // dist to Темирханова=1, to Трошов=2 → Темирханова
		unnamedOCR(), // dist to Темирханова=2, to Трошов=1 → Трошов
		studentOCR("Трошов Пётр Николаевич", "РИ-320931"),
	})

	result, err := newPipeline(provider).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	assertPageCount(t, result, "Темирханова Зарина Кокеновна", 2)
	assertPageCount(t, result, "Трошов Пётр Николаевич", 2)
}

// When no names are recognised, all pages become orphans.
func TestRun_AllOrphans(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 3)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		unnamedOCR(),
		unnamedOCR(),
		unnamedOCR(),
	})

	result, err := newPipeline(provider).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.Orphans) != 3 {
		t.Errorf("expected 3 orphans, got %d", len(result.Orphans))
	}
	// Each orphan page is saved as its own file.
	if len(result.OutputFiles) != 3 {
		t.Errorf("expected 3 output files for orphans, got %d", len(result.OutputFiles))
	}
}

// OCR error on one page: that page becomes an orphan, other pages are unaffected.
func TestRun_OCRErrorOnOnePage(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 3)
	outDir := t.TempDir()

	m := mocks.NewProviderMock(mc)
	call := 0
	m.RecognizeTextMock.Set(func(_ context.Context, _ []byte) (string, error) {
		call++
		if call == 2 {
			return "", fmt.Errorf("OCR engine crashed")
		}
		return studentOCR("Иванов Иван Иванович", "РИ-330001"), nil
	})

	result, err := newPipeline(m).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one error for the failed page")
	}
	// Page 2 fails OCR → becomes unnamed → nearest-anchor assigns to Иванов.
	// All 3 pages end up in one student file.
	assertPageCount(t, result, "Иванов Иван Иванович", 3)
}

// Whitelist with only one student: supervisor pages are excluded from output.
// Real-world case: title pages carry supervisor name (Корнякова Елена Михайловна)
// alongside student section, but only whitelisted names should produce files.
func TestRun_WhitelistExcludesSupervisors(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	// Layout mirrors the real PDF:
	// p1: student title (Трошов)  p2: student review page (Трошов)
	// p3: supervisor page (Корнякова)  p4: another supervisor (Мартынов)
	pdf := makeBlankPDF(t, 4)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		studentOCR("Трошов Илья Владимирович", "РИ-320930"),
		studentOCR("Трошов Илья Владимирович", "РИ-320930"),
		supervisorOCR("Корнякова Елена Михайловна"),
		supervisorOCR("Мартынов Артём Максимович"),
	})

	result, err := newPipeline(provider, "Трошов").Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.OutputFiles) != 1 {
		t.Fatalf("expected 1 output file, got %d: %v", len(result.OutputFiles), outputNames(result))
	}
	if !containsName(outputNames(result), "Трошов Илья Владимирович") {
		t.Errorf("Трошов not found in output: %v", outputNames(result))
	}
	// Only Трошов's 2 pages; supervisor pages excluded.
	assertPageCount(t, result, "Трошов Илья Владимирович", 2)
}

// With whitelist: unnamed page between two students is assigned to the
// nearest whitelisted student. Unnamed page nearest to a non-whitelisted
// person (supervisor) is excluded entirely.
//
// Layout:
//
//	p1 Трошов (whitelisted)     → anchor
//	p2 Трошов (whitelisted)     → anchor
//	p3 unnamed                  → nearest=Трошов(dist=1) → kept
//	p4 unnamed                  → nearest=Корнякова(dist=1) → excluded
//	p5 Корнякова (supervisor)   → excluded
func TestRun_WhitelistUnnamedProximity(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 5)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		studentOCR("Трошов Илья Владимирович", "РИ-320930"),
		studentOCR("Трошов Илья Владимирович", "РИ-320930"),
		unnamedOCR(), // nearest original anchor = Трошов (dist=1) → kept
		unnamedOCR(), // nearest original anchor = Корнякова (dist=1) → excluded
		supervisorOCR("Корнякова Елена Михайловна"),
	})

	result, err := newPipeline(provider, "Трошов").Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.OutputFiles) != 1 {
		t.Fatalf("expected 1 output file, got %d", len(result.OutputFiles))
	}
	// Pages 1, 2 (named Трошов) + page 3 (unnamed, nearest to Трошов).
	assertPageCount(t, result, "Трошов Илья Владимирович", 3)
}

// With whitelist: multiple whitelisted students, all get correct pages.
// Non-whitelisted names and their adjacent unnamed pages are excluded.
func TestRun_WhitelistMultipleStudents(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	// Mirrors real PDF structure from the test document:
	// Темирханова (2 pages) | unnamed | Трошов (3 pages) | supervisor (2 pages)
	pdf := makeBlankPDF(t, 8)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		studentOCR("Темирханова Ева Арсеновна", "РИ-320930"),
		studentOCR("Темирханова Ева Арсеновна", "РИ-320930"),
		unnamedOCR(), // nearest = Темирханова (dist=1) → Темирханова
		studentOCR("Трошов Илья Владимирович", "РИ-320930"),
		studentOCR("Трошов Илья Владимирович", "РИ-320930"),
		studentOCR("Трошов Илья Владимирович", "РИ-320930"),
		supervisorOCR("Корнякова Елена Михайловна"), // not in whitelist → excluded
		supervisorOCR("Корнякова Елена Михайловна"), // not in whitelist → excluded
	})

	result, err := newPipeline(provider, "Темирханова", "Трошов").
		Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.OutputFiles) != 2 {
		t.Fatalf("expected 2 output files, got %d: %v", len(result.OutputFiles), outputNames(result))
	}
	// Unnamed p3 between Темирханова (prevDist=1) and Трошов (dist=1):
	// preceding wins tie → Темирханова gets 3 pages.
	assertPageCount(t, result, "Темирханова Ева Арсеновна", 3)
	assertPageCount(t, result, "Трошов Илья Владимирович", 3)
}

// A page where OCR extracts supervisor name as primary, but student name is
// also present in the raw text → page is reassigned to the student, not excluded.
func TestRun_WhitelistSecondaryNameScan(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 2)
	outDir := t.TempDir()

	// Page 1: no "Студент:" label → fallback finds first 3-word Cyrillic sequence.
	// Supervisor name comes first → primary extracted name = Корнякова (not in whitelist).
	// But Трошов also appears on the page → secondary scan reassigns page to Трошов.
	mixedPage := "Корнякова Елена Михайловна\nруководитель практики от предприятия\nТрошов Илья Владимирович\nГруппа: РИ-320930\n"
	provider := ocrSequence(t, mc, []string{
		mixedPage,
		studentOCR("Трошов Илья Владимирович", "РИ-320930"),
	})

	result, err := newPipeline(provider, "Трошов").Run(context.Background(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.OutputFiles) != 1 {
		t.Fatalf("expected 1 output file, got %d: %v", len(result.OutputFiles), outputNames(result))
	}
	// Both pages: page 1 reassigned to Трошов via secondary scan, page 2 direct.
	assertPageCount(t, result, "Трошов Илья Владимирович", 2)
}

// Empty whitelist = no filtering, all names create anchors.
func TestRun_EmptyWhitelistNoFilter(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 2)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		studentOCR("Иванов Иван Иванович", "РИ-330001"),
		supervisorOCR("Корнякова Елена Михайловна"),
	})

	// No whitelist → supervisor name also creates a bucket.
	result, err := newPipeline(provider).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.OutputFiles) != 2 {
		t.Fatalf("expected 2 files (no filtering), got %d", len(result.OutputFiles))
	}
}

// Declension variants of the same name are merged into one student bucket.
func TestRun_DeclensionMerge(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 4)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		studentOCR("Малышев Тимофей Евгеньевич", "РИ-330001"),
		studentOCR("Малышеву Тимофею Евгеньевичу", ""), // dative
		studentOCR("Малышева Тимофея Евгеньевича", ""), // genitive
		studentOCR("Малышев Тимофей Евгеньевич", "РИ-330001"),
	})

	result, err := newPipeline(provider).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.OutputFiles) != 1 {
		t.Fatalf("expected 1 file after declension merge, got %d: %v", len(result.OutputFiles), outputNames(result))
	}
	assertPageCount(t, result, "Малышев Тимофей Евгеньевич", 4)
}

// Cancellation mid-run returns an error.
func TestRun_Cancelled(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 3)
	outDir := t.TempDir()

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	m := mocks.NewProviderMock(mc)
	m.RecognizeTextMock.Optional().Set(func(_ context.Context, _ []byte) (string, error) {
		return studentOCR("Иванов Иван Иванович", "РИ-330001"), nil
	})

	_, err := newPipeline(m).Run(ctx, pdf, outDir, nil)
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

// Combined initials "М.Ю." on one page + full name "Матвей Юрьевич" on another
// must merge into a single student. The trailing bare "А" is OCR junk.
func TestRun_CombinedInitialsMerge(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 3)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		studentOCR("Свидер М.Ю. А", "РИ-420932"),        // combined initials + trailing junk
		studentOCR("Свидер Матвей Юрьевич", "РИ-420932"), // full name
		studentOCR("Свидер Матвей Юрьевич", "РИ-420932"),
	})

	result, err := newPipeline(provider).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.OutputFiles) != 1 {
		t.Fatalf("expected 1 output file, got %d: %v", len(result.OutputFiles), outputNames(result))
	}
	assertPageCount(t, result, "Свидер Матвей Юрьевич", 3)
}

// OCR truncation: "Серг" is a truncated "Сергеевна". Both forms must merge.
func TestRun_TruncatedNameMerge(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 4)
	outDir := t.TempDir()

	provider := ocrSequence(t, mc, []string{
		studentOCR("Долгих Александра Сергеевна", "РИ-420932"),
		studentOCR("Долгих Александра Сергеевна", "РИ-420932"),
		studentOCR("Долгих Александра Серг", ""),   // OCR truncation
		studentOCR("Долгих Александра Сергеевна", "РИ-420932"),
	})

	result, err := newPipeline(provider).Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.OutputFiles) != 1 {
		t.Fatalf("expected 1 output file, got %d: %v", len(result.OutputFiles), outputNames(result))
	}
	assertPageCount(t, result, "Долгих Александра Сергеевна", 4)
}

// Page has no "Студент:" label and no 3-word full name, but has an abbreviated
// name "Тенкачевой Д.А." found by Priority 3 scanner. Whitelist matches it.
func TestRun_WhitelistAbbreviatedNameInAllNames(t *testing.T) {
	t.Parallel()
	mc := minimock.NewController(t)

	pdf := makeBlankPDF(t, 3)
	outDir := t.TempDir()

	// Page 1: normal titled page for Тенкачева
	// Page 2: document with only abbreviated name "Тенкачевой Д.А." and
	//         another abbreviated name "Поротникову М.Г." (not in whitelist)
	// Page 3: normal titled page for Тенкачева
	abbreviatedPage := "РАСПОРЯЖЕНИЕ\nО назначении наставника практики\nстудента УрФУ Тенкачевой Д.А.\nНазначить наставником Поротникову М.Г.\n"
	provider := ocrSequence(t, mc, []string{
		studentOCR("Тенкачева Дарья Андреевна", "РИ-420932"),
		abbreviatedPage, // no full name, no label → AllNames: [Тенкачевой Д.А., Поротникову М.Г.]
		studentOCR("Тенкачева Дарья Андреевна", "РИ-420932"),
	})

	result, err := newPipeline(provider, "Тенкачева").Run(t.Context(), pdf, outDir, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.OutputFiles) != 1 {
		t.Fatalf("expected 1 output file, got %d: %v", len(result.OutputFiles), outputNames(result))
	}
	// All 3 pages belong to Тенкачева: page 2 matched via AllNames + whitelist.
	assertPageCount(t, result, "Тенкачева Дарья Андреевна", 3)
}

// ── Assertion helpers ─────────────────────────────────────────────────────────

func outputNames(r *models.ProcessingResult) []string {
	names := make([]string, len(r.OutputFiles))
	for i, f := range r.OutputFiles {
		names[i] = f.Student.FullName
	}
	return names
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// assertPageCount checks that the output file for the given student has exactly
// wantPages pages. Uses namesSimilar so declension variants match.
func assertPageCount(t *testing.T, r *models.ProcessingResult, studentName string, wantPages int) {
	t.Helper()
	for _, f := range r.OutputFiles {
		if namesSimilar(f.Student.FullName, studentName) {
			if len(f.Student.Pages) != wantPages {
				t.Errorf("student %q: want %d pages, got %d", studentName, wantPages, len(f.Student.Pages))
			}
			return
		}
	}
	t.Errorf("student %q not found in output files: %v", studentName, outputNames(r))
}
