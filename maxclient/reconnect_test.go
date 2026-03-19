package maxclient

import (
	"testing"
	"time"
)

func TestBackoffCalculation(t *testing.T) {
	min := 1 * time.Second
	max := 30 * time.Second

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second}, // capped
		{6, 30 * time.Second}, // stays capped
	}

	for _, tt := range tests {
		got := calcBackoff(tt.attempt, min, max)
		if got != tt.expected {
			t.Errorf("backoff(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}
