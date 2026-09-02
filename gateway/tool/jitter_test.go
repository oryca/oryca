package tool

import (
	"testing"
	"time"
)

func TestJitteredInterval(t *testing.T) {
	interval := 100 * time.Second
	min := 80 * time.Second
	max := 120 * time.Second

	for i := 0; i < 1000; i++ {
		got := JitteredInterval(interval)
		if got < min || got > max {
			t.Fatalf("JitteredInterval(%v) = %v, want within [%v, %v]", interval, got, min, max)
		}
	}
}
