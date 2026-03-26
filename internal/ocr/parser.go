package ocr

import (
	"regexp"
	"strings"
	"unicode"
)

// ParseResult holds student data extracted from OCR text.
type ParseResult struct {
	FullName   string   // Best candidate: "Иванов Иван Иванович"
	AllNames   []string // All name candidates found, ranked by priority
	Group      string   // "РИ-330942"
	Confidence float64  // 0..1: 1.0 both found, 0.5 one found, 0.0 neither
}

// groupRe matches group codes like "РИ-330942", "МО-230115", "ИКИТ-123456".
var groupRe = regexp.MustCompile(`[А-ЯЁA-Z]{2,4}-\d{6}`)

// nameStopWords — слова, которые встречаются в подписях документов и не являются
// частью ФИО студента. Их наличие в трёхсловной последовательности исключает её
// из кандидатов на имя.
var nameStopWords = map[string]bool{
	"Студент":        true,
	"Студентка":      true,
	"Подпись":        true,
	"Полпись":        true, // частая ошибка OCR вместо «Подпись»
	"Группа":         true,
	"Кафедра":        true,
	"Специальность":  true,
	"Направление":    true,
	"Руководитель":   true,
	"Дата":           true,
	"Оценка":         true,
	"Университет":    true,
	"Институт":       true,
	"Факультет":      true,
}

// Parse extracts student info from raw OCR text.
func Parse(text string) ParseResult {
	allNames := extractNames(text)
	group := groupRe.FindString(text)

	var fullName string
	if len(allNames) > 0 {
		fullName = allNames[0]
	}

	var confidence float64
	switch {
	case fullName != "" && group != "":
		confidence = 1.0
	case fullName != "" || group != "":
		confidence = 0.5
	}

	return ParseResult{
		FullName:   fullName,
		AllNames:   allNames,
		Group:      group,
		Confidence: confidence,
	}
}

// studentPrefixes — метки строк, за которыми следует ФИО студента.
var studentPrefixes = []string{"Студент:", "Студентка:", "Студент", "Студентка"}

// extractNames collects all name candidates from the text, ranked by priority:
//  1. Names after "Студент:" / "Студентка:" labels (highest confidence).
//  2. All sequences of 3 consecutive capitalized Cyrillic words (full names).
//  3. All "Surname + initials" patterns found anywhere in the text.
//
// Duplicates are removed. The first element is the best candidate (used as FullName).
func extractNames(text string) []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	// Priority 1: labeled names after "Студент:" / "Студентка:" etc.
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range studentPrefixes {
			after, ok := strings.CutPrefix(line, prefix)
			if !ok {
				continue
			}
			candidate := strings.TrimSpace(after)
			if isFullName(candidate) {
				add(candidate)
				continue
			}
			if isAbbreviatedName(candidate) {
				add(candidate)
				continue
			}
			// OCR often appends noise after the name (e.g. "Котляров Н. А, Кол |").
			// Try the first 3 words, then 2 words, stripping trailing garbage.
			parts := strings.Fields(candidate)
			for n := 3; n >= 2; n-- {
				if n > len(parts) {
					continue
				}
				sub := strings.Join(parts[:n], " ")
				if isAbbreviatedName(sub) {
					add(sub)
					break
				}
			}
		}
	}

	// Priority 2: all sequences of 3 consecutive capitalized Cyrillic words.
	words := strings.Fields(text)
	if len(words) >= 3 {
		for i := range len(words) - 2 {
			a, b, c := words[i], words[i+1], words[i+2]
			if !isCyrillicTitle(a) || !isCyrillicTitle(b) || !isCyrillicTitle(c) {
				continue
			}
			if nameStopWords[a] || nameStopWords[b] || nameStopWords[c] {
				continue
			}
			add(a + " " + b + " " + c)
		}
	}

	// Priority 3: all abbreviated names (Surname + initials) found anywhere.
	// Surname must be ≥ 3 runes to avoid matching "Б.Н." patterns.
	for i, w := range words {
		if !isCyrillicTitle(w) || len([]rune(w)) < 3 || nameStopWords[w] {
			continue
		}
		// Try 3-word form first (Surname И. О.), then 2-word (Surname И.О.).
		if i+2 < len(words) {
			candidate3 := w + " " + words[i+1] + " " + words[i+2]
			if isAbbreviatedName(candidate3) {
				add(candidate3)
				continue // prefer longer form, skip 2-word check
			}
		}
		if i+1 < len(words) {
			candidate2 := w + " " + words[i+1]
			if isAbbreviatedName(candidate2) {
				add(candidate2)
			}
		}
	}

	return names
}

// isFullName returns true for exactly three space-separated Cyrillic title-case words.
func isFullName(s string) bool {
	parts := strings.Fields(s)
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if !isCyrillicTitle(p) {
			return false
		}
	}
	return true
}

// isCyrillicTitle returns true when s starts with an uppercase Cyrillic rune
// followed only by lowercase Cyrillic runes (min length 2).
func isCyrillicTitle(s string) bool {
	runes := []rune(s)
	if len(runes) < 2 || !unicode.IsUpper(runes[0]) || !isCyrillic(runes[0]) {
		return false
	}
	for _, r := range runes[1:] {
		if !unicode.IsLower(r) || !isCyrillic(r) {
			return false
		}
	}
	return true
}

func isCyrillic(r rune) bool {
	return (r >= 'А' && r <= 'я') || r == 'ё' || r == 'Ё'
}

// isAbbreviatedName returns true for strings like "Котляров И.Н." or "Котляров И. Н."
// (a Cyrillic surname followed by 1-2 initials, each being a single uppercase
// Cyrillic letter optionally followed by a period).
// This covers the common Russian signature-page format where only initials are used.
func isAbbreviatedName(s string) bool {
	parts := strings.Fields(s)
	// Need at least surname + 1 initial; accept up to surname + 2 initials.
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	// First word must be a proper Cyrillic surname.
	if !isCyrillicTitle(parts[0]) {
		return false
	}
	// Remaining words must be initials: А. / А.Б. / А (bare letter)
	for _, p := range parts[1:] {
		if !isInitial(p) {
			return false
		}
	}
	return true
}

// isInitial returns true for single-letter initials in the forms: А  А.  А.Б.
func isInitial(s string) bool {
	runes := []rune(s)
	switch len(runes) {
	case 1: // А
		return unicode.IsUpper(runes[0]) && isCyrillic(runes[0])
	case 2: // А.
		return unicode.IsUpper(runes[0]) && isCyrillic(runes[0]) && runes[1] == '.'
	case 4: // А.Б.
		return unicode.IsUpper(runes[0]) && isCyrillic(runes[0]) && runes[1] == '.' &&
			unicode.IsUpper(runes[2]) && isCyrillic(runes[2]) && runes[3] == '.'
	}
	return false
}
