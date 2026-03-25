package pipeline

import "testing"

// ── normaliseYo ──────────────────────────────────────────────────────────────

func TestNormaliseYo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"Пётр", "Петр"},
		{"Фёдоров", "Федоров"},
		{"ёж", "еж"},
		{"Ёж", "Еж"},
		{"без ё", "без е"},
		{"Иванов", "Иванов"},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := normaliseYo(tc.input)
			if got != tc.want {
				t.Errorf("normaliseYo(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ── abs ──────────────────────────────────────────────────────────────────────

func TestAbs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int
		want  int
	}{
		{0, 0},
		{5, 5},
		{-3, 3},
		{-1, 1},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			if got := abs(tc.input); got != tc.want {
				t.Errorf("abs(%d) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

// ── isLowercaseInitial ───────────────────────────────────────────────────────

func TestIsLowercaseInitial(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		// Valid initials (already lowercased)
		{"а", true},
		{"а.", true},
		{"а.б.", true},

		// ё counts as Cyrillic lower
		{"ё", true},
		{"ё.", true},

		// Not initials
		{"ab", false},   // Latin
		{"аб", false},   // two Cyrillic letters, no dots
		{"а.б", false},  // missing trailing dot
		{"abc", false},  // Latin 3 chars
		{"", false},     // empty
		{"А", false},    // uppercase
		{"а.б.в.", false}, // too long
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := isLowercaseInitial(tc.input)
			if got != tc.want {
				t.Errorf("isLowercaseInitial(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ── hasInitials ──────────────────────────────────────────────────────────────

func TestHasInitials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"full name", "Иванов Иван Иванович", false},
		{"abbreviated", "Иванов И.В.", true},
		{"abbreviated with spaces", "Иванов И. В.", true},
		{"single word", "Иванов", false},
		{"empty", "", false},
		{"first word is initial", "и. Иванов Иванович", false}, // first word skipped (surname)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := hasInitials(tc.input)
			if got != tc.want {
				t.Errorf("hasInitials(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ── namesSimilar (additional cases beyond pipeline_test.go) ──────────────────

func TestNamesSimilar_AbbreviatedForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{
			name: "abbreviated 2-word vs full 3-word same person",
			a:    "Котляров Н.",
			b:    "Котляров Николай Алексеевич",
			want: true,
		},
		{
			name: "abbreviated 2-word vs full 3-word different initial",
			a:    "Котляров А.",
			b:    "Котляров Николай Алексеевич",
			want: false,
		},
		{
			name: "same-length initials vs full words",
			a:    "Задевалов А. Е.",
			b:    "Задевалов Алексей Евгеньевич",
			want: true,
		},
		{
			name: "initials mismatch second word",
			a:    "Задевалов А. М.",
			b:    "Задевалов Алексей Евгеньевич",
			want: false,
		},
		{
			name: "yo normalisation",
			a:    "Фёдоров Пётр Николаевич",
			b:    "Федоров Петр Николаевич",
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := namesSimilar(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("namesSimilar(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
