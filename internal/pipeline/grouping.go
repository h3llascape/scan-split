package pipeline

import (
	"fmt"
	"strings"

	"github.com/hellascape/scansplit/internal/models"
)

type (
	anchor struct {
		idx   int
		name  string
		group string
	}
	bucket struct {
		canonical string // shortest (likely nominative) name seen
		group     string
		pages     []models.Page
	}
)

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
func (p *Pipeline) groupByStudent(pages []models.ParsedPage) (students []models.Student, orphans []models.ParsedPage) {
	var anchors []anchor
	for i, pp := range pages {
		if pp.FullName != "" {
			anchors = append(anchors, anchor{i, pp.FullName, pp.Group})
			p.logger.Debug("anchor found", "page", pp.Page.Number, "name", pp.FullName, "group", pp.Group, "confidence", pp.Confidence)
		}
	}

	if len(anchors) == 0 {
		p.logger.Debug("no anchors — all pages are orphans", "count", len(pages))
		return nil, pages
	}

	buckets := make(map[string]*bucket, len(anchors))
	var order []string

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
				bestDist := abs(i - best.idx)
				for _, a := range anchors[1:] {
					if d := abs(i - a.idx); d < bestDist {
						bestDist = d
						best = a
					}
				}
				name, group = best.name, best.group
				reason = fmt.Sprintf("nearest-anchor (dist=%d)", bestDist)
			}
		}

		p.logger.Debug("page assigned",
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

	students = make([]models.Student, 0, len(order))
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
		// other word. Handles "А. Е." form vs "Алексей Евгеньевич" in same-length
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
// (e.g. "А.", "И.В."). Used to prefer full names over abbreviated forms.
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

var yoReplacer = strings.NewReplacer("ё", "е", "Ё", "Е")

// normaliseYo replaces 'ё'/'Ё' with 'е'/'Е' so that OCR variants and
// declension-driven vowel shifts (Пётр→Петра) don't break prefix matching.
func normaliseYo(s string) string {
	return yoReplacer.Replace(s)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
