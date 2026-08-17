package consumer

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ยิงเข้าด้วย JSON ชุดเดียวกับที่ gateway publish จริง เพื่อคุมว่าสองฝั่งไม่หลุดกัน
const gatewayPayload = `{
  "time": "2026-08-15T10:19:35.435Z",
  "level": "INFO",
  "message": "proxy",
  "traceId": "trace-1",
  "userId": "user-1",
  "apiKeyId": "key-1",
  "serviceId": "svc-1",
  "packageId": "pkg-1",
  "cacheStatus": "MISS",
  "request": { "host": "h", "ip": "1.2.3.4", "method": "GET", "path": "/p" },
  "response": {
    "statusCode": 200, "size": 17011, "duration": 253,
    "timing": {
      "authMs": 1, "rateLimitMs": 5, "cacheCheckMs": 6,
      "singleflightMs": 2, "bodyReadMs": 3, "postUpstreamMs": 4
    }
  },
  "target": { "request": { "method": "GET" }, "response": { "statusCode": 200, "duration": 234 } }
}`

func TestDecodeMessageKeepsLatencyFields(t *testing.T) {
	doc := decodeMessage(redis.XMessage{ID: "1-0", Values: map[string]any{"log": gatewayPayload}})
	require.NotNil(t, doc)

	assert.Equal(t, "pkg-1", doc.PackageID)
	assert.Equal(t, "MISS", doc.CacheStatus)

	assert.Equal(t, int64(253), doc.DurationMs)
	assert.Equal(t, int64(234), doc.UpstreamDurationMs)
	assert.Equal(t, 200, doc.UpstreamStatus)
	assert.Equal(t, int64(19), doc.DurationMs-doc.UpstreamDurationMs, "gateway overhead")

	assert.Equal(t, int64(1), doc.AuthMs)
	assert.Equal(t, int64(5), doc.RateLimitMs)
	assert.Equal(t, int64(6), doc.CacheCheckMs)
	assert.Equal(t, int64(2), doc.SingleflightMs)
	assert.Equal(t, int64(3), doc.BodyReadMs)
	assert.Equal(t, int64(4), doc.PostUpstreamMs)
}

// log เก่าที่ค้างอยู่ใน stream ไม่มี timing กับ target ต้องไม่ทำให้ decode พัง
func TestDecodeMessageAcceptsOlderPayload(t *testing.T) {
	old := `{"time":"2026-08-15T10:19:35.435Z","traceId":"t","response":{"statusCode":200,"duration":10}}`
	doc := decodeMessage(redis.XMessage{ID: "1-0", Values: map[string]any{"log": old}})
	require.NotNil(t, doc)

	assert.Equal(t, int64(10), doc.DurationMs)
	assert.Zero(t, doc.UpstreamDurationMs)
	assert.Zero(t, doc.UpstreamStatus)
	assert.Zero(t, doc.AuthMs)
}
