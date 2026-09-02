package cache_test

import (
	"testing"
	"time"

	"github.com/oryca/oryca/gateway/cache"
)

// rateLimiter interface ที่ test ใช้. เหมือนกับที่ handler จะเรียก
type rateLimiter interface {
	Allow(userID, packageID, serviceID, resourcePath, memberHint string, tiers []cache.RateLimitTier) (allowed bool, limit int, remaining int, retryAfterSec int, resetSec int)
}

// stubRateLimiter จำลอง sliding window ใน memory เพื่อ test logic ที่ handler ใช้
type stubRateLimiter struct {
	counters map[string][]time.Time
}

func newStubRateLimiter() *stubRateLimiter {
	return &stubRateLimiter{counters: make(map[string][]time.Time)}
}

// evict ลบ entries เก่าออกจาก window และคืน entries ที่ยังอยู่
func (s *stubRateLimiter) evict(key string, window time.Duration) []time.Time {
	cutoff := time.Now().Add(-window)
	fresh := s.counters[key][:0]
	for _, ts := range s.counters[key] {
		if ts.After(cutoff) {
			fresh = append(fresh, ts)
		}
	}
	s.counters[key] = fresh
	return fresh
}

// resetSecs คำนวณวินาทีจนกว่า oldest entry หมดอายุ
func resetSecs(entries []time.Time, window time.Duration) int {
	if len(entries) == 0 {
		return int(window.Seconds())
	}
	rs := int(time.Until(entries[0].Add(window)).Seconds()) + 1
	if rs < 1 {
		return 1
	}
	return rs
}

func (s *stubRateLimiter) Allow(userID, packageID, serviceID, resourcePath, memberHint string, tiers []cache.RateLimitTier) (bool, int, int, int, int) {
	now := time.Now()
	for _, tier := range tiers {
		key := cache.RateLimitKey(userID, packageID, serviceID, resourcePath, tier.WindowSec)
		window := time.Duration(tier.WindowSec) * time.Second
		fresh := s.evict(key, window)
		if len(fresh) >= tier.Limit {
			rs := resetSecs(fresh, window)
			return false, tier.Limit, 0, rs, rs
		}
	}
	for _, tier := range tiers {
		key := cache.RateLimitKey(userID, packageID, serviceID, resourcePath, tier.WindowSec)
		s.counters[key] = append(s.counters[key], now)
	}
	minRem, minLimit, minReset := int(^uint(0)>>1), 0, 0
	for _, tier := range tiers {
		key := cache.RateLimitKey(userID, packageID, serviceID, resourcePath, tier.WindowSec)
		window := time.Duration(tier.WindowSec) * time.Second
		rem := tier.Limit - len(s.counters[key])
		if rem < minRem {
			minRem = rem
			minLimit = tier.Limit
			minReset = resetSecs(s.counters[key], window)
		}
	}
	return true, minLimit, minRem, 0, minReset
}

// ── Tests ──

func TestRateLimitKey_Format(t *testing.T) {
	key := cache.RateLimitKey("u1", "pkg1", "svc1", "/data", 60)
	expected := "ratelimit:u1:pkg1:svc1:/data:60"
	if key != expected {
		t.Errorf("expected %q got %q", expected, key)
	}
}

func TestAllow_WithinLimit(t *testing.T) {
	rl := newStubRateLimiter()
	tiers := []cache.RateLimitTier{{Limit: 5, WindowSec: 60}}

	for i := 0; i < 5; i++ {
		allowed, _, _, _, _ := rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestAllow_ExceedsLimit_Returns429Info(t *testing.T) {
	rl := newStubRateLimiter()
	tiers := []cache.RateLimitTier{{Limit: 3, WindowSec: 60}}

	for i := 0; i < 3; i++ {
		rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
	}

	allowed, limit, remaining, retryAfterSec, _ := rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
	if allowed {
		t.Error("4th request should be denied")
	}
	if limit != 3 {
		t.Errorf("expected limit=3 got %d", limit)
	}
	if remaining != 0 {
		t.Errorf("expected remaining=0 got %d", remaining)
	}
	if retryAfterSec <= 0 {
		t.Errorf("expected retryAfterSec > 0 got %d", retryAfterSec)
	}
}

func TestAllow_IsolatedPerUser(t *testing.T) {
	rl := newStubRateLimiter()
	tiers := []cache.RateLimitTier{{Limit: 2, WindowSec: 60}}

	rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
	rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
	denied, _, _, _, _ := rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
	if denied {
		t.Error("u1 should be denied")
	}

	allowed, _, _, _, _ := rl.Allow("u2", "pkg1", "svc1", "/data", "", tiers)
	if !allowed {
		t.Error("u2 should be allowed independently")
	}
}

func TestAllow_IsolatedPerPackage(t *testing.T) {
	rl := newStubRateLimiter()
	tiersA := []cache.RateLimitTier{{Limit: 1, WindowSec: 60}}
	tiersB := []cache.RateLimitTier{{Limit: 10, WindowSec: 60}}

	rl.Allow("u1", "pkgA", "svc1", "/data", "", tiersA)
	denied, _, _, _, _ := rl.Allow("u1", "pkgA", "svc1", "/data", "", tiersA)
	if denied {
		t.Error("pkgA should be denied after 1 req")
	}

	allowed, _, _, _, _ := rl.Allow("u1", "pkgB", "svc1", "/data", "", tiersB)
	if !allowed {
		t.Error("pkgB counter is independent")
	}
}

func TestAllow_MultipleTiers_StrictestWins(t *testing.T) {
	rl := newStubRateLimiter()
	tiers := []cache.RateLimitTier{
		{Limit: 10, WindowSec: 60},
		{Limit: 2, WindowSec: 1},
	}

	rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
	rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)

	allowed, _, _, _, _ := rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
	if allowed {
		t.Error("should be denied by strict tier (2/sec)")
	}
}

func TestAllow_Remaining_DecreasesCorrectly(t *testing.T) {
	rl := newStubRateLimiter()
	tiers := []cache.RateLimitTier{{Limit: 5, WindowSec: 60}}

	_, limit, remaining, _, _ := rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
	if limit != 5 {
		t.Errorf("expected limit=5 got %d", limit)
	}
	if remaining != 4 {
		t.Errorf("after 1st req, remaining should be 4 got %d", remaining)
	}
}

func TestAllow_EmptyTiers_AlwaysAllowed(t *testing.T) {
	rl := newStubRateLimiter()
	allowed, _, _, _, _ := rl.Allow("u1", "pkg1", "svc1", "/data", "", nil)
	if !allowed {
		t.Error("empty tiers should always allow")
	}
}

func TestAllow_ResetSec_AccurateOnDeny(t *testing.T) {
	rl := newStubRateLimiter()
	tiers := []cache.RateLimitTier{{Limit: 2, WindowSec: 10}}

	rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
	rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)

	_, _, _, retryAfter, resetSec := rl.Allow("u1", "pkg1", "svc1", "/data", "", tiers)
	// resetSec ต้องอยู่ใน [1, 10]. ไม่ใช่ worst-case เต็ม window
	if resetSec < 1 || resetSec > 10 {
		t.Errorf("resetSec should be in [1,10] got %d", resetSec)
	}
	if retryAfter != resetSec {
		t.Errorf("retryAfter should equal resetSec: retryAfter=%d resetSec=%d", retryAfter, resetSec)
	}
}
