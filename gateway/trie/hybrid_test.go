package trie

import (
	"context"
	"crypto/rsa"
	"errors"
	"github.com/oryca/oryca/gateway/model"
	gwsync "github.com/oryca/oryca/gateway/sync"
	"testing"
)

// stubProvider implements gwsync.SyncProvider for testing
type stubProvider struct {
	services []*gwsync.ServicePayload
	sources  []*gwsync.SourcePayload
	err      error
}

func (s *stubProvider) GetServices(ctx context.Context) ([]*gwsync.ServicePayload, error) {
	return s.services, s.err
}
func (s *stubProvider) GetSources(ctx context.Context) ([]*gwsync.SourcePayload, error) {
	return s.sources, s.err
}
func (s *stubProvider) GetApiKeys(ctx context.Context) ([]*gwsync.ApiKeyPayload, error) {
	return nil, nil
}
func (s *stubProvider) GetJWKS(ctx context.Context) (*rsa.PublicKey, error) { return nil, nil }
func (s *stubProvider) GetTransformConfigs(ctx context.Context) ([]*gwsync.TransformConfigPayload, error) {
	return nil, nil
}
func (s *stubProvider) GetUser(ctx context.Context, userID string) (*gwsync.ApiKeyOwnerPayload, error) {
	return nil, nil
}

// noopCache satisfies trieCache without requiring Redis
type noopCache struct{}

func (n *noopCache) SaveServices(_ context.Context, _ []*model.GatewayService) error { return nil }
func (n *noopCache) SaveSources(_ context.Context, _ []*gwsync.SourcePayload) error  { return nil }
func (n *noopCache) LoadAllServices(_ context.Context) ([]*model.GatewayService, error) {
	return nil, nil
}
func (n *noopCache) LoadAllSources(_ context.Context) (map[string]*model.Source, error) {
	return map[string]*model.Source{}, nil
}

func newTestHybrid(provider gwsync.SyncProvider) *HybridTrie {
	return &HybridTrie{
		trie:      New(),
		cache:     &noopCache{},
		provider:  provider,
		sourceMap: make(map[string]*model.Source),
	}
}

func TestDoSync_LoadsServices(t *testing.T) {
	provider := &stubProvider{
		services: []*gwsync.ServicePayload{
			{ID: "svc1", Name: "Test", BasePath: "/test", Enabled: true,
				Resources: []*gwsync.ResourcePayload{{Path: "/data", Methods: []string{"GET"}}}},
		},
	}
	h := newTestHybrid(provider)
	h.DoSync(context.Background())

	if !h.IsReady() {
		t.Fatal("hybrid should be ready after sync")
	}
	m := h.FindBestMatch("/test/data")
	if m == nil || m.Service.ID != "svc1" {
		t.Errorf("expected svc1, got %v", m)
	}
}

func TestDoSync_PreservesServicesOnEmptyResponse(t *testing.T) {
	provider := &stubProvider{
		services: []*gwsync.ServicePayload{
			{ID: "svc1", BasePath: "/test", Enabled: true,
				Resources: []*gwsync.ResourcePayload{{Path: "/data", Methods: []string{"GET"}}}},
		},
	}
	h := newTestHybrid(provider)
	h.DoSync(context.Background())

	// provider ส่ง 0 services → ต้องไม่ล้าง trie
	provider.services = []*gwsync.ServicePayload{}
	h.DoSync(context.Background())

	m := h.FindBestMatch("/test/data")
	if m == nil || m.Service.ID != "svc1" {
		t.Error("trie should be preserved when provider returns 0 services")
	}
}

func TestDoSync_PreservesServicesOnError(t *testing.T) {
	provider := &stubProvider{
		services: []*gwsync.ServicePayload{
			{ID: "svc1", BasePath: "/test", Enabled: true,
				Resources: []*gwsync.ResourcePayload{{Path: "/data", Methods: []string{"GET"}}}},
		},
	}
	h := newTestHybrid(provider)
	h.DoSync(context.Background())

	// provider error → ต้องไม่ล้าง trie
	provider.err = errors.New("provider down")
	h.DoSync(context.Background())

	m := h.FindBestMatch("/test/data")
	if m == nil || m.Service.ID != "svc1" {
		t.Error("trie should be preserved when provider errors")
	}
}

func TestDoSync_LoadsSources(t *testing.T) {
	provider := &stubProvider{
		sources: []*gwsync.SourcePayload{
			{Alias: "gistda", Type: "REST", URL: "http://upstream.example.com"},
		},
	}
	h := newTestHybrid(provider)
	h.DoSync(context.Background())

	src, ok := h.GetSource("gistda")
	if !ok || src.URL != "http://upstream.example.com" {
		t.Errorf("expected gistda source, got %v %v", ok, src)
	}
}

func TestGetSource_NotFound(t *testing.T) {
	h := newTestHybrid(&stubProvider{})
	_, ok := h.GetSource("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestIsReady_FalseBeforeSync(t *testing.T) {
	h := newTestHybrid(&stubProvider{})
	if h.IsReady() {
		t.Error("should not be ready before sync")
	}
}
