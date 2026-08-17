package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func tiersFixture() map[string]*gwRateLimitPayload {
	return map[string]*gwRateLimitPayload{
		"pkg1": {Tiers: []gwRateLimitTierPayload{{Limit: 20, WindowSec: 1}}},
	}
}

func TestFindBestRateLimit(t *testing.T) {
	t.Run("exact path", func(t *testing.T) {
		m := map[string]map[string]*gwRateLimitPayload{"/ping": tiersFixture()}
		assert.NotNil(t, findBestRateLimit("/ping", m))
	})

	t.Run("param segment normalizes to a wildcard grant", func(t *testing.T) {
		m := map[string]map[string]*gwRateLimitPayload{"/tiles/*": tiersFixture()}
		assert.NotNil(t, findBestRateLimit("/tiles/{z}", m))
	})

	t.Run("prefix wildcard covers a deeper path", func(t *testing.T) {
		m := map[string]map[string]*gwRateLimitPayload{"/tiles/*": tiersFixture()}
		assert.NotNil(t, findBestRateLimit("/tiles/a/b", m))
	})

	// A package that grants "/*" opens every path in the service, so every resource
	// has to carry the package's limit. Returning nil here ships the resource to the
	// gateway with no rateLimit at all, and the gateway then serves it unlimited.
	t.Run("root wildcard grant covers every resource", func(t *testing.T) {
		m := map[string]map[string]*gwRateLimitPayload{"/*": tiersFixture()}

		assert.NotNil(t, findBestRateLimit("/ping", m), "plain path under a root wildcard grant")
		assert.NotNil(t, findBestRateLimit("/tiles/{z}", m), "param path under a root wildcard grant")
		assert.NotNil(t, findBestRateLimit("/a/b/c", m), "nested path under a root wildcard grant")
	})

	t.Run("the most specific wildcard wins", func(t *testing.T) {
		specific := map[string]*gwRateLimitPayload{
			"pkg1": {Tiers: []gwRateLimitTierPayload{{Limit: 5, WindowSec: 1}}},
		}
		m := map[string]map[string]*gwRateLimitPayload{
			"/*":       tiersFixture(),
			"/tiles/*": specific,
		}
		got := findBestRateLimit("/tiles/{z}", m)
		assert.Equal(t, 5, got["pkg1"].Tiers[0].Limit, "/tiles/* must beat /*")
	})

	t.Run("no grant matches", func(t *testing.T) {
		m := map[string]map[string]*gwRateLimitPayload{"/other/*": tiersFixture()}
		assert.Nil(t, findBestRateLimit("/ping", m))
	})
}
