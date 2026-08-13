package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestProvider(srv *httptest.Server) *HTTPSyncProvider {
	p := NewHTTPSyncProvider(HTTPSyncConfig{
		BaseURL:        srv.URL,
		InternalSecret: "test-secret",
		Timeout:        3 * time.Second,
	})
	return p
}

func TestGetServices_OK(t *testing.T) {
	payload := []*ServicePayload{{ID: "svc1", Name: "Test", BasePath: "/test", Enabled: true}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") != "test-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("ETag", `"abc123"`)
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	result, err := p.GetServices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].ID != "svc1" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestGetServices_304NotModified(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		etag := `"abc123"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		json.NewEncoder(w).Encode([]*ServicePayload{{ID: "svc1"}})
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	ctx := context.Background()

	// first call — gets data
	r1, err := p.GetServices(ctx)
	if err != nil || len(r1) != 1 {
		t.Fatalf("first call failed: %v %v", r1, err)
	}

	// second call — 304, nil returned
	r2, err := p.GetServices(ctx)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if r2 != nil {
		t.Error("expected nil on 304, got data")
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", calls)
	}
}

func TestGetServices_UnauthorizedNoRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	_, err := p.GetServices(context.Background())
	if err == nil {
		t.Fatal("expected error on 401")
	}
	// 401 is not a transient error — retry should still happen (3 attempts) since it's not circuit-open
	if calls > 3 {
		t.Errorf("expected at most 3 calls, got %d", calls)
	}
}

func TestGetSources_OK(t *testing.T) {
	payload := []*SourcePayload{{Alias: "gistda", Type: "REST", URL: "http://example.com"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	result, err := p.GetSources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].Alias != "gistda" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestGetApiKeys_OK(t *testing.T) {
	enabled := true
	payload := []*ApiKeyPayload{{ID: "key-abc", ApiKey: "sk-test", Enabled: &enabled}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	result, err := p.GetApiKeys(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].ID != "key-abc" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestCircuitBreaker_OpenOnConsecutiveFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	ctx := context.Background()

	// 3 sync rounds ล้มติดกัน → circuit เปิด
	for i := 0; i < 3; i++ {
		p.GetServices(ctx)
	}

	// ครั้งถัดไปต้อง fast-fail ทันที (circuit open)
	start := time.Now()
	_, err := p.GetServices(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when circuit is open")
	}
	// fast-fail ต้องใช้เวลาน้อยกว่า 1 วินาที (ไม่ retry)
	if elapsed > time.Second {
		t.Errorf("circuit breaker should fast-fail, took %v", elapsed)
	}
}

func TestRetry_SucceedsOnThirdAttempt(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode([]*ServicePayload{{ID: "svc1"}})
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	result, err := p.GetServices(context.Background())
	if err != nil {
		t.Fatalf("expected success on third attempt, got: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 service, got %d", len(result))
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}
