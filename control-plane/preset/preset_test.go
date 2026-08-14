package preset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForType(t *testing.T) {
	// every service type the product accepts has something to offer
	for _, t2 := range []string{
		"OGC_API_Features", "OGC_API_STAC", "OGC_API_Styles",
		"OGC_API_Tiles", "OGC_API_SensorThings", "General",
	} {
		assert.NotEmpty(t, ForType(t2), t2+" has no preset")
	}
	assert.Equal(t, len(All()), len(ForType("")))
}

func TestRewriteUsesTheUpstreamRoot(t *testing.T) {
	p, ok := Find("features-links")
	require.True(t, ok)

	// A service usually registers the landing page and a path or two below it.
	// Rewriting the deeper ones as well would map /master/collections onto the
	// gateway root and drop the path.
	rules := p.Rewrite([]string{
		"https://demo.example.io/master",
		"https://demo.example.io/master/collections",
		"https://demo.example.io/master/collections/",
	})

	// three rules in the preset, one upstream root: three rules out
	require.Len(t, rules, 3)
	for _, r := range rules {
		assert.Equal(t, "https://demo.example.io/master", r.Params.Find)
		assert.Equal(t, "{{oryca_gateway_url}}", r.Params.Replace)
	}
}

func TestRewriteKeepsUnrelatedUpstreams(t *testing.T) {
	p, ok := Find("features-links")
	require.True(t, ok)

	rules := p.Rewrite([]string{"https://a.example/api", "https://b.example/api"})

	// three rules across two unrelated upstreams
	require.Len(t, rules, 6)
	found := map[string]bool{}
	for _, r := range rules {
		found[r.Params.Find] = true
	}
	assert.True(t, found["https://a.example/api"] && found["https://b.example/api"])
}

func TestRewriteQuotesTheRegexForm(t *testing.T) {
	p, ok := Find("sensorthings-links")
	require.True(t, ok)

	rules := p.Rewrite([]string{"https://sensors.example/v1.1"})

	require.Len(t, rules, 1)
	// The dot in v1.1 must not stay a regex wildcard.
	assert.Equal(t, `^https://sensors\.example/v1\.1`, rules[0].Params.Regex)
}
