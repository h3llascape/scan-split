package ocr

import (
	"regexp"
	"strings"
	"unicode"
)

// ParseResult holds student data extracted from OCR text.
type ParseResult struct {
	FullName   string  // "Иванов Иван Иванович"
	Group      string  // "РИ-330942"
	Confidence float64 // 0..1: 1.0 both found, 0.5 one found, 0.0 neither
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

// Parser extracts FullName and Group from OCR text.
type Parser struct{}

// NewParser creates a new Parser.
func NewParser() *Parser { return &Parser{} }

// Parse extracts student info from raw OCR text.
func (p *Parser) Parse(text string) ParseResult {
	name := extractName(text)
	group := groupRe.FindString(text)

	var confidence float64
	switch {
	case name != "" && group != "":
		confidence = 1.0
	case name != "" || group != "":
		confidence = 0.5
	}

	return ParseResult{
		FullName:   name,
		Group:      group,
		Confidence: confidence,
	}
}

// studentPrefixes — метки строк, за которыми следует ФИО студента.
var studentPrefixes = []string{"Студент:", "Студентка:", "Студент", "Студентка"}

// extractName looks for a student name in the text.
// Priority:
//  1. Lines starting with "Студент:" / "Студентка:" etc. — name is taken after the label.
//     Accepts both full 3-word names and abbreviated forms (Фамилия И.О. / Фамилия И. О.).
//  2. Fallback — three consecutive capitalized Cyrillic words not in the stop-word list.
func extractName(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range studentPrefixes {
			if after, ok := strings.CutPrefix(line, prefix); ok {
				candidate := strings.TrimSpace(after)
				// Full 3-word name takes priority.
				if isFullName(candidate) {
					return candidate
				}
				// Accept abbreviated form: Фамилия + initials (e.g. "Котляров И.Н.").
				// Only used when clearly labeled with "Студент:" to avoid false positives.
				if isAbbreviatedName(candidate) {
					return candidate
				}
				// OCR often appends noise after the name (e.g. "Котляров Н. А, Кол |").
				// Try the first 3 words, then 2 words, stripping trailing garbage.
				parts := strings.Fields(candidate)
				for n := 3; n >= 2; n-- {
					if n > len(parts) {
						continue
					}
					if isAbbreviatedName(strings.Join(parts[:n], " ")) {
						return strings.Join(parts[:n], " ")
					}
				}
			}
		}
	}

	// Fallback: scan all words for three consecutive capitalized Cyrillic words
	// that don't include known document-label stop words.
	words := strings.Fields(text)
	for i := range len(words) - 2 {
		a, b, c := words[i], words[i+1], words[i+2]
		if !isCyrillicTitle(a) || !isCyrillicTitle(b) || !isCyrillicTitle(c) {
			continue
		}
		if nameStopWords[a] || nameStopWords[b] || nameStopWords[c] {
			continue
		}
		return a + " " + b + " " + c
	}

	return ""
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
