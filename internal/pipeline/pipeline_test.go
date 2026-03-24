package pipeline

import (
	"io"
	"log/slog"
	"testing"

	"github.com/hellascape/scansplit/internal/models"
)

// ── namesSimilar ─────────────────────────────────────────────────────────────

func TestNamesSimilar(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		// Exact match
		{"Иванов Иван Иванович", "Иванов Иван Иванович", true},

		// Declension: surname only
		{"Малышев Иван Иванович", "Малышева Ивана Ивановича", true},
		{"Трошов Пётр Николаевич", "Трошова Петра Николаевича", true},
		{"Иванов Иван Иванович", "Иванова Ивана Ивановича", true},

		// Different people — same surname, different name/patronymic
		{"Иванов Иван Иванович", "Иванова Мария Петровна", false},

		// Completely different names
		{"Малышев Иван Иванович", "Темирханова Айгуль Бековна", false},
		{"Трошов Пётр Николаевич", "Темирханова Зарина Кокеновна", false},
		{"Иванов Иван Иванович", "Петров Пётр Петрович", false},

		// Same-length declension: last char changes (й→ю, й→я).
		// Covers dative "Тимофею" vs nominative "Тимофей" — same byte length,
		// first n-1 runes identical.
		{"Малышев Тимофей Евгеньевич", "Малышеву Тимофею Евгеньевичу", true},
		{"Малышев Тимофей Евгеньевич", "Малышева Тимофея Евгеньевича", true},

		// Word count mismatch
		{"Иванов Иван", "Иванов Иван Иванович", false},

		// Empty strings
		{"", "", true},
		{"Иванов Иван Иванович", "", false},

		// Suffix longer than 3 runes — should NOT merge
		{"Ли Ин Хун", "Лиабвгд Ин Хун", false},
	}

	for _, tc := range tests {
		t.Run(tc.a+"|"+tc.b, func(t *testing.T) {
			got := namesSimilar(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("namesSimilar(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// ── groupByStudent ────────────────────────────────────────────────────────────

func page(n int, path string) models.Page {
	return models.Page{Number: n, PDFPath: path}
}

func parsed(n int, name, group string) models.ParsedPage {
	return models.ParsedPage{
		Page:     page(n, "p"+string(rune('0'+n))+".pdf"),
		FullName: name,
		Group:    group,
	}
}

func parsedNoName(n int) models.ParsedPage {
	return models.ParsedPage{Page: page(n, "p"+string(rune('0'+n))+".pdf")}
}

// Cover-page heuristic: unnamed page immediately before a named page is treated
// as the cover page of that student's section (→ assigned to the following student).
//
// Layout: [Иванов-title(0)][unnamed(1) ← cover of Петров][Петров-title(2)][unnamed(3) ← trailing/Петров nearest]
func TestGroupByStudent_Basic(t *testing.T) {
	pages := []models.ParsedPage{
		parsed(1, "Иванов Иван Иванович", "РИ-330001"),
		parsedNoName(2), // immediately before Петров → cover-page heuristic → Петров
		parsed(3, "Петров Пётр Петрович", "РИ-330002"),
		parsedNoName(4), // nearest anchor Петров(idx2, dist=1) → Петров
	}
	students, orphans := groupByStudent(pages, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if len(orphans) != 0 {
		t.Fatalf("expected 0 orphans, got %d", len(orphans))
	}
	if len(students) != 2 {
		t.Fatalf("expected 2 students, got %d", len(students))
	}
	// Иванов: only his own title page (idx 0).
	// Петров: cover(idx 1) + title(idx 2) + trailing(idx 3).
	if students[0].FullName != "Иванов Иван Иванович" || len(students[0].Pages) != 1 {
		t.Errorf("student[0] wrong (want 1 page): %+v", students[0])
	}
	if students[1].FullName != "Петров Пётр Петрович" || len(students[1].Pages) != 3 {
		t.Errorf("student[1] wrong (want 3 pages): %+v", students[1])
	}
}

// Pages with different declensions of the same name must land in one bucket.
func TestGroupByStudent_DeclensionMerge(t *testing.T) {
	pages := []models.ParsedPage{
		parsed(1, "Малышев Иван Иванович", "РИ-330001"),
		parsedNoName(2),
		parsed(3, "Малышева Ивана Ивановича", ""), // same person, genitive
		parsedNoName(4),
	}
	students, orphans := groupByStudent(pages, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if len(orphans) != 0 {
		t.Fatalf("expected 0 orphans, got %d", len(orphans))
	}
	if len(students) != 1 {
		t.Fatalf("expected 1 student after merge, got %d: %v", len(students), studentNames(students))
	}
	if len(students[0].Pages) != 4 {
		t.Errorf("expected 4 pages, got %d", len(students[0].Pages))
	}
	// Canonical form should be the shorter (nominative) name.
	if students[0].FullName != "Малышев Иван Иванович" {
		t.Errorf("canonical name wrong: %q", students[0].FullName)
	}
}

// Leading nameless pages appear before the student's title page (nearest-anchor
// assigns them to the only named student found).
func TestGroupByStudent_LeadingPages(t *testing.T) {
	pages := []models.ParsedPage{
		parsedNoName(1),
		parsedNoName(2),
		parsed(3, "Иванов Иван Иванович", "РИ-330001"),
	}
	students, orphans := groupByStudent(pages, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if len(orphans) != 0 {
		t.Fatalf("expected 0 orphans with nearest-anchor, got %d", len(orphans))
	}
	if len(students) != 1 {
		t.Fatalf("expected 1 student, got %d", len(students))
	}
	if len(students[0].Pages) != 3 {
		t.Errorf("expected 3 pages, got %d", len(students[0].Pages))
	}
}

// Completely nameless PDF → all orphans.
func TestGroupByStudent_AllOrphans(t *testing.T) {
	pages := []models.ParsedPage{parsedNoName(1), parsedNoName(2)}
	_, orphans := groupByStudent(pages, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if len(orphans) != 2 {
		t.Fatalf("expected 2 orphans, got %d", len(orphans))
	}
}

// Cover-page heuristic in action: unnamed page immediately before Трошов's title
// is treated as Трошов's cover page — NOT the trailing body of Темирханова.
func TestGroupByStudent_InterleavedAttachment(t *testing.T) {
	pages := []models.ParsedPage{
		parsed(1, "Темирханова Зарина Кокеновна", "РИ-330001"), // idx 0
		parsedNoName(2), // idx 1: immediately before Трошов → cover heuristic → Трошов
		parsed(3, "Трошов Пётр Николаевич", "РИ-330002"), // idx 2
		parsedNoName(4), // idx 3: nearest anchor Трошов(idx2, dist=1) → Трошов
	}
	students, _ := groupByStudent(pages, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if len(students) != 2 {
		t.Fatalf("expected 2 students, got %d", len(students))
	}
	tema, tros := students[0], students[1]
	// Темирханова gets only her own title page.
	if len(tema.Pages) != 1 {
		t.Errorf("Темирханова: want 1 page, got %d", len(tema.Pages))
	}
	// Трошов gets: cover(idx1) + title(idx2) + trailing(idx3).
	if len(tros.Pages) != 3 {
		t.Errorf("Трошов: want 3 pages, got %d", len(tros.Pages))
	}
}

// Nearest-anchor splits unnamed pages correctly between two students.
//
// Layout (4 pages):
//
//	idx0: Темирханова title
//	idx1: unnamed  → dist(0)=1 < dist(3)=2  → Темирханова
//	idx2: unnamed  → dist(0)=2 > dist(3)=1  → Трошов   ← key: goes to the FOLLOWING student
//	idx3: Трошов title
func TestGroupByStudent_UnnamedBeforeTitle(t *testing.T) {
	pages := []models.ParsedPage{
		parsed(1, "Темирханова Зарина Кокеновна", "РИ-330001"), // idx 0
		parsedNoName(2), // idx 1 → Темирханова (dist 1 vs 2)
		parsedNoName(3), // idx 2 → Трошов    (dist 1 vs 2)
		parsed(4, "Трошов Пётр Николаевич", "РИ-330002"), // idx 3
	}
	students, _ := groupByStudent(pages, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if len(students) != 2 {
		t.Fatalf("expected 2 students, got %d", len(students))
	}
	tema, tros := students[0], students[1]
	if tema.FullName != "Темирханова Зарина Кокеновна" {
		t.Errorf("student[0]: %q", tema.FullName)
	}
	if tros.FullName != "Трошов Пётр Николаевич" {
		t.Errorf("student[1]: %q", tros.FullName)
	}
	// Nearest-anchor: idx1→Темирханова, idx2→Трошов.
	if len(tema.Pages) != 2 {
		t.Errorf("Темирханова: want 2 pages (idx0+idx1), got %d", len(tema.Pages))
	}
	if len(tros.Pages) != 2 {
		t.Errorf("Трошов: want 2 pages (idx2+idx3), got %d", len(tros.Pages))
	}
}

// Unnamed page just before the next student's title is closer to that student →
// must NOT end up in the preceding student's file.
func TestGroupByStudent_UnnamedCloserToNextStudent(t *testing.T) {
	// Layout: Темирханова(0) … body(1) … body(2) … Трошов-body(3) … Трошов-title(4)
	// Page 3 is distance 1 from Трошов's title and distance 3 from Темирханова's.
	pages := []models.ParsedPage{
		parsed(1, "Темирханова Зарина Кокеновна", "РИ-330001"), // idx 0
		parsedNoName(2), // idx 1 → Темирханова (dist 1 vs 3)
		parsedNoName(3), // idx 2 → Темирханова (dist 2 vs 2, tie→preceding)
		parsedNoName(4), // idx 3 → Трошов (dist 3 vs 1) ← key assertion
		parsed(5, "Трошов Пётр Николаевич", "РИ-330002"), // idx 4
	}
	students, _ := groupByStudent(pages, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if len(students) != 2 {
		t.Fatalf("expected 2 students, got %d", len(students))
	}
	tema, tros := students[0], students[1]
	if len(tema.Pages) != 3 {
		t.Errorf("Темирханова: want 3 pages, got %d (pages should be idx 0,1,2)", len(tema.Pages))
	}
	if len(tros.Pages) != 2 {
		t.Errorf("Трошов: want 2 pages (idx 3+4), got %d", len(tros.Pages))
	}
}

// Group back-fill: a nameless page that carries a group code should fill in
// the student's group if it was unknown.
func TestGroupByStudent_GroupBackfill(t *testing.T) {
	pages := []models.ParsedPage{
		parsed(1, "Иванов Иван Иванович", ""), // no group on first page
		{
			Page:  page(2, "p2.pdf"),
			Group: "РИ-330001", // group appears on second page
		},
	}
	students, _ := groupByStudent(pages, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if len(students) != 1 {
		t.Fatalf("expected 1 student, got %d", len(students))
	}
	if students[0].Group != "РИ-330001" {
		t.Errorf("group not back-filled: %q", students[0].Group)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func studentNames(ss []models.Student) []string {
	names := make([]string, len(ss))
	for i, s := range ss {
		names[i] = s.FullName
	}
	return names
}
