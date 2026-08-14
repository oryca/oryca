package cache

import (
	"context"
	"crypto/rsa"
	"sync"
	"testing"
	"time"

	gwsync "github.com/oryca/oryca/gateway/sync"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUserProvider implements gwsync.SyncProvider. Only GetUser is exercised by these
// tests. Tracks per-userID call counts and concurrent-in-flight count, and signals a
// channel after each call completes so tests can wait for background work deterministically
// instead of sleeping and hoping.
type fakeUserProvider struct {
	mu            sync.Mutex
	calls         map[string]int
	current       int
	maxConcurrent int
	done          chan string
	resultFn      func(userID string) (*gwsync.ApiKeyOwnerPayload, error)
	delay         time.Duration
}

func newFakeUserProvider() *fakeUserProvider {
	return &fakeUserProvider{calls: make(map[string]int), done: make(chan string, 10000)}
}

func (f *fakeUserProvider) GetServices(ctx context.Context) ([]*gwsync.ServicePayload, error) {
	return nil, nil
}
func (f *fakeUserProvider) GetSources(ctx context.Context) ([]*gwsync.SourcePayload, error) {
	return nil, nil
}
func (f *fakeUserProvider) GetApiKeys(ctx context.Context) ([]*gwsync.ApiKeyPayload, error) {
	return nil, nil
}
func (f *fakeUserProvider) GetJWKS(ctx context.Context) (*rsa.PublicKey, error) { return nil, nil }
func (f *fakeUserProvider) GetTransformConfigs(ctx context.Context) ([]*gwsync.TransformConfigPayload, error) {
	return nil, nil
}

func (f *fakeUserProvider) GetUser(ctx context.Context, userID string) (*gwsync.ApiKeyOwnerPayload, error) {
	f.mu.Lock()
	f.calls[userID]++
	f.current++
	if f.current > f.maxConcurrent {
		f.maxConcurrent = f.current
	}
	fn := f.resultFn
	delay := f.delay
	f.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}

	f.mu.Lock()
	f.current--
	f.mu.Unlock()

	var result *gwsync.ApiKeyOwnerPayload
	var err error
	if fn != nil {
		result, err = fn(userID)
	}
	f.done <- userID
	return result, err
}

func (f *fakeUserProvider) callCount(userID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[userID]
}

func (f *fakeUserProvider) totalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, c := range f.calls {
		total += c
	}
	return total
}

// waitPackageID waits for the cached entry to carry the expected package.
// The provider signals done before the cache stores the result, so waiting on that
// signal alone can read the old value on a slow machine.
func waitPackageID(t *testing.T, c *UserFreshnessCache, userID, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got, err := c.Get(userID); err == nil && got != nil && got.PackageID == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to become %s", userID, want)
}

// waitDone drains n signals from the done channel (or fails the test after a timeout).
func waitDone(t *testing.T, f *fakeUserProvider, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-f.done:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for background refresh #%d/%d", i+1, n)
		}
	}
}

func testPayload(id, packageID string) *gwsync.ApiKeyOwnerPayload {
	return &gwsync.ApiKeyOwnerPayload{ID: id, PackageID: packageID}
}

func TestUserFreshnessCache_Get_HitSkipsProvider(t *testing.T) {
	provider := newFakeUserProvider()
	c := NewUserFreshnessCache(provider, time.Minute, 5)
	c.Set("u1", testPayload("u1", "pkg-a"))

	got, err := c.Get("u1")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "pkg-a", got.PackageID)
	assert.Equal(t, 0, provider.callCount("u1")) // no network call on a fresh hit
}

func TestUserFreshnessCache_Get_TrueMissFailsOpenAndSchedulesRefresh(t *testing.T) {
	provider := newFakeUserProvider()
	provider.resultFn = func(userID string) (*gwsync.ApiKeyOwnerPayload, error) {
		return testPayload(userID, "pkg-fresh"), nil
	}
	c := NewUserFreshnessCache(provider, time.Minute, 5)

	got, err := c.Get("u1")
	require.NoError(t, err)
	assert.Nil(t, got) // nothing to serve yet — caller fails open

	waitDone(t, provider, 1)
	waitPackageID(t, c, "u1", "pkg-fresh")
}

