package breaker

import (
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxRequests:         2,
		Interval:            time.Minute,
		Timeout:             20 * time.Millisecond, // short so tests don't sleep long
		ConsecutiveFailures: 3,
	}
}

func TestPerDomainBreaker_TripsAfterConsecutiveFailures(t *testing.T) {
	p := NewPerDomainBreaker(testConfig())
	url := "http://upstream-a.example.com/foo"
	networkErr := errors.New("dial tcp: connection refused")

	for i := 0; i < 3; i++ {
		err := p.Execute(url, func() error { return networkErr })
		require.Equal(t, networkErr, err)
	}

	assert.Equal(t, gobreaker.StateOpen, p.State(url))

	// further calls fail fast without invoking fn
	called := false
	err := p.Execute(url, func() error { called = true; return nil })
	assert.ErrorIs(t, err, ErrOpenState)
	assert.False(t, called, "Execute must not invoke fn while circuit is open")
}

func TestPerDomainBreaker_4xxDoesNotCountAsFailure(t *testing.T) {
	p := NewPerDomainBreaker(testConfig())
	url := "http://upstream-b.example.com/foo"

	for i := 0; i < 10; i++ {
		err := p.Execute(url, func() error { return &UpstreamError{StatusCode: 404} })
		var ue *UpstreamError
		require.ErrorAs(t, err, &ue)
	}

	// 404 is a definitive client-error response, not an upstream failure. Breaker stays closed
	assert.Equal(t, gobreaker.StateClosed, p.State(url))
}

func TestPerDomainBreaker_5xxCountsAsFailure(t *testing.T) {
	p := NewPerDomainBreaker(testConfig())
	url := "http://upstream-c.example.com/foo"

	for i := 0; i < 3; i++ {
		_ = p.Execute(url, func() error { return &UpstreamError{StatusCode: 503} })
	}

	assert.Equal(t, gobreaker.StateOpen, p.State(url))
}

func TestPerDomainBreaker_PerKeyIsolation(t *testing.T) {
	p := NewPerDomainBreaker(testConfig())
	failingURL := "http://upstream-d.example.com/foo"
	healthyURL := "http://upstream-e.example.com/foo"
	networkErr := errors.New("boom")

	for i := 0; i < 3; i++ {
		_ = p.Execute(failingURL, func() error { return networkErr })
	}
	assert.Equal(t, gobreaker.StateOpen, p.State(failingURL))

	// a different upstream must be unaffected
	err := p.Execute(healthyURL, func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, gobreaker.StateClosed, p.State(healthyURL))
}

func TestPerDomainBreaker_RecoversAfterTimeout(t *testing.T) {
	cfg := testConfig()
	p := NewPerDomainBreaker(cfg)
	url := "http://upstream-f.example.com/foo"
	networkErr := errors.New("boom")

	for i := 0; i < 3; i++ {
		_ = p.Execute(url, func() error { return networkErr })
	}
	require.Equal(t, gobreaker.StateOpen, p.State(url))

	time.Sleep(cfg.Timeout + 10*time.Millisecond)

	// half-open now. Needs MaxRequests consecutive successes to close again
	for i := 0; i < cfg.MaxRequests; i++ {
		err := p.Execute(url, func() error { return nil })
		require.NoError(t, err)
	}
	assert.Equal(t, gobreaker.StateClosed, p.State(url))
}

func TestPerDomainBreaker_IdleEviction(t *testing.T) {
	cfg := testConfig()
	cfg.IdleEvictAfter = 20 * time.Millisecond
	p := NewPerDomainBreaker(cfg)
	url := "http://upstream-g.example.com/foo"

	for i := 0; i < 3; i++ {
		_ = p.Execute(url, func() error { return errors.New("boom") })
	}
	require.Equal(t, gobreaker.StateOpen, p.State(url))

	_, loadedBefore := p.breakers.Load(cbKey(url))
	require.True(t, loadedBefore, "breaker entry must exist before eviction")

	time.Sleep(cfg.IdleEvictAfter * 2)
	p.evictIdle()

	_, loadedAfter := p.breakers.Load(cbKey(url))
	assert.False(t, loadedAfter, "idle breaker must be evicted")

	// after eviction, the key gets a fresh (closed) breaker on next use
	assert.Equal(t, gobreaker.StateClosed, p.State(url))
}

func TestPerDomainBreaker_IdleEvictionDisabledByDefault(t *testing.T) {
	p := NewPerDomainBreaker(testConfig()) // IdleEvictAfter left at zero value
	url := "http://upstream-h.example.com/foo"
	_ = p.Execute(url, func() error { return nil })

	p.evictIdle() // must be a no-op when disabled
	_, loaded := p.breakers.Load(cbKey(url))
	assert.True(t, loaded, "breaker must survive evictIdle when IdleEvictAfter <= 0")
}

func TestCbKey(t *testing.T) {
	cases := map[string]string{
		"https://map.example.com/osm/tiles/1/2/3.png": "map.example.com/osm",
		"https://api.example.com/":                    "api.example.com",
		"https://api.example.com":                     "api.example.com",
		// no scheme/host. Url.Parse treats the whole string as a relative path
		"not-a-valid-url-but-still-a-string": "/not-a-valid-url-but-still-a-string",
	}
	for input, want := range cases {
		assert.Equal(t, want, cbKey(input), "input=%s", input)
	}
}
