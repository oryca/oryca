package tool

import "time"

// NextBackoff doubles delay up to maxDelay. Used by reconnect loops
// that back off exponentially between retries instead of hammering a down dependency.
func NextBackoff(delay, maxDelay time.Duration) time.Duration {
	if delay < maxDelay {
		delay *= 2
	}
	return delay
}
