package model

import (
	"regexp"
	"strings"
)

// Service types. Anything other than TypeGeneral names an OGC API standard.
const (
	TypeGeneral            = "General"
	TypeOGCAPIFeatures     = "OGC_API_Features"
	TypeOGCAPISTAC         = "OGC_API_STAC"
	TypeOGCAPIStyles       = "OGC_API_Styles"
	TypeOGCAPISensorThings = "OGC_API_SensorThings"
	TypeOGCAPITiles        = "OGC_API_Tiles"
)

// OGCInfo records which version of the standard the upstream implements and which
// parts of it. Metadata only: it shows up in the catalog and the docs and never
// affects routing.
type OGCInfo struct {
	Version string   `json:"version,omitempty" bson:"version,omitempty"`
	Parts   []string `json:"parts,omitempty" bson:"parts,omitempty"`
}

// ogcCorePaths lists the endpoints a client of each standard expects to find.
// They are deliberately the core ones only: registering a service that exposes
// just a slice of a standard is legitimate, so a missing path is a warning to the
// person filling the form, never a reason to reject what they typed.
//
// ORYCA does not implement any of these standards. It sits in front of servers
// that do, and knows their shapes well enough to suggest paths and rewrite links.
// The versions each set follows are noted per entry; a server on a different
// version may legitimately answer at other paths.
var ogcCorePaths = map[string][]string{
	// OGC API - Features - Part 1: Core 1.0 (OGC 17-069r4, also ISO 19168-1)
	// The API definition is reached through the landing page's service-desc link
	// rather than a fixed path, so it is not listed.
	TypeOGCAPIFeatures: {
		"/", "/conformance", "/collections",
		"/collections/{collectionId}",
		"/collections/{collectionId}/items",
		"/collections/{collectionId}/items/{featureId}",
	},
	// STAC API 1.0. A community specification, not an OGC standard, though its
	// item access follows OGC API - Features closely, which is where the
	// collection and item paths come from.
	TypeOGCAPISTAC: {
		"/", "/conformance", "/collections",
		"/collections/{collectionId}",
		"/collections/{collectionId}/items",
		"/collections/{collectionId}/items/{featureId}",
		"/search",
	},
	// OGC API - Styles - Part 1: Core, still a draft at the time of writing, so
	// these paths can change before it is adopted.
	TypeOGCAPIStyles: {
		"/", "/conformance", "/styles",
		"/styles/{styleId}",
		"/styles/{styleId}/metadata",
	},
	// OGC API - Tiles - Part 1: Core 1.0 (OGC 20-057). Only the vector tileset
	// classes are listed. Map tiles and styled map tiles are separate
	// requirements classes that fewer servers implement, and listing every one
	// would bury the paths most services actually have. The standard says these
	// paths are examples rather than requirements, so a server that documents
	// others in its API definition is still conformant.
	TypeOGCAPITiles: {
		"/", "/conformance", "/tileMatrixSets",
		"/tileMatrixSets/{tileMatrixSetId}",
		"/tiles",
		"/tiles/{tileMatrixSetId}",
		"/tiles/{tileMatrixSetId}/{tileMatrix}/{tileRow}/{tileCol}",
		"/collections/{collectionId}/tiles",
	},
	// SensorThings API 1.0 and 1.1 (OGC 18-088). All eight entity sets of Part 1
	// Sensing. It advertises conformance from its landing page rather than a
	// /conformance endpoint, so that path is not listed.
	TypeOGCAPISensorThings: {
		"/", "/Things", "/Locations", "/HistoricalLocations",
		"/Datastreams", "/Sensors", "/ObservedProperties",
		"/Observations", "/FeaturesOfInterest",
	},
}

var ogcParamSegment = regexp.MustCompile(`\{[^/]+\}`)

// OGCCorePaths returns the endpoints expected for a service type, or nil when the
// type is not an OGC standard.
func OGCCorePaths(serviceType string) []string {
	return ogcCorePaths[serviceType]
}

// MissingOGCPaths reports which of the expected endpoints the given resource
// paths do not cover. Path parameters are compared by shape, so
// /collections/{id} satisfies /collections/{collectionId}, and a trailing
// wildcard covers everything below it.
func MissingOGCPaths(serviceType string, resourcePaths []string) []string {
	expected := ogcCorePaths[serviceType]
	if len(expected) == 0 {
		return nil
	}

	configured := make([]string, 0, len(resourcePaths))
	for _, p := range resourcePaths {
		configured = append(configured, normalizeOGCPath(p))
	}

	var missing []string
	for _, want := range expected {
		if !ogcPathCovered(normalizeOGCPath(want), configured) {
			missing = append(missing, want)
		}
	}
	return missing
}

// normalizeOGCPath makes two paths comparable: parameters become *, and a
// trailing slash is dropped so /collections and /collections/ are the same path.
func normalizeOGCPath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = ogcParamSegment.ReplaceAllString(p, "*")
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

func ogcPathCovered(want string, configured []string) bool {
	for _, have := range configured {
		if have == want {
			return true
		}
		if prefix, ok := strings.CutSuffix(have, "/*"); ok && strings.HasPrefix(want, prefix+"/") {
			return true
		}
	}
	return false
}
