package trie

import (
	"context"
	"github.com/oryca/oryca/gateway/cache"
	"github.com/oryca/oryca/gateway/logger"
	"github.com/oryca/oryca/gateway/model"
	gwsync "github.com/oryca/oryca/gateway/sync"
	"github.com/oryca/oryca/gateway/tool"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// trieCache abstracts cache operations used by HybridTrie. Allows injection in tests.
type trieCache interface {
	SaveServices(ctx context.Context, services []*model.GatewayService) error
	SaveSources(ctx context.Context, sources []*gwsync.SourcePayload) error
	LoadAllServices(ctx context.Context) ([]*model.GatewayService, error)
	LoadAllSources(ctx context.Context) (map[string]*model.Source, error)
}

type HybridTrie struct {
	trie         *PathTrie
	cache        trieCache
	provider     gwsync.SyncProvider
	ready        atomic.Bool
	sourceMu     sync.RWMutex
	sourceMap    map[string]*model.Source
	transformMu  sync.RWMutex
	transformMap map[string][]*gwsync.TransformConfigPayload // serviceId → configs sorted by specificity
	sfGroup      singleflight.Group
	lastSync     atomic.Value // time.Time
}

func NewHybrid(c *cache.Cache, provider gwsync.SyncProvider) *HybridTrie {
	return &HybridTrie{
		trie:         New(),
		cache:        c,
		provider:     provider,
		sourceMap:    make(map[string]*model.Source),
		transformMap: make(map[string][]*gwsync.TransformConfigPayload),
	}
}

func (h *HybridTrie) IsReady() bool {
	return h.ready.Load()
}

func (h *HybridTrie) LastSync() time.Time {
	if t, ok := h.lastSync.Load().(time.Time); ok {
		return t
	}
	return time.Time{}
}

// Load โหลด services + sources จาก Redis ตอน startup (ก่อนที่ provider จะ sync)
func (h *HybridTrie) Load(ctx context.Context) error {
	if err := h.reloadSourcesFromCache(ctx); err != nil {
		logger.Error("trie: load sources from cache failed: " + err.Error())
	}

	services, err := h.cache.LoadAllServices(ctx)
	if err != nil {
		logger.Error("trie: load services from cache failed: " + err.Error())
		return nil
	}
	if len(services) == 0 {
		logger.Info("trie: no services in Redis yet — starting in not-ready state")
		return nil
	}
	h.trie.LoadBulk(services)
	h.ready.Store(true)
	logger.Info("trie: loaded " + itoa(len(services)) + " services from Redis cache")
	return nil
}

// DoSync pulls services+sources from provider and updates trie + Redis cache.
// Uses singleflight to deduplicate concurrent calls.
func (h *HybridTrie) DoSync(ctx context.Context) {
	h.sfGroup.Do("sync", func() (interface{}, error) {
		// Sources ก่อนเสมอ เพราะ services อ้าง sourceAlias
		h.syncSources(ctx)
		h.syncServices(ctx)
		h.syncTransformConfigs(ctx)
		h.lastSync.Store(time.Now())
		return nil, nil
	})
}

func (h *HybridTrie) syncSources(ctx context.Context) {
	sources, err := h.provider.GetSources(ctx)
	if err != nil {
		logger.Error("trie: get sources failed: " + err.Error())
		return
	}
	if sources == nil { // nil = 304 not modified
		return
	}
	if len(sources) == 0 {
		h.sourceMu.RLock()
		existing := len(h.sourceMap)
		h.sourceMu.RUnlock()
		if existing > 0 {
			logger.Error("trie: provider returned 0 sources, preserving existing " + itoa(existing))
		}
		return
	}
	_ = h.cache.SaveSources(ctx, sources)
	newMap := make(map[string]*model.Source, len(sources))
	for _, s := range sources {
		newMap[s.Alias] = toModelSource(s)
	}
	h.sourceMu.Lock()
	h.sourceMap = newMap
	h.sourceMu.Unlock()
	logger.Info("trie: sources synced count=" + itoa(len(sources)))
}

func (h *HybridTrie) syncServices(ctx context.Context) {
	services, err := h.provider.GetServices(ctx)
	if err != nil {
		logger.Error("trie: get services failed: " + err.Error())
		return
	}
	if services == nil { // nil = 304 not modified
		return
	}
	if len(services) == 0 {
		if h.ready.Load() {
			logger.Error("trie: provider returned 0 services, preserving existing trie")
		}
		return
	}
	modelSvcs := toModelServices(services)
	_ = h.cache.SaveServices(ctx, modelSvcs)
	h.trie.LoadBulk(modelSvcs)
	h.ready.Store(true)
	logger.Info("trie: services synced count=" + itoa(len(services)))
}

func (h *HybridTrie) syncTransformConfigs(ctx context.Context) {
	cfgs, err := h.provider.GetTransformConfigs(ctx)
	if err != nil {
		logger.Error("trie: get transform configs failed: " + err.Error())
		return
	}
	if cfgs == nil { // nil = 304 not modified
		return
	}

	// group by serviceId
	newMap := make(map[string][]*gwsync.TransformConfigPayload)
	for _, cfg := range cfgs {
		if !cfg.Enabled {
			continue
		}
		newMap[cfg.ServiceID] = append(newMap[cfg.ServiceID], cfg)
	}

	// sort each group by path specificity desc (most specific first)
	for _, list := range newMap {
		sortBySpecificity(list)
	}

	h.transformMu.Lock()
	h.transformMap = newMap
	h.transformMu.Unlock()
	logger.Info("trie: transform configs synced count=" + itoa(len(cfgs)))
}

// FindTransformConfig returns the first transform config that matches serviceID, path and method.
func (h *HybridTrie) FindTransformConfig(serviceID, path, method string) *gwsync.TransformConfigPayload {
	h.transformMu.RLock()
	cfgs := h.transformMap[serviceID]
	h.transformMu.RUnlock()

	for _, cfg := range cfgs {
		if matchWildcardPath(cfg.Match.Path, path) && matchMethod(cfg.Match.Methods, method) {
			return cfg
		}
	}
	return nil
}

// matchWildcardPath checks if pattern (with * wildcard) matches path.
func matchWildcardPath(pattern, path string) bool {
	if pattern == "*" || pattern == "/*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == path
	}
	// prefix wildcard: /collections/* matches /collections/anything
	prefix := strings.TrimSuffix(pattern, "*")
	return strings.HasPrefix(path, prefix)
}

