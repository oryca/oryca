package tool

import (
	"math/rand/v2"
	"time"
)

func JitteredInterval(interval time.Duration) time.Duration {
	jitter := time.Duration(rand.Int64N(int64(interval)*2/5)) - time.Duration(int64(interval)/5)
	return interval + jitter
}
