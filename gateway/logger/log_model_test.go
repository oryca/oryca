package logger

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeLog(t *testing.T, f ProxyLogFields) map[string]any {
	t.Helper()
	raw, err := BuildProxyLogJSON(f)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestBuildProxyLogJSONCarriesTiming(t *testing.T) {
	got := decodeLog(t, ProxyLogFields{
		TotalMs:        253,
		StatusCode:     200,
		UpstreamStatus: 200,
		UpstreamMs:     234,
		CacheStatus:    "MISS",
		PackageID:      "pkg1",
		AuthMs:         1,
		RateLimitMs:    5,
		CacheCheckMs:   6,
		SingleflightMs: 2,
		BodyReadMs:     3,
		PostUpstreamMs: 4,
	})

	assert.Equal(t, "MISS", got["cacheStatus"])
	assert.Equal(t, "pkg1", got["packageId"])

	resp := got["response"].(map[string]any)
	timing := resp["timing"].(map[string]any)
	assert.Equal(t, float64(1), timing["authMs"])
	assert.Equal(t, float64(5), timing["rateLimitMs"])
	assert.Equal(t, float64(6), timing["cacheCheckMs"])
	assert.Equal(t, float64(2), timing["singleflightMs"])
	assert.Equal(t, float64(3), timing["bodyReadMs"])
	assert.Equal(t, float64(4), timing["postUpstreamMs"])
}

// เหตุผลหลักที่เพิ่ม target เข้ามา คือหา overhead ของ gateway ได้จาก
// response.duration - target.response.duration โดยไม่ต้องเดา
func TestBuildProxyLogJSONCarriesTargetForComparison(t *testing.T) {
	got := decodeLog(t, ProxyLogFields{
		Method:         "GET",
		StatusCode:     200,
		TotalMs:        253,
		UpstreamStatus: 200,
		UpstreamMs:     234,
	})

	target := got["target"].(map[string]any)
	tr := target["response"].(map[string]any)
	assert.Equal(t, float64(200), tr["statusCode"])
	assert.Equal(t, float64(234), tr["duration"])
	assert.Equal(t, "GET", target["request"].(map[string]any)["method"])

	total := got["response"].(map[string]any)["duration"].(float64)
	assert.Equal(t, float64(19), total-tr["duration"].(float64), "gateway overhead")
}

// cache HIT ไม่ได้ยิง upstream จึงต้องไม่มี target ให้เข้าใจผิดว่า upstream ตอบใน 0ms
func TestBuildProxyLogJSONOmitsTargetWhenUpstreamNotCalled(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    ProxyLogFields
	}{
		{"cache hit", ProxyLogFields{StatusCode: 200, TotalMs: 7, CacheStatus: "HIT"}},
		{"rejected at auth", ProxyLogFields{StatusCode: 401, TotalMs: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeLog(t, tc.f)
			_, hasTarget := got["target"]
			assert.False(t, hasTarget, "target must be absent when nothing was proxied")
		})
	}
}

// credential ต้องไม่หลุดลง log เพราะ CE ไม่มีตัว mask
func TestBuildProxyLogJSONKeepsCredentialsOut(t *testing.T) {
	raw, err := BuildProxyLogJSON(ProxyLogFields{
		Path:           "/gateway/api/resources/svc/items",
		UpstreamURL:    "https://upstream.example/items?api_key=SUPERSECRET",
		UpstreamStatus: 200,
		UpstreamMs:     10,
	})
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "SUPERSECRET", "upstream credential leaked into the log")
	assert.NotContains(t, string(raw), "api_key")
	assert.NotContains(t, string(raw), "upstream.example", "upstream address is not logged")
}
