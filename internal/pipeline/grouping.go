package pipeline

import (
	"fmt"
	"slices"
	"strings"

	"github.com/h3llascape/scan-split/internal/models"
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
	// If a whitelist is provided, use AllNames to find the best whitelisted
	// candidate for each page. Pages with no whitelisted candidate are excluded.
	// Unnamed pages near non-whitelisted names are also excluded.
	excluded := make(map[int]bool)
	if len(p.cfg.Whitelist) > 0 {
		// Build original anchor list for proximity check on unnamed pages.
		var origAnchors []anchor
		for i, pp := range pages {
			if pp.FullName != "" {
				origAnchors = append(origAnchors, anchor{i, pp.FullName, pp.Group})
			}
		}

		filtered := make([]models.ParsedPage, len(pages))
		copy(filtered, pages)
		for i, pp := range filtered {
			switch {
			case pp.FullName != "":
				// Check AllNames for any whitelisted candidate.
				if alt := p.firstWhitelistedName(pp.AllNames); alt != "" {
					if alt != pp.FullName {
						p.logger.Debug("page reassigned to whitelisted name",
							"page", pp.Page.Number, "original_name", pp.FullName, "reassigned_to", alt)
						filtered[i].FullName = alt
						filtered[i].Group = "" // group likely belongs to the original name
					}
					// else: FullName is already whitelisted, keep as-is
				} else {
					p.logger.Debug("no whitelisted name among candidates, excluding page",
						"page", pp.Page.Number, "primary_name", pp.FullName)
					filtered[i].FullName = ""
					filtered[i].Group = ""
					excluded[i] = true
				}

			case len(pp.AllNames) > 0:
				// Page had no primary name but has lower-priority candidates
				// (e.g. abbreviated names found in text). Check them.
				if alt := p.firstWhitelistedName(pp.AllNames); alt != "" {
					p.logger.Debug("unnamed page matched whitelisted candidate",
						"page", pp.Page.Number, "matched", alt)
					filtered[i].FullName = alt
					filtered[i].Group = ""
				} else if len(origAnchors) > 0 {
					best := nearestAnchor(i, origAnchors)
					if !p.isWhitelisted(best.name) {
						p.logger.Debug("unnamed page nearest to non-whitelisted name, excluding",
							"page", pp.Page.Number, "nearest", best.name)
						excluded[i] = true
					}
				}

			default:
				// Truly unnamed page (no candidates at all): check nearest anchor.
				if len(origAnchors) > 0 {
					best := nearestAnchor(i, origAnchors)
					if !p.isWhitelisted(best.name) {
						p.logger.Debug("unnamed page nearest to non-whitelisted name, excluding",
							"page", pp.Page.Number, "nearest", best.name)
						excluded[i] = true
					}
				}
			}
		}
		pages = filtered
	}

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
		if excluded[i] {
			p.logger.Debug("page excluded (non-whitelisted name)", "page", pp.Page.Number)
			continue
		}

		name, group := pp.FullName, pp.Group
		reason := "own name"

		if name == "" {
			// Find nearest preceding anchor distance.
			prevDist := len(pages) // sentinel: no preceding anchor
			for _, a := range anchors {
				if a.idx < i {
					if d := i - a.idx; d < prevDist {
						prevDist = d
					}
				}
			}

			// Cover-page heuristic: if the very next page carries a FULL name,
			// this may be the cover page of that student's section — assign it
			// forward. Only fires when no preceding anchor is adjacent (dist ≤ 1),
			// otherwise the page is more likely a trailing page of the preceding
			// student. Also skips abbreviated forms like "Трошов И.В." to avoid
			// stealing trailing pages.
			if prevDist > 1 &&
				i+1 < len(pages) && pages[i+1].FullName != "" && !hasInitials(pages[i+1].FullName) {
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
		// prefer the shorter string (likely nominative case), but not if the
		// difference is too large (> 4 runes) — that indicates OCR truncation
		// like "Серг" instead of "Сергеевна", not a declension variant.
		existingAbbrev := hasInitials(b.canonical)
		newAbbrev := hasInitials(name)
		if (!newAbbrev && existingAbbrev) ||
			(newAbbrev == existingAbbrev && len(name) < len(b.canonical) &&
				len([]rune(b.canonical))-len([]rune(name)) <= 4) {
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

// normaliseNameWords splits a name into lowercase words after normalising ё→е,
// stripping trailing commas, and expanding combined initials (м.ю. → м. + ю.).
func normaliseNameWords(s string) []string {
	s = normaliseYo(strings.ToLower(s))
	words := strings.Fields(s)
	var result []string
	for _, w := range words {
		w = strings.TrimRight(w, ",")
		if w == "" {
			continue
		}
		r := []rune(w)
		// Expand combined initials: "м.ю." → ["м.", "ю."]
		if len(r) == 4 && isCyrillicLower(r[0]) && r[1] == '.' && isCyrillicLower(r[2]) && r[3] == '.' {
			result = append(result, string(r[:2]), string(r[2:]))
		} else {
			result = append(result, w)
		}
	}
	return result
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
	wa := normaliseNameWords(a)
	wb := normaliseNameWords(b)

	// When word counts differ, trim trailing bare single Cyrillic letters
	// (OCR junk like "А" in "Свидер М.Ю. А") from the longer list.
	if len(wa) != len(wb) {
		trim := func(words []string, target int) []string {
			for len(words) > target {
				r := []rune(words[len(words)-1])
				if len(r) == 1 && isCyrillicLower(r[0]) {
					words = words[:len(words)-1]
				} else {
					break
				}
			}
			return words
		}
		if len(wa) > len(wb) {
			wa = trim(wa, len(wb))
		} else {
			wb = trim(wb, len(wa))
		}
	}

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

		// Case 1: one word is a prefix of the other.
		// Suffix ≤ 3 covers declension (Малышев→Малышева, Евгеньевичу).
		// When the shorter word has ≥ 4 runes we accept any suffix length
		// to handle OCR truncation (Серг→Сергеевна).
		short, long := ra, rb
		if len(short) > len(long) {
			short, long = long, short
		}
		if string(long[:len(short)]) == string(short) &&
			(len(long)-len(short) <= 3 || len(short) >= 4) {
			continue
		}

		// Case 2: same-length words differing only in the last rune.
		// Handles Тимофей→Тимофею, Тимофей→Тимофея (й→ю/я declension).
		if len(ra) == len(rb) && len(ra) > 2 && string(ra[:len(ra)-1]) == string(rb[:len(rb)-1]) {
			continue
		}

		// Case 3: shared stem with both sides having short suffixes.
		// Handles Ивченкова→Ивченковой (stem "ивченков", suffixes "а" and "ой").
		minLen := len(short)
		commonPrefix := 0
		for commonPrefix < minLen && short[commonPrefix] == long[commonPrefix] {
			commonPrefix++
		}
		if commonPrefix >= 2 && len(short)-commonPrefix <= 3 && len(long)-commonPrefix <= 3 {
			continue
		}

		// Case 4: single-letter initial matching the first letter of the other word.
		// Handles "Задевалов А. Е." vs "Задевалов Алексей Евгеньевич".
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
	return slices.ContainsFunc(parts[1:], isLowercaseInitial)
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

// firstWhitelistedName returns the first name from candidates that matches
// the whitelist, or empty string if none match.
func (p *Pipeline) firstWhitelistedName(candidates []string) string {
	for _, name := range candidates {
		if p.isWhitelisted(name) {
			return name
		}
	}
	return ""
}

// nearestAnchor returns the anchor closest to index i.
// On a tie, the first (preceding) anchor wins.
func nearestAnchor(i int, anchors []anchor) anchor {
	best := anchors[0]
	bestDist := abs(i - best.idx)
	for _, a := range anchors[1:] {
		if d := abs(i - a.idx); d < bestDist {
			bestDist = d
			best = a
		}
	}
	return best
}

// isWhitelisted reports whether name fuzzy-matches any entry in the whitelist.
// Returns true if the whitelist is empty (no filtering).
func (p *Pipeline) isWhitelisted(name string) bool {
	if len(p.cfg.Whitelist) == 0 {
		return true
	}
	nameWords := strings.Fields(name)
	for _, w := range p.cfg.Whitelist {
		if namesSimilar(name, w) {
			return true
		}
		// Partial match: if whitelist entry has fewer words than OCR name,
		// compare only the first N words (surname-only or surname+name).
		// "Трошов" matches "Трошов Илья Владимирович" / "Трошова Ильи Владимировича".
		wWords := strings.Fields(w)
		if len(wWords) > 0 && len(wWords) < len(nameWords) {
			namePrefix := strings.Join(nameWords[:len(wWords)], " ")
			if namesSimilar(namePrefix, w) {
				return true
			}
		}
	}
	return false
}
