package tool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNowUTC(t *testing.T) {
	t.Run("คืนค่า UTC timezone", func(t *testing.T) {
		now := NowUTC()
		assert.Equal(t, time.UTC, now.Location())
	})

	t.Run("ค่าใกล้เคียงเวลาปัจจุบัน", func(t *testing.T) {
		before := time.Now().UTC()
		now := NowUTC()
		after := time.Now().UTC()

		assert.False(t, now.Before(before.Add(-time.Second)))
		assert.False(t, now.After(after.Add(time.Second)))
	})

	t.Run("ตัด precision เหลือ millisecond (ไม่มี microsecond)", func(t *testing.T) {
		now := NowUTC()
		assert.Equal(t, 0, now.Nanosecond()%1_000_000)
	})
}
