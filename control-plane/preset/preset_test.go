package preset

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForType(t *testing.T) {
	// every service type the product accepts has something to offer
	for _, t2 := range []string{
		"OGC_API_Features", "OGC_API_STAC", "OGC_API_Styles",
		"OGC_API_Tiles", "OGC_API_SensorThings", "General",
	} {
		assert.NotEmpty(t, ForType(t2), t2+" has nothing to offer")
	}
	assert.Equal(t, len(All()), len(ForType("")))
}

func TestTemplatesWithoutATypeSuitEverything(t *testing.T) {
	// The action templates describe one kind of rule rather than one standard,
	// so they belong on a plain REST service as much as on an OGC one.
	general := ForType("General")
	names := map[string]bool{}
	for _, p := range general {
		names[p.Name] = true
	}
	assert.True(t, names["action-replace"], "action-replace should be offered for a plain API")
	assert.True(t, names["blank"], "blank should be offered for a plain API")
}

func TestRewriteUsesTheUpstreamRoot(t *testing.T) {
	p, ok := Find("ogc-api-features")
	require.True(t, ok)

	// A service usually registers the landing page and a path or two below it.
	// Rewriting the deeper ones as well would map /master/collections onto the
	// gateway root and drop the path.
	rules := p.Rewrite([]string{
		"https://demo.example.io/master",
		"https://demo.example.io/master/collections",
		"https://demo.example.io/master/collections/",
	})

	var filled int
	for _, r := range rules {
		if r.Params.Find != "" {
			filled++
			assert.Equal(t, "https://demo.example.io/master", r.Params.Find)
		}
	}
	assert.Equal(t, 1, filled, "one upstream root means one filled rule")
}

func TestRewriteDropsTheQueryString(t *testing.T) {
	p, ok := Find("ogc-api-features")
	require.True(t, ok)

	// A source address often carries the upstream's own credential. It must not
	// reach a stored rule, and a link in a response never repeats it, so a rule
	// that kept it would match nothing.
	rules := p.Rewrite([]string{"https://demo.example.io/master?api_key=SECRET"})

	for _, r := range rules {
		assert.NotContains(t, r.Params.Find, "SECRET")
		assert.NotContains(t, r.Params.Regex, "SECRET")
	}
	assert.Equal(t, "https://demo.example.io/master", rules[0].Params.Find)
}

func TestRewriteKeepsUnrelatedUpstreams(t *testing.T) {
	p, ok := Find("ogc-api-features")
	require.True(t, ok)

	rules := p.Rewrite([]string{"https://a.example/api", "https://b.example/api"})

	found := map[string]bool{}
	for _, r := range rules {
		found[r.Params.Find] = true
	}
	assert.True(t, found["https://a.example/api"] && found["https://b.example/api"])
}

func TestRewriteQuotesTheRegexForm(t *testing.T) {
	rule := fillSourceURL(model.TransformRule{
		Params: model.TransformParams{Regex: "^" + sourceURLVar},
	}, "https://sensors.example/v1.1")

	// The dot in v1.1 must not stay a regex wildcard.
	assert.Equal(t, `^https://sensors\.example/v1\.1`, rule.Params.Regex)
}

func TestEveryTemplateStripsTheUpstreamCredential(t *testing.T) {
	// A link an upstream returns can carry that upstream's key. Rewriting the
	// address alone would hand it to the caller, so each template that rewrites
	// links also replaces the credential with the caller's own.
	for _, name := range []string{"ogc-api-features", "stac", "wmts-xml", "ogc-sensorthings-v2"} {
		p, ok := Find(name)
		require.True(t, ok, name)

		var strips bool
		for _, r := range p.Rules {
			if strings.Contains(r.Params.Regex, "api_key") && r.Params.Replace == "{{oryca_auth}}" {
				strips = true
				break
			}
		}
		assert.True(t, strips, name+" does not replace the upstream credential")
	}
}

func TestTemplatesCarryNoRealAddress(t *testing.T) {
	// Templates are read by people deciding what to change. Anything that looks
	// like a real deployment invites them to leave it in place.
	for _, p := range All() {
		raw, err := json.Marshal(p)
		require.NoError(t, err)
		text := string(raw)
		assert.NotContains(t, text, "vallaris", p.Name)
		assert.NotContains(t, text, "gistda", p.Name)
	}
}

// JSON field names bind by tag, and a rule that loses headerName or notEquals
// loads with those parts silently missing.
func TestJSONKeysBindToTheRule(t *testing.T) {
	var rule model.TransformRule
	src := `{
		"type": "json",
		"target": "headers",
		"action": "replace",
		"headerName": "X-Example",
		"conditions": [{"field": "rel", "notEquals": ["resource"]}],
		"params": {"field": "href", "find": "a", "replace": "b"}
	}`
	require.NoError(t, json.Unmarshal([]byte(src), &rule))

	assert.Equal(t, "X-Example", rule.HeaderName)
	require.Len(t, rule.Conditions, 1)
	assert.Equal(t, []interface{}{"resource"}, rule.Conditions[0].NotEquals)
	assert.Equal(t, "href", rule.Params.Field)
}
