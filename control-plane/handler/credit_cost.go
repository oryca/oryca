package handler

import (
	"regexp"
	"strings"

	"github.com/oryca/oryca/control-plane/model"
)

var paramSegmentRe = regexp.MustCompile(`\{[^/]+\}`)

// gwRateLimitTierPayload คือ 1 tier ของ sliding window rate limit
type gwRateLimitTierPayload struct {
	Limit     int `json:"limit"`     // max requests per window
	WindowSec int `json:"windowSec"` // window size in seconds
}

// gwRateLimitPayload rate limit config ต่อ package สำหรับ resource path
type gwRateLimitPayload struct {
	Tiers []gwRateLimitTierPayload `json:"tiers"`
}

type gwResourcePayload struct {
	Path        string                         `json:"path"`
	Methods     []string                       `json:"methods"`
	SourceAlias string                         `json:"sourceAlias"`
	RateLimit   map[string]*gwRateLimitPayload `json:"rateLimit,omitempty"` // packageID → rate limit
}

type gwServicePayload struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	BasePath   string               `json:"basePath"`
	Enabled    bool                 `json:"enabled"`
	IsPublic   bool                 `json:"isPublic"`
	PackageIDs []string             `json:"packageIds,omitempty"`
	Resources  []*gwResourcePayload `json:"resources"`
}

// buildRateLimitMap turns the package-to-service links into the packageIDs list and the rate limit
// map the gateway expects. packageLimits carries each package's own limit, used for any path that
// does not set one of its own — set it once on the package, override it where a path needs
// something different.
func buildRateLimitMap(links []*model.PackageSvcLink, packageLimits map[string]*model.PackageRateLimit) ([]string, map[string]map[string]*gwRateLimitPayload) {
	packageIDs := make([]string, 0, len(links))
	rateLimitMap := make(map[string]map[string]*gwRateLimitPayload)
	for _, link := range links {
		pkgID := link.PackageID.Hex()
		packageIDs = append(packageIDs, pkgID)
		for _, pp := range link.Paths {
			buildPathRateLimit(pp, pkgID, packageLimits[pkgID], rateLimitMap)
		}
	}
	return packageIDs, rateLimitMap
}

// effectiveRateLimit is the path's own limit when it has an enabled one, and the package's
// otherwise. A path that sets enabled=false turns the limit off for itself, rather than falling
// back to the package.
func effectiveRateLimit(pp *model.PackagePath, pkgLimit *model.PackageRateLimit) *model.PackageRateLimit {
	if pp.Policies != nil && pp.Policies.RateLimit != nil {
		return pp.Policies.RateLimit
	}
	return pkgLimit
}

func buildPathRateLimit(pp *model.PackagePath, pkgID string, pkgLimit *model.PackageRateLimit, rateLimitMap map[string]map[string]*gwRateLimitPayload) {
	rl := effectiveRateLimit(pp, pkgLimit)
	if rl == nil || !rl.Enabled {
		return
	}
	if len(rl.Tiers) == 0 {
		return
	}
	tiers := make([]gwRateLimitTierPayload, 0, len(rl.Tiers))
	for _, t := range rl.Tiers {
		if t.Limit > 0 && t.WindowSec > 0 {
			tiers = append(tiers, gwRateLimitTierPayload{Limit: t.Limit, WindowSec: t.WindowSec})
		}
	}
	if len(tiers) == 0 {
		return
	}
	if _, ok := rateLimitMap[pp.Path]; !ok {
		rateLimitMap[pp.Path] = make(map[string]*gwRateLimitPayload)
	}
	rateLimitMap[pp.Path][pkgID] = &gwRateLimitPayload{Tiers: tiers}
}

// buildServicePayload แปลง GatewayService → gwServicePayload พร้อม RateLimit per path per package
func buildServicePayload(svc *model.GatewayService, packageIDs []string, rateLimitMap map[string]map[string]*gwRateLimitPayload) *gwServicePayload {
	enabled := svc.Enabled != nil && *svc.Enabled
	isPublic := svc.IsPublic != nil && *svc.IsPublic

	resources := make([]*gwResourcePayload, 0, len(svc.ResourcePaths))
	for _, rp := range svc.ResourcePaths {
		r := &gwResourcePayload{
			Path:        rp.Path,
			Methods:     rp.Methods,
			SourceAlias: rp.SourceAlias,
		}
		if rl := findBestRateLimit(rp.Path, rateLimitMap); len(rl) > 0 {
			r.RateLimit = rl
		}
		resources = append(resources, r)
	}

	return &gwServicePayload{
		ID:         svc.ID.Hex(),
		Name:       svc.Name,
		BasePath:   svc.BasePath,
		Enabled:    enabled,
		IsPublic:   isPublic,
		PackageIDs: packageIDs,
		Resources:  resources,
	}
}

// findBestRateLimit หา rate limit map ที่ตรงกับ resourcePath โดย:
// 1. exact match
// 2. normalize {param} → * แล้ว exact match
// 3. prefix wildcard match (หา pattern/* ที่ specific ที่สุด)
func findBestRateLimit(resourcePath string, rateLimitMap map[string]map[string]*gwRateLimitPayload) map[string]*gwRateLimitPayload {
	if rl, ok := rateLimitMap[resourcePath]; ok && len(rl) > 0 {
		return rl
	}
	normalized := paramSegmentRe.ReplaceAllString(resourcePath, "*")
	if rl, ok := rateLimitMap[normalized]; ok && len(rl) > 0 {
		return rl
	}
	var bestPrefix string
	var best map[string]*gwRateLimitPayload
	for pattern, rl := range rateLimitMap {
		if !strings.HasSuffix(pattern, "/*") || len(rl) == 0 {
			continue
		}
		prefix := strings.TrimSuffix(pattern, "/*")
		if strings.HasPrefix(normalized, prefix+"/") && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			best = rl
		}
	}
	return best
}
