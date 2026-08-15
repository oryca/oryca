// Package preset holds the response-transform templates the portal offers.
//
// Each template is a JSON file under templates/, embedded in the binary. They
// are a library to start from, not settings to edit. Applying one writes an
// ordinary transform config, which is then yours to change.
//
// Some templates suit one API standard, some suit any HTTP API at all. The ones
// with an empty types list are offered for every service.
package preset

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/oryca/oryca/control-plane/model"
)

//go:embed templates/*.json
var embedded embed.FS

// sourceURLVar is replaced with the upstream address when a template is applied.
// A template that leaves it out is asking to be filled in by hand, which is what
// the ones pointing at a second server do. Everything else in a rule is left for
// the gateway to resolve per request.
const sourceURLVar = "{{sourceUrl}}"

// Preset is one template. What it does, which service types it suits, the
// standard it follows, and the rules it writes.
type Preset struct {
	Name          string                `json:"name"`
	Types         []string              `json:"types"`
	Title         string                `json:"title"`
	Description   string                `json:"description"`
	Reference     string                `json:"reference,omitempty"`
	ReferenceNote string                `json:"referenceNote,omitempty"`
	Notes         []string              `json:"notes,omitempty"`
	Match         model.TransformMatch  `json:"match"`
	Rules         []model.TransformRule `json:"rules"`
}

// file is the shape on disk. Name and Title read better in a file as key and
// label, which is what the templates were written with.
type file struct {
	Key           string                `json:"key"`
	Label         string                `json:"label"`
	Types         []string              `json:"types"`
	Reference     string                `json:"reference"`
	ReferenceNote string                `json:"referenceNote"`
	Description   string                `json:"description"`
	Notes         []string              `json:"notes"`
	Match         model.TransformMatch  `json:"match"`
	Rules         []model.TransformRule `json:"rules"`
}

var presets []Preset

func init() {
	entries, err := fs.ReadDir(embedded, "templates")
	if err != nil {
		panic("preset: cannot read templates: " + err.Error())
	}
	for _, e := range entries {
		raw, err := embedded.ReadFile("templates/" + e.Name())
		if err != nil {
			panic("preset: cannot read " + e.Name() + ": " + err.Error())
		}
		var f file
		if err := json.Unmarshal(raw, &f); err != nil {
			panic("preset: cannot parse " + e.Name() + ": " + err.Error())
		}
		presets = append(presets, Preset{
			Name:          f.Key,
			Types:         f.Types,
			Title:         f.Label,
			Description:   f.Description,
			Reference:     f.Reference,
			ReferenceNote: f.ReferenceNote,
			Notes:         f.Notes,
			Match:         f.Match,
			Rules:         f.Rules,
		})
	}
	sort.Slice(presets, func(i, j int) bool { return presets[i].Name < presets[j].Name })
}

// All returns every template.
func All() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return out
}

// ForType returns the templates that suit a service type, which is the ones
// naming it plus the ones that name no type at all. An empty type returns all
// of them.
func ForType(serviceType string) []Preset {
	if serviceType == "" {
		return All()
	}
	var out []Preset
	for _, p := range presets {
		if len(p.Types) == 0 {
			out = append(out, p)
			continue
		}
		for _, t := range p.Types {
			if t == serviceType {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// Find looks a template up by name.
func Find(name string) (Preset, bool) {
	for _, p := range presets {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

// Rewrite returns the template's rules with the upstream address filled in, one
// copy per address. A service assembled from several upstreams needs a rule for
// each of them. Rules that do not mention the address are returned once.
func (p Preset) Rewrite(sourceURLs []string) []model.TransformRule {
	roots := upstreamRoots(sourceURLs)
	out := make([]model.TransformRule, 0, len(p.Rules))
	for _, rule := range p.Rules {
		if !mentionsSourceURL(rule) || len(roots) == 0 {
			out = append(out, rule)
			continue
		}
		for _, u := range roots {
			out = append(out, fillSourceURL(rule, u))
		}
	}
	return out
}

func mentionsSourceURL(rule model.TransformRule) bool {
	return strings.Contains(rule.Params.Find, sourceURLVar) ||
		strings.Contains(rule.Params.Regex, sourceURLVar)
}

// upstreamRoots turns the addresses a service points at into the prefixes worth
// rewriting.
//
// The query string and fragment are dropped first. A source address often
// carries the upstream's own credential, which must never reach a stored rule,
// and a link in a response never repeats it anyway, so a rule that kept it
// would match nothing.
//
// An address that sits under another in the list is then dropped. A service
// usually points at several addresses on the same server: the landing page,
// /collections, and so on. Rewriting each separately would map
// https://host/api/collections onto the gateway root and lose the path.
// Rewriting the shortest is enough, since everything below it keeps its tail.
func upstreamRoots(sourceURLs []string) []string {
	cleaned := make([]string, 0, len(sourceURLs))
	seen := make(map[string]struct{}, len(sourceURLs))
	for _, u := range sourceURLs {
		u = stripQuery(u)
		if u = strings.TrimRight(u, "/"); u == "" {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		cleaned = append(cleaned, u)
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

// stripQuery removes the query string and fragment, keeping the address itself.
// An address it cannot parse is cut at the first ? or # instead, so a malformed
// one still loses its credential.
func stripQuery(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		return raw[:i]
	}
	return raw
}

// fillSourceURL substitutes the address into the fields that can carry it. The
// regex field is quoted, since a URL is full of characters a regex reads as
// syntax.
func fillSourceURL(rule model.TransformRule, sourceURL string) model.TransformRule {
	rule.Params.Find = strings.ReplaceAll(rule.Params.Find, sourceURLVar, sourceURL)
	rule.Params.Regex = strings.ReplaceAll(rule.Params.Regex, sourceURLVar, regexp.QuoteMeta(sourceURL))
	return rule
}
