package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/oryca/oryca/gateway/breaker"
	"github.com/oryca/oryca/gateway/model"
	"github.com/oryca/oryca/gateway/service"
)

// ── upstream error pass-through (RFC 9110: gateway ห้าม rewrite status ที่ origin ตอบมา) ──

// TestFetchFromUpstream5xxPassThrough. Upstream ตอบ 503 → ต้องได้ status/body/headers จริง ไม่ใช่ err
func TestFetchFromUpstream5xxPassThrough(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"maintenance"}`))
	}))
	defer up.Close()

	h := newTestProxyHandler()
	req := httptest.NewRequest("GET", "/", nil)
	ur := h.fetchFromUpstream(req, up.URL, "127.0.0.1", "rid", &model.Source{}, 10_000)

	if ur.err != nil {
		t.Fatalf("5xx ต้อง pass-through ไม่ใช่ err: %v", ur.err)
	}
	if ur.statusCode != http.StatusServiceUnavailable {
		t.Errorf("status: want 503 got %d", ur.statusCode)
	}
	if string(ur.body) != `{"error":"maintenance"}` {
		t.Errorf("body ต้องถึงครบ: got %q", ur.body)
	}
	if ur.headers.Get("Retry-After") != "30" {
		t.Errorf("Retry-After ต้อง forward: %v", ur.headers)
	}
}

// TestFetchFromUpstream429PassThrough. 429 ต้อง pass-through เช่นกัน (ไม่ใช่ 502)
func TestFetchFromUpstream429PassThrough(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("slow down"))
	}))
	defer up.Close()

	h := newTestProxyHandler()
	req := httptest.NewRequest("GET", "/", nil)
	ur := h.fetchFromUpstream(req, up.URL, "127.0.0.1", "rid", &model.Source{}, 10_000)

	if ur.err != nil || ur.statusCode != http.StatusTooManyRequests || string(ur.body) != "slow down" {
		t.Fatalf("429 pass-through: err=%v status=%d body=%q", ur.err, ur.statusCode, ur.body)
	}
	if ur.headers.Get("Retry-After") != "5" {
		t.Errorf("Retry-After ต้อง forward: %v", ur.headers)
	}
}

// TestFetchFromUpstreamNetworkError. คุยกับ upstream ไม่ได้จริง ต้องยังเป็น err (handler ตอบ 502 ตามเดิม)
func TestFetchFromUpstreamNetworkError(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	up.Close() // ปิดทันที — connection refused

	h := newTestProxyHandler()
	req := httptest.NewRequest("GET", "/", nil)
	ur := h.fetchFromUpstream(req, up.URL, "127.0.0.1", "rid", &model.Source{}, 10_000)

	if ur.err == nil || ur.statusCode != 0 {
		t.Fatalf("network error ต้องคง err: err=%v status=%d", ur.err, ur.statusCode)
	}
}

// TestUpstreamErrorBodyCapped. Error body ยักษ์ต้องถูกตัดที่เพดาน ไม่ buffer ทั้งก้อน
func TestUpstreamErrorBodyCapped(t *testing.T) {
	huge := strings.Repeat("e", 2<<20) // 2MB
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(huge))
	}))
	defer up.Close()

	h := newTestProxyHandler()
	req := httptest.NewRequest("GET", "/", nil)
	ur := h.fetchFromUpstream(req, up.URL, "127.0.0.1", "rid", &model.Source{}, maxBufferedUpstreamBytes)

	if ur.err != nil || ur.statusCode != http.StatusInternalServerError {
		t.Fatalf("5xx pass-through: err=%v status=%d", ur.err, ur.statusCode)
	}
	got := int64(len(ur.body))
	if ur.stream != nil {
		ur.claimStream()
		rest, _ := io.ReadAll(ur.stream)
		ur.stream.Close()
		got += int64(len(rest))
	}
	if got > service.MaxErrorBodyBytes {
		t.Errorf("error body ต้องถูก cap: got %d bytes", got)
	}
	// Content-Length header ต้องตรงกับ body จริงหลังตัด (กัน client ค้างรอ bytes ที่ไม่มา)
	if cl := ur.headers.Get("Content-Length"); cl != "" && cl != strconv.Itoa(service.MaxErrorBodyBytes) {
		t.Errorf("Content-Length ต้องตรง body หลังตัด: got %s", cl)
	}
}

// TestIsUpstreamFailure. เส้นแบ่ง refund/CB ต้องมาจาก classification เดียวกัน
func TestIsUpstreamFailure(t *testing.T) {
	for _, s := range []int{500, 502, 503, 504, 408, 429} {
		if !breaker.IsUpstreamFailure(s) {
			t.Errorf("%d ต้องนับเป็น upstream failure", s)
		}
	}
	for _, s := range []int{200, 204, 301, 400, 401, 403, 404, 422} {
		if breaker.IsUpstreamFailure(s) {
			t.Errorf("%d ต้องไม่นับเป็น upstream failure", s)
		}
	}
}

// TestForwardingHeaders. X-Forwarded-Host ต้องเป็น host จริงของ client request (Go เก็บใน req.Host ไม่ใช่ header map)
func TestForwardingHeaders(t *testing.T) {
	var gotXFH, gotXFF string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXFH = r.Header.Get("X-Forwarded-Host")
		gotXFF = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	h := newTestProxyHandler()
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "dev-oryca.i-bitz.world"
	ur := h.fetchFromUpstream(req, up.URL, "10.1.2.3", "rid", &model.Source{}, 10_000)

	if ur.err != nil {
		t.Fatalf("fetch: %v", ur.err)
	}
	if gotXFH != "dev-oryca.i-bitz.world" {
		t.Errorf("X-Forwarded-Host: want dev-oryca.i-bitz.world got %q", gotXFH)
	}
	if gotXFF != "10.1.2.3" {
		t.Errorf("X-Forwarded-For: want 10.1.2.3 got %q", gotXFF)
	}
}