func matchMethod(methods []string, method string) bool {
	if len(methods) == 0 {
		return true
	}
	for _, m := range methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

// sortBySpecificity sorts configs most-specific first using segment scoring.
// exact: segments×10, wildcard: segments×8 (consistent with claystone trie scoring)
func sortBySpecificity(cfgs []*gwsync.TransformConfigPayload) {
	for i := 1; i < len(cfgs); i++ {
		for j := i; j > 0 && pathScore(cfgs[j].Match.Path) > pathScore(cfgs[j-1].Match.Path); j-- {
			cfgs[j], cfgs[j-1] = cfgs[j-1], cfgs[j]
		}
	}
}

func pathScore(pattern string) int {
	segments := len(strings.Split(strings.Trim(pattern, "/"), "/"))
	if strings.HasSuffix(pattern, "*") {
		return segments * 8
	}
	return segments * 10
}

// StartPolling polls provider every interval with ±20% jitter to avoid thundering herd.
func (h *HybridTrie) StartPolling(ctx context.Context, interval time.Duration) {
	logger.GoLoop("trie: poll loop", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(tool.JitteredInterval(interval)):
				h.DoSync(ctx)
			}
		}
	})
}

func (h *HybridTrie) reloadSourcesFromCache(ctx context.Context) error {
	sources, err := h.cache.LoadAllSources(ctx)
	if err != nil {
		return err
	}
	h.sourceMu.Lock()
	h.sourceMap = sources
	h.sourceMu.Unlock()
	logger.Info("trie: source registry loaded from cache, " + itoa(len(sources)) + " sources")
	return nil
}

func (h *HybridTrie) GetSource(alias string) (*model.Source, bool) {
	h.sourceMu.RLock()
	src, ok := h.sourceMap[alias]
	h.sourceMu.RUnlock()
	return src, ok
}

func (h *HybridTrie) FindBestMatch(path string) *PathMatch {
	return h.trie.FindBestMatch(path)
}

func (h *HybridTrie) Insert(svc *model.GatewayService) {
	h.trie.Insert(svc)
	if !h.ready.Load() {
		h.ready.Store(true)
	}
}

func (h *HybridTrie) Delete(serviceID string) {
	h.trie.Delete(serviceID)
}

func toModelSource(s *gwsync.SourcePayload) *model.Source {
	return &model.Source{
		Alias:       s.Alias,
		Type:        s.Type,
		Protocol:    s.Protocol,
		URL:         s.URL,
		ContentType: s.ContentType,
		Body:        s.Body,
		Headers:     toModelHeaders(s.Headers),
	}
}

func toModelHeaders(hs []*gwsync.SourceKeyValue) []*model.SourceKeyValue {
	if len(hs) == 0 {
		return nil
	}
	out := make([]*model.SourceKeyValue, len(hs))
	for i, h := range hs {
		out[i] = &model.SourceKeyValue{Key: h.Key, Value: h.Value}
	}
	return out
}

func toModelServices(svcs []*gwsync.ServicePayload) []*model.GatewayService {
	result := make([]*model.GatewayService, 0, len(svcs))
	for _, s := range svcs {
		svc := &model.GatewayService{
			ID:         s.ID,
			Name:       s.Name,
			BasePath:   s.BasePath,
			Enabled:    s.Enabled,
			IsPublic:   s.IsPublic,
			PackageIDs: s.PackageIDs,
		}
		svc.ResourcePaths = make([]*model.ResourcePath, 0, len(s.Resources))
		for _, r := range s.Resources {
			svc.ResourcePaths = append(svc.ResourcePaths, &model.ResourcePath{
				Path:        r.Path,
				Methods:     r.Methods,
				SourceAlias: r.SourceAlias,
				RateLimit:   toModelRateLimit(r.RateLimit),
			})
		}
		result = append(result, svc)
	}
	return result
}

func toModelRateLimit(rl map[string]*gwsync.RateLimitPayload) map[string][]model.RateLimitTier {
	if len(rl) == 0 {
		return nil
	}
	m := make(map[string][]model.RateLimitTier, len(rl))
	for pkgID, payload := range rl {
		if payload == nil || len(payload.Tiers) == 0 {
			continue
		}
		tiers := make([]model.RateLimitTier, len(payload.Tiers))
		for i, t := range payload.Tiers {
			tiers[i] = model.RateLimitTier{Limit: t.Limit, WindowSec: t.WindowSec}
		}
		m[pkgID] = tiers
	}
	return m
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