func TestUserFreshnessCache_Get_ConcurrentMissForSameUserDedupedBySingleflight(t *testing.T) {
	provider := newFakeUserProvider()
	provider.delay = 20 * time.Millisecond
	provider.resultFn = func(userID string) (*gwsync.ApiKeyOwnerPayload, error) {
		return testPayload(userID, "pkg-a"), nil
	}
	c := NewUserFreshnessCache(provider, time.Minute, 10)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Get("same-user")
		}()
	}
	wg.Wait()

	// give the (deduped) single background fetch time to land
	waitDone(t, provider, 1)

	assert.Equal(t, 1, provider.callCount("same-user"))
}

func TestUserFreshnessCache_Get_ExpiredServesStaleAndRefreshesInBackground(t *testing.T) {
	provider := newFakeUserProvider()
	provider.resultFn = func(userID string) (*gwsync.ApiKeyOwnerPayload, error) {
		return testPayload(userID, "pkg-new"), nil
	}
	c := NewUserFreshnessCache(provider, time.Millisecond, 5)
	c.Set("u1", testPayload("u1", "pkg-old"))
	time.Sleep(5 * time.Millisecond) // let ttl expire

	got, err := c.Get("u1") // must return immediately with the stale value, not block
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "pkg-old", got.PackageID) // stale-while-revalidate

	waitPackageID(t, c, "u1", "pkg-new")
}

func TestUserFreshnessCache_Get_BurstOfDistinctMissesBoundedByConcurrency(t *testing.T) {
	const concurrency = 5
	const burst = 40
	provider := newFakeUserProvider()
	provider.delay = 15 * time.Millisecond
	provider.resultFn = func(userID string) (*gwsync.ApiKeyOwnerPayload, error) {
		return testPayload(userID, "pkg-a"), nil
	}
	c := NewUserFreshnessCache(provider, time.Minute, concurrency)

	for i := 0; i < burst; i++ {
		c.Get(userIDFor(i)) // every call is a true miss — never blocks
	}

	waitDone(t, provider, burst)

	provider.mu.Lock()
	maxConcurrent := provider.maxConcurrent
	provider.mu.Unlock()
	assert.LessOrEqual(t, maxConcurrent, concurrency)
	assert.Equal(t, burst, provider.totalCalls())
}

func userIDFor(i int) string {
	return "user-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
}

func TestUserFreshnessCache_Set_VisibleWithoutProviderCall(t *testing.T) {
	provider := newFakeUserProvider()
	c := NewUserFreshnessCache(provider, time.Minute, 5)

	c.Set("u1", testPayload("u1", "pkg-pushed"))
	got, err := c.Get("u1")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "pkg-pushed", got.PackageID)
	assert.Equal(t, 0, provider.callCount("u1"))
}

func TestUserFreshnessCache_InvalidateAndRefresh_EagerlyRefetches(t *testing.T) {
	provider := newFakeUserProvider()
	provider.resultFn = func(userID string) (*gwsync.ApiKeyOwnerPayload, error) {
		return testPayload(userID, "pkg-eager"), nil
	}
	c := NewUserFreshnessCache(provider, time.Minute, 5)
	c.Set("u1", testPayload("u1", "pkg-old"))
	c.Set("u2", testPayload("u2", "pkg-old"))

	c.InvalidateAndRefresh([]string{"u1", "u2"})

	// immediately after invalidate, entries are gone. Get would fail open if called now
	waitDone(t, provider, 2)

	// The provider signals done before the cache stores what it fetched, so wait
	// for the value rather than the signal.
	waitPackageID(t, c, "u1", "pkg-eager")
	waitPackageID(t, c, "u2", "pkg-eager")
}

func TestUserFreshnessCache_NegativeCaching_ErrorsCachedTemporarily(t *testing.T) {
	provider := newFakeUserProvider()
	callErr := assertUserNotFoundErr
	provider.resultFn = func(userID string) (*gwsync.ApiKeyOwnerPayload, error) {
		return nil, callErr
	}
	c := NewUserFreshnessCache(provider, time.Minute, 5)

	c.Get("client-id-not-a-user") // triggers first fetch attempt
	waitDone(t, provider, 1)

	// repeated Get calls within TTL must not trigger another provider call. The
	// failure itself is cached (protects against a permanently-404 user id
	// hammering control-plane on every request).
	for i := 0; i < 5; i++ {
		got, err := c.Get("client-id-not-a-user")
		assert.Nil(t, got)
		assert.Error(t, err)
	}
	assert.Equal(t, 1, provider.callCount("client-id-not-a-user"))
}

var assertUserNotFoundErr = gwsync.ErrUserNotFound
