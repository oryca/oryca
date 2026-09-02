package breaker

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sony/gobreaker"

	"github.com/oryca/oryca/gateway/logger"
)

var ErrOpenState = gobreaker.ErrOpenState

// CircuitBreakerConfig tunes PerDomainBreaker. Zero-value Config (from config.Config
// defaults) reproduces the original hardcoded behavior exactly.
type CircuitBreakerConfig struct {
	MaxRequests         int
	Interval            time.Duration
	Timeout             time.Duration
	ConsecutiveFailures int
	// IdleEvictAfter. Drop a breaker that hasn't been touched in this long, so per-path
	// cardinality can't grow without bound. <= 0 disables eviction.
	IdleEvictAfter time.Duration
}

type breakerEntry struct {
	cb         *gobreaker.CircuitBreaker
	lastAccess atomic.Int64 // unix nano, for idle eviction
}

type PerDomainBreaker struct {
	breakers       sync.Map
	settings       gobreaker.Settings
	idleEvictAfter time.Duration
}

func NewPerDomainBreaker(cfg CircuitBreakerConfig) *PerDomainBreaker {
	return &PerDomainBreaker{
		idleEvictAfter: cfg.IdleEvictAfter,
		settings: gobreaker.Settings{
			MaxRequests: uint32(cfg.MaxRequests),
			Interval:    cfg.Interval,
			Timeout:     cfg.Timeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= uint32(cfg.ConsecutiveFailures)
			},
			IsSuccessful: func(err error) bool {
				if err == nil {
					return true
				}
				// upstream 4xx ไม่ถือว่า upstream failure
				if ue, ok := err.(*UpstreamError); ok {
					return !IsUpstreamFailure(ue.StatusCode)
				}
				return false
			},
			OnStateChange: func(name string, from, to gobreaker.State) {
				logger.Info("circuit breaker: " + name + " " + from.String() + " -> " + to.String())
			},
		},
	}
}

func (p *PerDomainBreaker) Execute(upstreamURL string, fn func() error) error {
	key := cbKey(upstreamURL)
	cb := p.getOrCreate(key)
	_, err := cb.Execute(func() (interface{}, error) {
		return nil, fn()
	})
	return err
}

func (p *PerDomainBreaker) State(upstreamURL string) gobreaker.State {
	key := cbKey(upstreamURL)
	if v, ok := p.breakers.Load(key); ok {
		return v.(*breakerEntry).cb.State()
	}
	return gobreaker.StateClosed
}

func (p *PerDomainBreaker) getOrCreate(key string) *gobreaker.CircuitBreaker {
	now := time.Now().UnixNano()
	if v, ok := p.breakers.Load(key); ok {
		entry := v.(*breakerEntry)
		entry.lastAccess.Store(now)
		return entry.cb
	}
	settings := p.settings
	settings.Name = key
	entry := &breakerEntry{cb: gobreaker.NewCircuitBreaker(settings)}
	entry.lastAccess.Store(now)
	actual, _ := p.breakers.LoadOrStore(key, entry)
	return actual.(*breakerEntry).cb
}

// StartIdleEviction runs a background sweep that drops breakers untouched for
// IdleEvictAfter, until ctx is cancelled. No-op if IdleEvictAfter <= 0.
func (p *PerDomainBreaker) StartIdleEviction(ctx context.Context) {
	if p.idleEvictAfter <= 0 {
		return
	}
	interval := p.idleEvictAfter / 2
	logger.GoLoop("circuit breaker: idle eviction loop", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.evictIdle()
			}
		}
	})
}

func (p *PerDomainBreaker) evictIdle() {
	if p.idleEvictAfter <= 0 {
		return
	}
	cutoff := time.Now().Add(-p.idleEvictAfter).UnixNano()
	evicted := 0
	p.breakers.Range(func(key, value interface{}) bool {
		if value.(*breakerEntry).lastAccess.Load() < cutoff {
			p.breakers.Delete(key)
			evicted++
		}
		return true
	})
	if evicted > 0 {
		logger.Info("circuit breaker: evicted " + strconv.Itoa(evicted) + " idle breaker(s)")
	}
}

// cbKey สร้าง key จาก domain + path segment แรก
// เช่น https://map.example.com/osm/tiles → "map.example.com/osm"
func cbKey(upstreamURL string) string {
	u, err := url.Parse(upstreamURL)
	if err != nil {
		return upstreamURL
	}
	seg := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 2)[0]
	if seg == "" {
		return u.Host
	}
	return u.Host + "/" + seg
}

// IsUpstreamFailure. Status ที่นับเป็นความล้มเหลวฝั่ง provider (ไม่ใช่ความผิด client)
// เป็น single source of truth ของทั้ง circuit breaker และ refund policy. ห้ามแยกเกณฑ์
func IsUpstreamFailure(status int) bool {
	return status >= 500 || status == 408 || status == 429
}

// UpstreamError ใช้แยก HTTP error ของ upstream ออกจาก network error
type UpstreamError struct {
	StatusCode int
}

func (e *UpstreamError) Error() string {
	return "upstream error: " + strings.Join([]string{itoa(e.StatusCode)}, "")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [10]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
