package tool

import (
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	cases := []struct {
		name     string
		delay    time.Duration
		maxDelay time.Duration
		want     time.Duration
	}{
		{"doubles below cap", 5 * time.Second, 30 * time.Second, 10 * time.Second},
		{"doubles right up to cap", 15 * time.Second, 30 * time.Second, 30 * time.Second},
		{"stays once delay equals cap", 30 * time.Second, 30 * time.Second, 30 * time.Second},
		// delay < maxDelay ยังจริง (20<30) เลย double ได้อีกครั้งแม้ผลลัพธ์จะเกิน cap ไปแล้ว —
		// พฤติกรรมเดิมของทั้งสองที่ที่เคยเขียนแยกกัน (ไม่ clamp ผลลัพธ์ ใช้แค่เงื่อนไขว่า "ยังต่ำกว่า cap อยู่ไหมก่อน double")
		{"delay below cap still doubles even past cap", 20 * time.Second, 30 * time.Second, 40 * time.Second},
		{"steady state once delay exceeds cap", 40 * time.Second, 30 * time.Second, 40 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NextBackoff(c.delay, c.maxDelay)
			if got != c.want {
				t.Errorf("NextBackoff(%v, %v) = %v, want %v", c.delay, c.maxDelay, got, c.want)
			}
		})
	}
}
