package sync

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/oryca/oryca/gateway/logger"
	"net/http"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

const headerInternalKey = "X-Internal-Key"

// ErrUserNotFound is returned by GetUser when control-plane has no such user. Distinct
// from a network/transport error so callers can (if they ever want to) treat the two
// differently, though the current cache policy fails open on both alike.
var ErrUserNotFound = errors.New("sync: user not found")

type HTTPSyncProvider struct {
	baseURL        string
	internalSecret string
	httpClient     *http.Client
	breaker        *gobreaker.CircuitBreaker
	// userBreaker is isolated from breaker on purpose: GetUser runs synchronously in the
	// request path (unlike the background-polling methods below), so a spike of
	// per-request user lookups must never trip the breaker guarding services/sources/
	// api-key polling, and vice versa.
	userBreaker *gobreaker.CircuitBreaker
	lastETag    sync.Map // path → etag string
}

type HTTPSyncConfig struct {
	BaseURL        string
	InternalSecret string
	Timeout        time.Duration
}

func NewHTTPSyncProvider(cfg HTTPSyncConfig) *HTTPSyncProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "sync-provider",
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Error(fmt.Sprintf("sync: circuit breaker %s → %s", from.String(), to.String()))
		},
	})

	userCb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "sync-provider-user-lookup",
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Error(fmt.Sprintf("sync: user-lookup circuit breaker %s → %s", from.String(), to.String()))
		},
	})

	return &HTTPSyncProvider{
		baseURL:        cfg.BaseURL,
		internalSecret: cfg.InternalSecret,
		httpClient:     &http.Client{Timeout: timeout},
		breaker:        cb,
		userBreaker:    userCb,
	}
}

// doRequest executes a single HTTP GET. Timeout จัดการโดย httpClient
func (p *HTTPSyncProvider) doRequest(ctx context.Context, path string, dest interface{}) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set(headerInternalKey, p.internalSecret)

	if etag, ok := p.lastETag.Load(path); ok {
		req.Header.Set("If-None-Match", etag.(string))
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("sync: unexpected status %d for %s", resp.StatusCode, path)
	}

	if etag := resp.Header.Get("ETag"); etag != "" {
		p.lastETag.Store(path, etag)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return false, fmt.Errorf("sync: decode %s: %w", path, err)
	}
	return true, nil
}

// getWithRetry. ลำดับถูกต้องตาม Resilience Pipeline:
// Circuit Breaker (outer) → Retry (inner) → Timeout (httpClient)
func (p *HTTPSyncProvider) getWithRetry(ctx context.Context, path string, dest interface{}) (bool, error) {
	type result struct{ changed bool }

	val, err := p.breaker.Execute(func() (interface{}, error) {
		delays := []time.Duration{0, time.Second, 2 * time.Second}
		var lastErr error
		for i, d := range delays {
			if d > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(d):
				}
			}
			changed, err := p.doRequest(ctx, path, dest)
			if err == nil {
				return &result{changed: changed}, nil
			}
			lastErr = err
			logger.Error(fmt.Sprintf("sync: retry %d path=%s err=%s", i+1, path, err))
		}
		return nil, lastErr // ล้มทุก attempt → นับเป็น 1 failure ต่อ circuit breaker
	})
	if err != nil {
		return false, err
	}
	return val.(*result).changed, nil
}

func (p *HTTPSyncProvider) GetServices(ctx context.Context) ([]*ServicePayload, error) {
	var result []*ServicePayload
	changed, err := p.getWithRetry(ctx, "/internal/services", &result)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil // nil = not modified, caller should preserve existing
	}
	return result, nil
}

func (p *HTTPSyncProvider) GetSources(ctx context.Context) ([]*SourcePayload, error) {
	var result []*SourcePayload
	changed, err := p.getWithRetry(ctx, "/internal/sources", &result)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil
	}
	return result, nil
}

func (p *HTTPSyncProvider) GetApiKeys(ctx context.Context) ([]*ApiKeyPayload, error) {
	var result []*ApiKeyPayload
	changed, err := p.getWithRetry(ctx, "/internal/api-keys", &result)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil
	}
	return result, nil
}

func (p *HTTPSyncProvider) GetTransformConfigs(ctx context.Context) ([]*TransformConfigPayload, error) {
	var result []*TransformConfigPayload
	changed, err := p.getWithRetry(ctx, "/internal/response-transforms", &result)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil
	}
	return result, nil
}

// GetUser fetches one user's freshness snapshot for a cache miss. Unlike the polling
// methods above, this runs synchronously in the request path. A single fast attempt
// through its own circuit breaker, no retry-with-backoff delay sequence, and no ETag
// caching (a one-shot per-userID lookup gains nothing from it and would otherwise grow
// lastETag unboundedly across every user ever looked up).
func (p *HTTPSyncProvider) GetUser(ctx context.Context, userID string) (*ApiKeyOwnerPayload, error) {
	val, err := p.userBreaker.Execute(func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/internal/users/"+userID, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set(headerInternalKey, p.internalSecret)

		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return nil, ErrUserNotFound
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("sync: get user status %d", resp.StatusCode)
		}

		var result ApiKeyOwnerPayload
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("sync: decode user: %w", err)
		}
		return &result, nil
	})
	if err != nil {
		return nil, err
	}
	return val.(*ApiKeyOwnerPayload), nil
}

func (p *HTTPSyncProvider) GetJWKS(ctx context.Context) (*rsa.PublicKey, error) {
	var jwks struct {
		Keys []struct {
			X5C []string `json:"x5c"`
		} `json:"keys"`
	}
	// JWKS ไม่ใช้ ETag cache เพราะ response เล็กและ rotate ได้ตลอดเวลา
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/.well-known/jwks.json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(headerInternalKey, p.internalSecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jwks fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks: status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("jwks parse: %w", err)
	}
	if len(jwks.Keys) == 0 || len(jwks.Keys[0].X5C) == 0 {
		return nil, fmt.Errorf("jwks: no keys found")
	}

	der, err := base64.StdEncoding.DecodeString(jwks.Keys[0].X5C[0])
	if err != nil {
		return nil, fmt.Errorf("jwks decode x5c: %w", err)
	}

	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("jwks parse key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("jwks: not RSA key")
	}
	return rsaPub, nil
}
