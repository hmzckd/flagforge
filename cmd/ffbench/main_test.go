package main

import (
	"testing"
	"time"
)

func TestPercentileUsesTheNearestRank(t *testing.T) {
	var durations []time.Duration
	for i := 100; i >= 1; i-- {
		durations = append(durations, time.Duration(i)*time.Millisecond)
	}

	tests := []struct {
		p    float64
		want time.Duration
	}{
		{50, 50 * time.Millisecond},
		{95, 95 * time.Millisecond},
		{100, 100 * time.Millisecond},
		{0, 1 * time.Millisecond},
	}
	for _, tt := range tests {
		if got := percentile(durations, tt.p); got != tt.want {
			t.Errorf("p%v = %v, want %v", tt.p, got, tt.want)
		}
	}
	if durations[0] != 100*time.Millisecond {
		t.Fatal("percentile must not reorder the caller's slice")
	}
}

func TestPercentileOfNothingIsZero(t *testing.T) {
	if got := percentile(nil, 50); got != 0 {
		t.Fatalf("got %v", got)
	}
}
