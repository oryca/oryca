package handler

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func gzipCtx(t *testing.T, acceptGzip bool) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	if acceptGzip {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// TestShouldGzipResponse — เงื่อนไขครบทุกแขนง
func TestShouldGzipResponse(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	jsonHdr := http.Header{"Content-Type": []string{"application/json"}}

	cases := []struct {
		name string
		r    *http.Request
		h    http.Header
		st   int
		ln   int64
		want bool
	}{
		{"json ok", req, jsonHdr, 200, 5000, true},
		{"stream unknown len", req, jsonHdr, 200, -1, true},
		{"too small", req, jsonHdr, 200, 100, false},
		{"non-2xx", req, jsonHdr, 404, 5000, false},
		{"image skipped", req, http.Header{"Content-Type": []string{"image/png"}}, 200, 5000, false},
		{"protobuf tile skipped", req, http.Header{"Content-Type": []string{"application/x-protobuf"}}, 200, 5000, false},
		{"geo+json ok", req, http.Header{"Content-Type": []string{"application/geo+json"}}, 200, 5000, true},
		{"already encoded", req, http.Header{"Content-Type": []string{"application/json"}, "Content-Encoding": []string{"gzip"}}, 200, 5000, false},
		{"no accept-encoding", httptest.NewRequest("GET", "/", nil), jsonHdr, 200, 5000, false},
	}
	for _, c := range cases {
		if got := shouldGzipResponse(c.r, c.h, c.st, c.ln); got != c.want {
			t.Errorf("%s: want %v got %v", c.name, c.want, got)
		}
	}
}

// TestWriteGzipRoundtrip — บีบแล้วแตกกลับต้องได้ข้อมูลเดิม และคืนขนาดก่อนบีบ
func TestWriteGzipRoundtrip(t *testing.T) {
	c, rec := gzipCtx(t, true)
	body := []byte(strings.Repeat(`{"k":"v"},`, 500))

	n := writeGzip(c, body)
	if n != int64(len(body)) {
		t.Errorf("n must be pre-compress size: want %d got %d", len(body), n)
	}
	if rec.Body.Len() >= len(body) {
		t.Errorf("compressed should be smaller: %d >= %d", rec.Body.Len(), len(body))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	out, _ := io.ReadAll(zr)
	if string(out) != string(body) {
		t.Error("roundtrip mismatch")
	}
}

// TestWriteStreamedBodyGzip — stream mode บีบ: นับ bytes ก่อนบีบ + แตกกลับได้ครบ
func TestWriteStreamedBodyGzip(t *testing.T) {
	c, rec := gzipCtx(t, true)
	prefix := []byte(strings.Repeat("A", 2000))
	tail := strings.Repeat("B", 3000)

	n := writeStreamedBody(c, prefix, io.NopCloser(strings.NewReader(tail)), true)
	if n != 5000 {
		t.Errorf("uncompressed count: want 5000 got %d", n)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	out, _ := io.ReadAll(zr)
	if len(out) != 5000 || string(out[:5]) != "AAAAA" || string(out[4999:]) != "B" {
		t.Errorf("roundtrip: len=%d", len(out))
	}
}

// TestWriteJSONMaybeGzip — เก็บขนาดก่อนบีบใน context ให้ log
func TestWriteJSONMaybeGzip(t *testing.T) {
	c, rec := gzipCtx(t, true)
	body := []byte(strings.Repeat(`{"x":1},`, 400))

	if err := writeJSONMaybeGzip(c, body); err != nil {
		t.Fatalf("write: %v", err)
	}
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected gzip encoding")
	}
	if v, ok := c.Get(ctxKeyUncompressedSize).(int64); !ok || v != int64(len(body)) {
		t.Errorf("uncompressed size in ctx: %v", c.Get(ctxKeyUncompressedSize))
	}

	// client ไม่รับ gzip → ส่งดิบ ไม่มี ctx key
	c2, rec2 := gzipCtx(t, false)
	if err := writeJSONMaybeGzip(c2, body); err != nil {
		t.Fatalf("write plain: %v", err)
	}
	if rec2.Header().Get("Content-Encoding") != "" || rec2.Body.Len() != len(body) {
		t.Error("plain path must send raw body")
	}
}
