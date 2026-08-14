package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/oryca/oryca/gateway/breaker"
	"github.com/oryca/oryca/gateway/cache"
	"github.com/oryca/oryca/gateway/model"
	"github.com/oryca/oryca/gateway/service"
)

// TestWriteStreamedBody. Prefix + ส่วนที่ stream ต้องถึง client ครบและนับ bytes ถูก
func TestWriteStreamedBody(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), rec)

	prefix := []byte("HEAD-")
	rest := io.NopCloser(strings.NewReader("STREAMED-TAIL"))

	n := writeStreamedBody(c, prefix, rest, false)

	want := "HEAD-STREAMED-TAIL"
	if rec.Body.String() != want {
		t.Errorf("body: want %q, got %q", want, rec.Body.String())
	}
	if n != int64(len(want)) {
		t.Errorf("bytes: want %d, got %d", len(want), n)
	}
}

// TestWriteStreamedBodyNoPrefix. Stream mode ล้วน (ไม่มี prefix)
func TestWriteStreamedBodyNoPrefix(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), rec)

	n := writeStreamedBody(c, nil, io.NopCloser(strings.NewReader("only-stream")), false)
	if rec.Body.String() != "only-stream" || n != 11 {
		t.Errorf("got body=%q n=%d", rec.Body.String(), n)
	}
}

// TestClaimStream. มีผู้จองสำเร็จได้คนเดียวแม้แย่งกันหลาย goroutine
func TestClaimStream(t *testing.T) {
	ur := upstreamResult{claimed: new(int32)}
	const workers = 16
	var wg sync.WaitGroup
	wins := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wins <- ur.claimStream()
		}()
	}
	wg.Wait()
	close(wins)
	count := 0
	for w := range wins {
		if w {
			count++
		}
	}
	if count != 1 {
		t.Errorf("want exactly 1 successful claim, got %d", count)
	}
}

// TestClaimStreamNil. Result ที่ไม่มี claimed ต้องไม่ panic และคืน false
func TestClaimStreamNil(t *testing.T) {
	ur := upstreamResult{}
	if ur.claimStream() {
		t.Error("nil claimed must return false")
	}
}

// ── fetchFromUpstream integration (httptest upstream จริง) ─────────────────────

func newTestProxyHandler() *Handler {
	svc := service.NewProxyService(service.ProxyConfig{
		MaxIdleConns: 10, MaxIdleConnsPerHost: 5,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second, DialTimeout: 5 * time.Second,
	}, breaker.NewPerDomainBreaker(breaker.CircuitBreakerConfig{
		MaxRequests: 5, Interval: 60 * time.Second, Timeout: 30 * time.Second, ConsecutiveFailures: 5,
	}))
	return &Handler{proxySvc: svc}
}

// TestFetchFromUpstreamModes. Buffered / oversize→prefix+stream / pure stream
func TestFetchFromUpstreamModes(t *testing.T) {
	payload := strings.Repeat("x", 1000)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer up.Close()

	h := newTestProxyHandler()
	req := httptest.NewRequest("GET", "/", nil)
	src := &model.Source{}

	// buffered: limit ใหญ่กว่า body. ได้ทั้งก้อน ไม่มี stream
	ur := h.fetchFromUpstream(req, up.URL, "127.0.0.1", "rid", src, 10_000)
	if ur.err != nil || ur.stream != nil || len(ur.body) != 1000 {
		t.Fatalf("buffered mode: err=%v stream=%v len=%d", ur.err, ur.stream != nil, len(ur.body))
	}

	// oversize: limit เล็กกว่า body. ได้ prefix + stream ที่เหลือ รวมแล้วครบ
	ur = h.fetchFromUpstream(req, up.URL, "127.0.0.1", "rid", src, 100)
	if ur.err != nil || ur.stream == nil {
		t.Fatalf("oversize mode: err=%v stream=%v", ur.err, ur.stream != nil)
	}
	if !ur.claimStream() {
		t.Fatal("first claim must succeed")
	}
	rest, _ := io.ReadAll(ur.stream)
	ur.stream.Close()
	if len(ur.body)+len(rest) != 1000 {
		t.Errorf("oversize total: prefix=%d rest=%d want 1000", len(ur.body), len(rest))
	}

	// pure stream: maxBuffer 0. ไม่ buffer เลย
	ur = h.fetchFromUpstream(req, up.URL, "127.0.0.1", "rid", src, 0)
	if ur.err != nil || ur.stream == nil || len(ur.body) != 0 {
		t.Fatalf("stream mode: err=%v stream=%v prefix=%d", ur.err, ur.stream != nil, len(ur.body))
	}
	ur.claimStream()
	all, _ := io.ReadAll(ur.stream)
	ur.stream.Close()
	if len(all) != 1000 {
		t.Errorf("stream total: %d want 1000", len(all))
	}
}

// TestFetchFromUpstreamRangePassthrough. Range header ต้องถึง upstream และได้ 206 + chunk ที่ถูกต้อง
// (เคส PMTiles: stream mode + Range ต้องไม่เพี้ยน)
func TestFetchFromUpstreamRangePassthrough(t *testing.T) {
	payload := "0123456789ABCDEF"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		if rng != "bytes=4-7" {
			t.Errorf("upstream expected Range bytes=4-7, got %q", rng)
		}
		w.Header().Set("Content-Range", "bytes 4-7/16")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(payload[4:8]))
	}))
	defer up.Close()

	h := newTestProxyHandler()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Range", "bytes=4-7")

	ur := h.fetchFromUpstream(req, up.URL, "127.0.0.1", "rid", &model.Source{}, 0)
	if ur.err != nil || ur.statusCode != http.StatusPartialContent || ur.stream == nil {
		t.Fatalf("range mode: err=%v status=%d stream=%v", ur.err, ur.statusCode, ur.stream != nil)
	}
	ur.claimStream()
	chunk, _ := io.ReadAll(ur.stream)
	ur.stream.Close()
	if string(chunk) != "4567" {
		t.Errorf("chunk: want %q got %q", "4567", string(chunk))
	}
	if ur.headers.Get("Content-Range") != "bytes 4-7/16" {
		t.Errorf("Content-Range missing: %v", ur.headers)
	}
}

// TestIsCacheableSize. เพดาน 5MB: เท่าพอดี cache ได้, เกิน 1 byte ไม่ได้
func TestIsCacheableSize(t *testing.T) {
	if !cache.IsCacheableSize(cache.MaxCacheableBodyBytes) {
		t.Error("exactly 5MB must be cacheable")
	}
	if cache.IsCacheableSize(cache.MaxCacheableBodyBytes + 1) {
		t.Error("5MB+1 must not be cacheable")
	}
	if !cache.IsCacheableSize(0) {
		t.Error("empty body must be cacheable")
	}
}
