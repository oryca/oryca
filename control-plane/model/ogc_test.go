package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMissingOGCPaths(t *testing.T) {
	features := []string{
		"/", "/conformance", "/collections",
		"/collections/{collectionId}",
		"/collections/{collectionId}/items",
		"/collections/{collectionId}/items/{featureId}",
	}

	t.Run("a complete Features service is missing nothing", func(t *testing.T) {
		assert.Empty(t, MissingOGCPaths(TypeOGCAPIFeatures, features))
	})

	t.Run("a different parameter name still counts as the same path", func(t *testing.T) {
		paths := []string{
			"/", "/conformance", "/collections",
			"/collections/{id}",
			"/collections/{id}/items",
		}
		assert.Empty(t, MissingOGCPaths(TypeOGCAPIFeatures, paths))
	})

	t.Run("a trailing wildcard covers everything below it", func(t *testing.T) {
		paths := []string{"/", "/conformance", "/collections", "/collections/*"}
		assert.Empty(t, MissingOGCPaths(TypeOGCAPIFeatures, paths))
	})

	t.Run("a trailing slash is the same path", func(t *testing.T) {
		paths := []string{"/", "/conformance/", "/collections/", "/collections/{id}", "/collections/{id}/items"}
		assert.Empty(t, MissingOGCPaths(TypeOGCAPIFeatures, paths))
	})

	t.Run("what is not covered is reported", func(t *testing.T) {
		missing := MissingOGCPaths(TypeOGCAPIFeatures, []string{"/", "/collections"})
		assert.ElementsMatch(t, []string{
			"/conformance",
			"/collections/{collectionId}",
			"/collections/{collectionId}/items",
			"/collections/{collectionId}/items/{featureId}",
		}, missing)
	})

	t.Run("a plain REST service is never warned about", func(t *testing.T) {
		assert.Nil(t, MissingOGCPaths(TypeGeneral, []string{"/anything"}))
		assert.Nil(t, MissingOGCPaths("", nil))
	})

	t.Run("SensorThings is checked on its entity sets, not /conformance", func(t *testing.T) {
		// All eight entity sets of Part 1 Sensing, and no /conformance, which
		// SensorThings advertises from its landing page instead.
		missing := MissingOGCPaths(TypeOGCAPISensorThings, []string{
			"/", "/Things", "/Locations", "/HistoricalLocations",
			"/Datastreams", "/Sensors", "/ObservedProperties",
			"/Observations", "/FeaturesOfInterest",
		})
		assert.Empty(t, missing)
		assert.NotContains(t, MissingOGCPaths(TypeOGCAPISensorThings, nil), "/conformance")
	})

	t.Run("an empty service reports every core path", func(t *testing.T) {
		assert.Len(t, MissingOGCPaths(TypeOGCAPIFeatures, nil), len(features))
	})
}
