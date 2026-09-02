package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"github.com/oryca/oryca/gateway/cache"
	"github.com/oryca/oryca/gateway/model"
	gwsync "github.com/oryca/oryca/gateway/sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubUserProvider implements gwsync.SyncProvider. Only GetUser is used, by
// cache.NewUserFreshnessCache in the overlayFreshness tests below.
type stubUserProvider struct {
	result *gwsync.ApiKeyOwnerPayload
	err    error
}

func (s *stubUserProvider) GetServices(ctx context.Context) ([]*gwsync.ServicePayload, error) {
	return nil, nil
}
func (s *stubUserProvider) GetSources(ctx context.Context) ([]*gwsync.SourcePayload, error) {
	return nil, nil
}
func (s *stubUserProvider) GetApiKeys(ctx context.Context) ([]*gwsync.ApiKeyPayload, error) {
	return nil, nil
}
func (s *stubUserProvider) GetJWKS(ctx context.Context) (*rsa.PublicKey, error) { return nil, nil }
func (s *stubUserProvider) GetTransformConfigs(ctx context.Context) ([]*gwsync.TransformConfigPayload, error) {
	return nil, nil
}
func (s *stubUserProvider) GetUser(ctx context.Context, userID string) (*gwsync.ApiKeyOwnerPayload, error) {
	return s.result, s.err
}

func mustGenerateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func signInternalToken(t *testing.T, key *rsa.PrivateKey, claims jwtClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	require.NoError(t, err)
	return tok
}

func TestValidateBearer_InternalToken(t *testing.T) {
	key := mustGenerateRSAKey(t)
	a := &AuthService{publicKey: &key.PublicKey}

	claims := jwtClaims{
		UserID:    "user-1",
		GrantType: "AccessToken",
		Role:      "admin",
		PackageID: "pkg-1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := signInternalToken(t, key, claims)

	user, err := a.ValidateBearer(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, "user-1", user.ID)
	assert.Equal(t, "admin", user.Role)
	assert.Equal(t, "pkg-1", user.PackageID)
}

func TestValidateBearer_InternalToken_WrongGrantType(t *testing.T) {
	key := mustGenerateRSAKey(t)
	a := &AuthService{publicKey: &key.PublicKey}

	claims := jwtClaims{UserID: "user-1", GrantType: "RefreshToken"}
	tok := signInternalToken(t, key, claims)

	_, err := a.ValidateBearer(context.Background(), tok)
	assert.Error(t, err)
}

// --- overlayFreshness ---

func TestOverlayFreshness_OverwritesPackageIDVerifiedEnabledExpiredAt(t *testing.T) {
	userCache := cache.NewUserFreshnessCache(&stubUserProvider{}, time.Minute, 5)
	verified := true
	enabled := false
	expiredAt := time.Now().Add(24 * time.Hour)
	userCache.Set("user-1", &gwsync.ApiKeyOwnerPayload{
		ID: "user-1", PackageID: "pkg-fresh", Verified: &verified, Enabled: &enabled, ExpiredAt: &expiredAt,
	})

	a := &AuthService{userCache: userCache}
	user := &model.User{ID: "user-1", PackageID: "pkg-stale", Role: "admin"}

	a.overlayFreshness(user)

	assert.Equal(t, "pkg-fresh", user.PackageID)
	require.NotNil(t, user.Verified)
	assert.True(t, *user.Verified)
	require.NotNil(t, user.Enabled)
	assert.False(t, *user.Enabled)
	require.NotNil(t, user.ExpiredAt)
	// Role is out of scope for this fix. Still JWT-derived, untouched.
	assert.Equal(t, "admin", user.Role)
}

func TestOverlayFreshness_CacheMissLeavesUserUntouched(t *testing.T) {
	userCache := cache.NewUserFreshnessCache(&stubUserProvider{err: gwsync.ErrUserNotFound}, time.Minute, 5)
	a := &AuthService{userCache: userCache}
	user := &model.User{ID: "never-cached", PackageID: "pkg-from-jwt"}

	a.overlayFreshness(user)

	assert.Equal(t, "pkg-from-jwt", user.PackageID) // true miss → fail open, untouched
}

func TestOverlayFreshness_NilUserCacheIsNoOp(t *testing.T) {
	a := &AuthService{} // userCache left nil, matching every pre-existing test in this file
	user := &model.User{ID: "user-1", PackageID: "pkg-from-jwt"}

	a.overlayFreshness(user)

	assert.Equal(t, "pkg-from-jwt", user.PackageID)
}

func TestValidateBearer_InternalToken_OverlaysFreshnessFromCache(t *testing.T) {
	key := mustGenerateRSAKey(t)
	userCache := cache.NewUserFreshnessCache(&stubUserProvider{}, time.Minute, 5)
	verified := true
	userCache.Set("user-1", &gwsync.ApiKeyOwnerPayload{ID: "user-1", PackageID: "pkg-fresh", Verified: &verified})

	a := &AuthService{publicKey: &key.PublicKey, userCache: userCache}
	claims := jwtClaims{
		UserID:    "user-1",
		GrantType: "AccessToken",
		PackageID: "pkg-stale-from-jwt", // must be overwritten by the cache
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := signInternalToken(t, key, claims)

	user, err := a.ValidateBearer(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, "pkg-fresh", user.PackageID)
}
