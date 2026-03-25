package pipeline

import "testing"

func TestSafeFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fullName string
		group    string
		want     string
	}{
		{
			name:     "typical full name",
			fullName: "Иванов Иван Иванович",
			group:    "РИ-330942",
			want:     "Иванов_Иван_Иванович_РИ-330942",
		},
		{
			name:     "name with extra spaces",
			fullName: "  Иванов   Иван  ",
			group:    "РИ-330942",
			want:     "Иванов_Иван_РИ-330942",
		},
		{
			name:     "illegal chars replaced",
			fullName: "Иванов/Иван:Иванович",
			group:    "РИ<330942>",
			want:     "Иванов_Иван_Иванович_РИ_330942_",
		},
		{
			name:     "windows-illegal chars",
			fullName: `Иванов "Иван" *Иванович*`,
			group:    "РИ-330942",
			want:     "Иванов__Иван___Иванович__РИ-330942",
		},
		{
			name:     "empty group",
			fullName: "Петров Пётр Петрович",
			group:    "",
			want:     "Петров_Пётр_Петрович_",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := safeFileName(tc.fullName, tc.group)
			if got != tc.want {
				t.Errorf("safeFileName(%q, %q) = %q, want %q", tc.fullName, tc.group, got, tc.want)
			}
		})
	}
}
