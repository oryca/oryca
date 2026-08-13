package tool

import "time"

const utcLayout = "2006-01-02T15:04:05.000Z"

// NowUTC returns the current time in UTC truncated to millisecond precision.
func NowUTC() time.Time {
	ts := time.Now().UTC()
	parsed, err := time.Parse(utcLayout, ts.Format(utcLayout))
	if err != nil {
		return ts
	}
	return parsed
}
