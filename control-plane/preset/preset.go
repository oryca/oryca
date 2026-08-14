// Package preset holds the ready-made response transforms offered for OGC API
// services.
//
// They ship with the binary rather than living in the database: they are a
// library to pick from, not settings to edit. Applying one writes an ordinary
// transform config, which is then yours to change.
package preset

import (
	_ "embed"
	"regexp"
	"strings"

	"github.com/oryca/oryca/control-plane/model"

	"gopkg.in/yaml.v3"
)

//go:embed ogc-transforms.yaml
var embedded []byte

// sourceURLVar is replaced with the upstream address when a preset is applied.
// Everything else in a rule is left for the gateway to resolve per request.
const sourceURLVar = "{{sourceUrl}}"

// Preset is one offer: what it does, which service types it suits, and the rules
// it writes.
type Preset struct {
	Name        string                `yaml:"name" json:"name"`
	Types       []string              `yaml:"types" json:"types"`
	Title       string                `yaml:"title" json:"title"`
	Description string                `yaml:"description" json:"description"`
	Match       model.TransformMatch  `yaml:"match" json:"match"`
	Rules       []model.TransformRule `yaml:"rules" json:"rules"`
}

var presets []Preset

func init() {
	var file struct {
		Presets []Preset `yaml:"presets"`
	}
	if err := yaml.Unmarshal(embedded, &file); err != nil {
		panic("preset: cannot read ogc-transforms.yaml: " + err.Error())
	}
	presets = file.Presets
}

// All returns every preset.
func All() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return out
}

// ForType returns the presets that suit a service type. An empty type returns
// all of them.
func ForType(serviceType string) []Preset {
	if serviceType == "" {
		return All()
	}
	var out []Preset
	for _, p := range presets {
		for _, t := range p.Types {
			if t == serviceType {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// Find looks a preset up by name.
func Find(name string) (Preset, bool) {
	for _, p := range presets {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

// Rewrite returns the preset's rules with the upstream address filled in, one
// copy per address: a service assembled from several upstreams needs a rule for
// each of them.
func (p Preset) Rewrite(sourceURLs []string) []model.TransformRule {
	roots := upstreamRoots(sourceURLs)
	out := make([]model.TransformRule, 0, len(p.Rules)*len(roots))
	for _, rule := range p.Rules {
		for _, u := range roots {
			out = append(out, fillSourceURL(rule, u))
		}
	}
	return out
}

// upstreamRoots drops any address that sits under another one in the list.
//
// A service usually points at several addresses under the same server — the
// landing page, /collections, and so on. Rewriting each of them separately would
// map https://host/api/collections onto the gateway root and lose the path.
// Rewriting the shortest one is enough: everything below it keeps its tail.
func upstreamRoots(sourceURLs []string) []string {
	cleaned := make([]string, 0, len(sourceURLs))
	for _, u := range sourceURLs {
		if u = strings.TrimRight(u, "/"); u != "" {
			cleaned = append(cleaned, u)
		}
	}

	var roots []string
	for _, u := range cleaned {
		under := false
		for _, other := range cleaned {
			if other != u && strings.HasPrefix(u, other+"/") {
				under = true
				break
			}
		}
		if !under {
			roots = append(roots, u)
		}
	}
	return roots
}

// fillSourceURL substitutes the address into the fields that can carry it. The
// regex field is quoted, since a URL is full of characters a regex reads as
// syntax.
func fillSourceURL(rule model.TransformRule, sourceURL string) model.TransformRule {
	rule.Params.Find = strings.ReplaceAll(rule.Params.Find, sourceURLVar, sourceURL)
	rule.Params.Regex = strings.ReplaceAll(rule.Params.Regex, sourceURLVar, regexp.QuoteMeta(sourceURL))
	return rule
}
