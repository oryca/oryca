package preset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForType(t *testing.T) {
	assert.NotEmpty(t, ForType("OGC_API_Features"))
	assert.NotEmpty(t, ForType("OGC_API_SensorThings"))
	assert.Empty(t, ForType("General"), "a plain REST service has nothing to rewrite")
	assert.Equal(t, len(All()), len(ForType("")))
}

func TestRewriteUsesTheUpstreamRoot(t *testing.T) {
	p, ok := Find("rewrite-links")
	require.True(t, ok)

	// A service usually registers the landing page and a path or two below it.
	// Rewriting the deeper ones as well would map /master/collections onto the
	// gateway root and drop the path.
	rules := p.Rewrite([]string{
		"https://demo.example.io/master",
		"https://demo.example.io/master/collections",
		"https://demo.example.io/master/collections/",
	})

	require.Len(t, rules, 1)
	assert.Equal(t, "https://demo.example.io/master", rules[0].Params.Find)
	assert.Equal(t, "{{oryca_gateway_url}}", rules[0].Params.Replace)
}

func TestRewriteKeepsUnrelatedUpstreams(t *testing.T) {
	p, ok := Find("rewrite-links")
	require.True(t, ok)

	rules := p.Rewrite([]string{"https://a.example/api", "https://b.example/api"})

	require.Len(t, rules, 2)
	assert.ElementsMatch(t,
		[]string{"https://a.example/api", "https://b.example/api"},
		[]string{rules[0].Params.Find, rules[1].Params.Find})
}

func TestRewriteQuotesTheRegexForm(t *testing.T) {
	p, ok := Find("rewrite-sensorthings-navigation")
	require.True(t, ok)

	rules := p.Rewrite([]string{"https://sensors.example/v1.1"})

	require.Len(t, rules, 1)
	// The dot in v1.1 must not stay a regex wildcard.
	assert.Equal(t, `^https://sensors\.example/v1\.1`, rules[0].Params.Regex)
}
