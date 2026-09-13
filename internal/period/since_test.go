package period

import (
	"testing"
	"time"
)

func TestParseSinceDays(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 12, 22, 0, 0, 0, time.UTC)

	got, err := ParseSince("30d", now)
	if err != nil {
		t.Fatalf("ParseSince() error = %v", err)
	}

	want := now.AddDate(0, 0, -30)
	if !got.Equal(want) {
		t.Fatalf("ParseSince() = %v, want %v", got, want)
	}
}

func TestParseSinceDate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 12, 22, 0, 0, 0, time.UTC)

	got, err := ParseSince("2026-09-01", now)
	if err != nil {
		t.Fatalf("ParseSince() error = %v", err)
	}

	want := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("ParseSince() = %v, want %v", got, want)
	}
}

func TestParseSinceRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 12, 22, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		input string
	}{
		{name: "zero days", input: "0d"},
		{name: "negative days", input: "-5d"},
		{name: "missing days suffix", input: "30"},
		{name: "impossible date", input: "2026-02-30"},
		{name: "future date", input: "2026-09-13"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParseSince(tt.input, now); err == nil {
				t.Fatal("ParseSince() error = nil, want error")
			}
		})
	}
}
