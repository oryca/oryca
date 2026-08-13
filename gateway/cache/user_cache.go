package cache

import (
	"context"
	"github.com/oryca/oryca/gateway/logger"
	"sync"
	"time"

	gwsync "github.com/oryca/oryca/gateway/sync"

	"golang.org/x/sync/singleflight"
)

const userRefreshTimeout = 5 * time.Second

type userFreshnessEntry struct {
	user      *gwsync.ApiKeyOwnerPayload // nil on a genuine "no such user" (negative cache)
	err       error
	expiresAt time.Time
}

// UserFreshnessCache is a per-pod, in-memory, fetch-on-demand cache of a user's
// packageId/enabled/verified/expiredAt — deliberately not a full-preload structure like
// trie.HybridTrie: at 60k+ users, and given the JWT-only baseline previously did zero
// lookups per request, preloading everything would be a pure regression for no benefit.
//
// Get never blocks on a network call: a fresh entry is an in-memory map read, a
// stale-but-present entry is served immediately while refreshed in the background
// (RFC 5861 stale-while-revalidate, same shape as Kong Gateway's lua-resty-mlcache), and
// a true first-time miss fails open (caller leaves whatever identity it already has
// untouched) while a background fetch primes the entry for next time. Background
// refreshes are deduped per key via singleflight (cache-stampede protection, originally
// from groupcache) and rate-limited by a bounded worker pool so a burst of many distinct
// cache misses (e.g. right after a pod restart) drains through control-plane smoothly
// instead of firing all at once. Fetch failures are cached too (negative caching, same
// idea as DNS/RFC 2308) so a permanently-404 id doesn't get re-fetched every request.
type UserFreshnessCache struct {
	mu       sync.RWMutex
	entries  map[string]*userFreshnessEntry
	ttl      time.Duration
	provider gwsync.SyncProvider
	sf       singleflight.Group

	refreshSem chan struct{}
}

func NewUserFreshnessCache(provider gwsync.SyncProvider, ttl time.Duration, refreshConcurrency int) *UserFreshnessCache {
	if refreshConcurrency <= 0 {
		refreshConcurrency = 1
	}
	return &UserFreshnessCache{
		entries:    make(map[string]*userFreshnessEntry),
		ttl:        ttl,
		provider:   provider,
		refreshSem: make(chan struct{}, refreshConcurrency),
	}
}

// Get returns the cached snapshot for userID without ever making the caller wait on a
// network call. A missing or expired entry triggers a background refresh and returns
// immediately with whatever's available (the stale value, or nil on a true first miss).
func (c *UserFreshnessCache) Get(userID string) (*gwsync.ApiKeyOwnerPayload, error) {
	c.mu.RLock()
	e, ok := c.entries[userID]
	c.mu.RUnlock()

	if ok && time.Now().Before(e.expiresAt) {
		return e.user, e.err
	}

	c.scheduleRefresh(userID)

	if ok {
		return e.user, e.err // stale-while-revalidate
	}
	return nil, nil // true first-time miss — nothing to serve yet, caller fails open
}

// scheduleRefresh launches a bounded, deduped background fetch for userID. Safe to call
// far more often than the concurrency limit allows — excess callers either join an
// in-flight singleflight call for the same id, or queue on refreshSem.
func (c *UserFreshnessCache) scheduleRefresh(userID string) {
	logger.Go("user cache: refresh", func() {
		_, _, _ = c.sf.Do(userID, func() (interface{}, error) {
			c.refreshSem <- struct{}{}
			defer func() { <-c.refreshSem }()

			ctx, cancel := context.WithTimeout(context.Background(), userRefreshTimeout)
			defer cancel()
			user, err := c.provider.GetUser(ctx, userID)
			c.mu.Lock()
			c.entries[userID] = &userFreshnessEntry{user: user, err: err, expiresAt: time.Now().Add(c.ttl)}
			c.mu.Unlock()
			return nil, nil
		})
	})
}

// Set writes a fresh entry directly from a real-time push event — no network round trip.
func (c *UserFreshnessCache) Set(userID string, user *gwsync.ApiKeyOwnerPayload) {
	c.mu.Lock()
	c.entries[userID] = &userFreshnessEntry{user: user, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// Invalidate drops a cached entry so the next Get re-fetches lazily.
func (c *UserFreshnessCache) Invalidate(userID string) {
	c.mu.Lock()
	delete(c.entries, userID)
	c.mu.Unlock()
}

// InvalidateAndRefresh drops the cached entries for userIDs and immediately schedules a
// background refetch for each, rather than waiting for the next Get to trigger it. Used
// for control-plane's bulk user-package events (AssignUsers/RemoveUsers/BulkDelete): the
// affected-id count there is bounded by one admin action (tens-hundreds), not by request
// volume, so refreshing eagerly is safe and is what makes a package change take effect
// on a user's very next request without needing to wait on the lazy path above.
func (c *UserFreshnessCache) InvalidateAndRefresh(userIDs []string) {
	c.mu.Lock()
	for _, id := range userIDs {
		delete(c.entries, id)
	}
	c.mu.Unlock()
	for _, id := range userIDs {
		c.scheduleRefresh(id)
	}
}
