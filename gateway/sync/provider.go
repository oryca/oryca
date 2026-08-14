package sync

import (
	"context"
	"crypto/rsa"
)

// SyncProvider abstracts the data source for gateway configuration data.
// Current implementation: HTTP polling. Future: gRPC/xDS.
type SyncProvider interface {
	GetServices(ctx context.Context) ([]*ServicePayload, error)
	GetSources(ctx context.Context) ([]*SourcePayload, error)
	GetApiKeys(ctx context.Context) ([]*ApiKeyPayload, error)
	GetJWKS(ctx context.Context) (*rsa.PublicKey, error)
	GetTransformConfigs(ctx context.Context) ([]*TransformConfigPayload, error)
	// GetUser fetches one user's freshness snapshot on cache miss. Called synchronously
	// in the request path, unlike the background-polling methods above, so implementations
	// must not retry-with-backoff.
	GetUser(ctx context.Context, userID string) (*ApiKeyOwnerPayload, error)
}
