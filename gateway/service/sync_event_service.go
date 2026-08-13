package service

import (
	"context"
	"encoding/json"
	"github.com/oryca/oryca/gateway/cache"
	"github.com/oryca/oryca/gateway/logger"
	gwsync "github.com/oryca/oryca/gateway/sync"
	"github.com/oryca/oryca/gateway/trie"
	"strconv"
	"sync"
)

// SyncEventService applies real-time sync events pushed via an EventListener onto the
// gateway's cache/trie. HTTP polling elsewhere is untouched — this only shortens the
// gap between a change on control-plane and the gateway picking it up; if the listener
// never delivers anything (Redis unreachable), polling alone still catches everything
// within its own interval.
type SyncEventService struct {
	trie      *trie.HybridTrie
	cache     *cache.Cache
	userCache *cache.UserFreshnessCache
	guard     *eventVersionGuard
}

func NewSyncEventService(ht *trie.HybridTrie, c *cache.Cache, userCache *cache.UserFreshnessCache) *SyncEventService {
	return &SyncEventService{trie: ht, cache: c, userCache: userCache, guard: newEventVersionGuard()}
}

// Start consumes from listener in the background until ctx is cancelled.
func (s *SyncEventService) Start(ctx context.Context, listener gwsync.EventListener) {
	events := listener.Listen(ctx)
	logger.GoLoop("sync: event consume loop", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-events:
				s.Apply(ctx, ev)
			}
		}
	})
}

// Apply routes a single event to the matching cache/trie update.
func (s *SyncEventService) Apply(ctx context.Context, ev gwsync.Event) {
	switch ev.Type {
	case gwsync.EventTypeReloadServices:
		// thin signal — ดึงจริงผ่าน HTTP GET+ETag เดิม, dedup ผ่าน singleflight อยู่แล้วใน DoSync
		s.trie.DoSync(ctx)
	case gwsync.EventTypeApiKey:
		s.applyApiKey(ctx, ev)
	case gwsync.EventTypeUser:
		s.applyUser(ev)
	case gwsync.EventTypeUserInvalidate:
		s.applyUserInvalidate(ev)
	default:
		logger.Error("sync: event: unknown type " + string(ev.Type))
	}
}

func (s *SyncEventService) applyApiKey(ctx context.Context, ev gwsync.Event) {
	var key gwsync.ApiKeyPayload
	if err := json.Unmarshal(ev.Payload, &key); err != nil {
		logger.Error("sync: event apikey: decode failed: " + err.Error())
		return
	}
	if !s.guard.allow("apikey:"+key.ID, ev.Version) {
		return
	}
	if _, err := s.cache.SetApiKeysBatch(ctx, []*gwsync.ApiKeyPayload{&key}); err != nil {
		logger.Error("sync: event apikey: apply failed: " + err.Error())
	}
}

// applyUser writes a full single-user freshness snapshot straight into the cache — no
// network round trip. Version-guarded like applyApiKey/applyBalance since this carries
// real data that could otherwise overwrite a newer value with an older, out-of-order one.
func (s *SyncEventService) applyUser(ev gwsync.Event) {
	if s.userCache == nil {
		return
	}
	var u gwsync.ApiKeyOwnerPayload
	if err := json.Unmarshal(ev.Payload, &u); err != nil {
		logger.Error("sync: event user: decode failed: " + err.Error())
		return
	}
	if !s.guard.allow("user:"+u.ID, ev.Version) {
		return
	}
	s.userCache.Set(u.ID, &u)
	logger.Info("sync: event user: applied id=" + u.ID + " packageId=" + u.PackageID)
}

// applyUserInvalidate evicts and eagerly re-fetches exactly the affected ids — deliberately
// not lazy: this event comes from bulk operations (AssignUsers/RemoveUsers/BulkDelete)
// whose affected-user count is bounded by one admin action, not by request volume, so
// eager refresh here is safe and is what makes a package change take effect on a user's
// very next request without needing to wait for a cache miss to trigger it. No version
// guard — eviction+refetch is idempotent, a duplicate delivery just costs one extra fetch.
func (s *SyncEventService) applyUserInvalidate(ev gwsync.Event) {
	if s.userCache == nil {
		return
	}
	var p gwsync.UserInvalidatePayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		logger.Error("sync: event user_invalidate: decode failed: " + err.Error())
		return
	}
	s.userCache.InvalidateAndRefresh(p.UserIDs)
	logger.Info("sync: event user_invalidate: applied count=" + strconv.Itoa(len(p.UserIDs)))
}

// eventVersionGuard drops out-of-order event deliveries per entity (in-memory,
// per-pod — each pod only needs to protect its own cache writes).
type eventVersionGuard struct {
	mu   sync.Mutex
	seen map[string]int64
}

func newEventVersionGuard() *eventVersionGuard {
	return &eventVersionGuard{seen: make(map[string]int64)}
}

// allow reports whether version is newer than the last one seen for key.
func (g *eventVersionGuard) allow(key string, version int64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if last, ok := g.seen[key]; ok && version <= last {
		return false
	}
	g.seen[key] = version
	return true
}
