package service

import (
	"context"
	"crypto/rsa"
	"fmt"
	"github.com/oryca/oryca/gateway/cache"
	"github.com/oryca/oryca/gateway/logger"
	"github.com/oryca/oryca/gateway/model"
	gwsync "github.com/oryca/oryca/gateway/sync"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtClaims. Internal API JWT (legacy) ที่ CP ออกเอง เซ็นด้วย ORYCA_API_JWT key เดียว
type jwtClaims struct {
	Session   string `json:"ses"`
	UserID    string `json:"uid"`
	GrantType string `json:"grt"`
	Role      string `json:"rol,omitempty"`
	PackageID string `json:"pkg,omitempty"`
	jwt.RegisteredClaims
}

type AuthService struct {
	provider  gwsync.SyncProvider
	cache     *cache.Cache
	userCache *cache.UserFreshnessCache
	publicKey *rsa.PublicKey
	mu        sync.RWMutex
}

func NewAuthService(provider gwsync.SyncProvider, c *cache.Cache, userCache *cache.UserFreshnessCache) *AuthService {
	return &AuthService{provider: provider, cache: c, userCache: userCache}
}

// overlayFreshness overwrites user's packageId/enabled/verified/expiredAt with the
// latest known values from the per-pod UserFreshnessCache. Replacing whatever was baked
// into a JWT at login time (fixed for the token's whole lifetime) or embedded in a
// cached api-key's Owner snapshot (refreshed only ~every ApiKeyPollInterval). Get() never
// blocks on network, so this adds no request latency on the hot path.
//
// A cache miss, fetch error, or not-found id all leave user untouched (fail open) —
// deliberately, including 404, which would otherwise 404 forever if treated as
// fail-closed. userCache == nil (e.g. tests constructing AuthService directly) is a
// no-op, so existing behavior/tests are unaffected.
func (a *AuthService) overlayFreshness(user *model.User) {
	if a.userCache == nil || user == nil || user.ID == "" {
		return
	}
	fresh, err := a.userCache.Get(user.ID)
	if err != nil || fresh == nil {
		return
	}
	user.PackageID = fresh.PackageID
	user.Verified = fresh.Verified
	user.Enabled = fresh.Enabled
	user.ExpiredAt = fresh.ExpiredAt
}

// FetchJWKS fetches the internal API JWT key set from control-plane and caches it in memory.
func (a *AuthService) FetchJWKS(ctx context.Context) error {
	pub, err := a.provider.GetJWKS(ctx)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.publicKey = pub
	a.mu.Unlock()
	logger.Info("auth: JWKS fetched and public key cached")
	return nil
}

// StartJWKSRefresh refreshes the key set every interval to handle key rotation.
func (a *AuthService) StartJWKSRefresh(ctx context.Context, interval time.Duration) {
	logger.GoLoop("auth: JWKS refresh loop", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pub, err := a.provider.GetJWKS(ctx)
				if err != nil {
					logger.Error("auth: JWKS refresh failed: " + err.Error())
					continue
				}
				a.mu.Lock()
				a.publicKey = pub
				a.mu.Unlock()
				logger.Info("auth: JWKS refreshed")
			}
		}
	})
}

// ValidateBearer validates a JWT Bearer token and returns the associated user.
func (a *AuthService) ValidateBearer(ctx context.Context, tokenStr string) (*model.User, error) {
	a.mu.RLock()
	pub := a.publicKey
	a.mu.RUnlock()

	return a.validateInternalBearer(tokenStr, pub)
}

func (a *AuthService) validateInternalBearer(tokenStr string, pub *rsa.PublicKey) (*model.User, error) {
	if pub == nil {
		return nil, fmt.Errorf("public key not loaded")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &jwtClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pub, nil
	})
	if err != nil || !token.Valid {
		msg := "invalid token"
		if err != nil {
			msg = err.Error()
		}
		logger.Error("auth: token parse failed: " + msg)
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*jwtClaims)
	if !ok {
		logger.Error("auth: unexpected claims type")
		return nil, fmt.Errorf("invalid token claims")
	}
	if claims.GrantType != "AccessToken" {
		logger.Error("auth: invalid claims grt=" + claims.GrantType)
		return nil, fmt.Errorf("invalid token claims")
	}

	user := &model.User{
		ID:        claims.UserID,
		Role:      claims.Role,
		PackageID: claims.PackageID,
	}
	a.overlayFreshness(user)
	return user, nil
}

// ValidateApiKey validates an API key and returns the owning user and key metadata.
func (a *AuthService) ValidateApiKey(ctx context.Context, keyValue string) (*model.User, *model.ApiKey, error) {
	ak, err := a.cache.GetApiKey(ctx, keyValue)
	if err != nil {
		return nil, nil, fmt.Errorf("apikey lookup failed")
	}
	if ak == nil {
		return nil, nil, fmt.Errorf("api key not found")
	}

	if ak.Enabled != nil && !*ak.Enabled {
		return nil, nil, fmt.Errorf("api key disabled")
	}
	if ak.ExpiredAt != nil && ak.ExpiredAt.Before(time.Now().UTC()) {
		return nil, nil, fmt.Errorf("api key expired")
	}

	if ak.Owner == nil {
		return nil, nil, fmt.Errorf("api key owner not found")
	}
	a.overlayFreshness(ak.Owner)

	return ak.Owner, ak, nil
}
