package pipeline

import (
	"log/slog"
	"testing"
)

func TestNew_DefaultConcurrency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      int
		wantOutput int
	}{
		{"zero defaults to 4", 0, 4},
		{"negative defaults to 4", -1, 4},
		{"positive kept as-is", 8, 8},
		{"one kept as-is", 1, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := New(Config{Concurrency: tc.input}, nil, slog.Default())
			if p.cfg.Concurrency != tc.wantOutput {
				t.Errorf("New(Concurrency=%d).cfg.Concurrency = %d, want %d",
					tc.input, p.cfg.Concurrency, tc.wantOutput)
			}
		})
	}
}
