package cache

import (
	"net/http"
	"testing"
)

func headersWith(ct, cc string) http.Header {
	h := http.Header{}
	if ct != "" {
		h.Set("Content-Type", ct)
	}
	if cc != "" {
		h.Set("Cache-Control", cc)
	}
	return h
}

func TestIsCacheableResponse(t *testing.T) {
	c := &UpstreamCache{}

	tests := []struct {
		name   string
		status int
		h      http.Header
		want   bool
	}{
		{"json ok", 200, headersWith("application/json", ""), true},
		{"image blocked", 200, headersWith("image/png", ""), false},
		{"vector tile blocked", 200, headersWith("application/x-protobuf", ""), false},
		{"mapbox vector tile blocked", 200, headersWith("application/vnd.mapbox-vector-tile", ""), false},
		{"no-store never cached", 200, headersWith("application/json", "no-store"), false},
		{"private never cached", 200, headersWith("application/json", "private"), false},
		{"5xx never cached", 502, headersWith("application/json", ""), false},
		{"404 cacheable", 404, headersWith("application/json", ""), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.IsCacheableResponse(tt.status, tt.h); got != tt.want {
				t.Errorf("IsCacheableResponse(%d, %v) = %v, want %v", tt.status, tt.h, got, tt.want)
			}
		})
	}
}

func TestUpstreamCacheEntryBinarySerialization(t *testing.T) {
	entry := &UpstreamCacheEntry{
		StatusCode: 200,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
			"X-Test":       {"value1", "value2"},
		},
		Body:   []byte("hello world binary payload"),
		ETag:   "W/etag123456",
		TTLSec: 3600,
	}

	data, err := entry.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}

	var decoded UpstreamCacheEntry
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}

	if decoded.StatusCode != entry.StatusCode {
		t.Errorf("StatusCode mismatch: got %d, want %d", decoded.StatusCode, entry.StatusCode)
	}
	if decoded.TTLSec != entry.TTLSec {
		t.Errorf("TTLSec mismatch: got %d, want %d", decoded.TTLSec, entry.TTLSec)
	}
	if decoded.ETag != entry.ETag {
		t.Errorf("ETag mismatch: got %q, want %q", decoded.ETag, entry.ETag)
	}
	if string(decoded.Body) != string(entry.Body) {
		t.Errorf("Body mismatch: got %q, want %q", string(decoded.Body), string(entry.Body))
	}
	if len(decoded.Headers) != len(entry.Headers) {
		t.Errorf("Headers count mismatch: got %d, want %d", len(decoded.Headers), len(entry.Headers))
	}
	for k, v := range entry.Headers {
		gotV, ok := decoded.Headers[k]
		if !ok {
			t.Errorf("Header %q missing in decoded", k)
			continue
		}
		if len(gotV) != len(v) {
			t.Errorf("Header %q length mismatch: got %d, want %d", k, len(gotV), len(v))
			continue
		}
		for i := range v {
			if gotV[i] != v[i] {
				t.Errorf("Header %q value mismatch at index %d: got %q, want %q", k, i, gotV[i], v[i])
			}
		}
	}
}
