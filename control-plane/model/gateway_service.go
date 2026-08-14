package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type GatewayService struct {
	ID            bson.ObjectID   `json:"id" bson:"_id,omitempty"`
	Name          string          `json:"name" bson:"name"`
	Description   string          `json:"description,omitempty" bson:"description,omitempty"`
	Type          string          `json:"type" bson:"type"` // General | OGC_API_Features | OGC_API_STAC | OGC_API_Styles | OGC_API_SensorThings | OGC_API_Tiles
	BasePath      string          `json:"basePath" bson:"basePath"`
	Enabled       *bool           `json:"enabled,omitempty" bson:"enabled,omitempty"`
	IsPublic      *bool           `json:"isPublic,omitempty" bson:"isPublic,omitempty"`
	ResourcePaths []*ResourcePath `json:"resourcePaths,omitempty" bson:"resourcePaths,omitempty"`
	OGC           *OGCInfo        `json:"ogc,omitempty" bson:"ogc,omitempty"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

type ResourcePathSourceInput struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        string            `json:"type"`
	Protocol    string            `json:"protocol"`
	URL         string            `json:"url"`
	Headers     []*SourceKeyValue `json:"headers"`
	ContentType string            `json:"contentType"`
	Body        string            `json:"body"`
}

type ResourcePath struct {
	Path        string                   `json:"path" bson:"path"`
	Methods     []string                 `json:"methods" bson:"methods"`
	SourceAlias string                   `json:"sourceAlias" bson:"sourceAlias"`
	Source      *ResourcePathSourceInput `json:"source,omitempty" bson:"-"`
}

// ResourcePathWithSource is the aggregate result. Source is embedded by $lookup
type ResourcePathWithSource struct {
	Path        string         `json:"path" bson:"path"`
	Methods     []string       `json:"methods" bson:"methods"`
	SourceAlias string         `json:"sourceAlias" bson:"sourceAlias"`
	Source      *GatewaySource `json:"source,omitempty" bson:"source,omitempty"`
}

// InvalidResourcePath describes a resourcePath entry that failed validation
type InvalidResourcePath struct {
	Index       int    `json:"index"`
	Path        string `json:"path"`
	SourceAlias string `json:"sourceAlias"`
}

// GatewayServiceWithSource is returned by aggregate queries. ResourcePaths include embedded source
type GatewayServiceWithSource struct {
	ID            bson.ObjectID             `json:"id" bson:"_id,omitempty"`
	Name          string                    `json:"name" bson:"name"`
	Description   string                    `json:"description,omitempty" bson:"description,omitempty"`
	Type          string                    `json:"type" bson:"type"`
	BasePath      string                    `json:"basePath" bson:"basePath"`
	Enabled       *bool                     `json:"enabled,omitempty" bson:"enabled,omitempty"`
	IsPublic      *bool                     `json:"isPublic,omitempty" bson:"isPublic,omitempty"`
	ResourcePaths []*ResourcePathWithSource `json:"resourcePaths,omitempty" bson:"resourcePaths,omitempty"`
	OGC           *OGCInfo                  `json:"ogc,omitempty" bson:"ogc,omitempty"`
	CreatedAt     *time.Time                `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy     *bson.ObjectID            `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt     *time.Time                `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy     *bson.ObjectID            `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt     *time.Time                `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy     *bson.ObjectID            `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

type GatewayServiceCreate struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Type          string          `json:"type"`
	BasePath      string          `json:"basePath"`
	Enabled       *bool           `json:"enabled"`
	IsPublic      *bool           `json:"isPublic"`
	ResourcePaths []*ResourcePath `json:"resourcePaths"`
	OGC           *OGCInfo        `json:"ogc,omitempty"`
}

type GatewayServiceUpdate struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Type          string          `json:"type"`
	BasePath      string          `json:"basePath"`
	Enabled       *bool           `json:"enabled"`
	IsPublic      *bool           `json:"isPublic"`
	ResourcePaths []*ResourcePath `json:"resourcePaths"`
	OGC           *OGCInfo        `json:"ogc,omitempty"`
}

type GatewayServiceCheckPathsRequest struct {
	// Type is optional; when it names an OGC standard the response also reports
	// which of its core endpoints are not covered.
	Type             string   `json:"type,omitempty"`
	BasePath         string   `json:"basePath"`
	ResourcePaths    []string `json:"resourcePaths"`
	ExcludeServiceID string   `json:"excludeServiceId,omitempty"`
}

type BasePathConflictInfo struct {
	ConflictingServiceID   string `json:"conflictingServiceId"`
	ConflictingServiceName string `json:"conflictingServiceName"`
	ConflictingBasePath    string `json:"conflictingBasePath"`
}

type InternalPathConflict struct {
	ConflictingPaths []string `json:"conflictingPaths"`
}

type GatewayServiceCheckPathsResponse struct {
	HasConflict           bool                    `json:"hasConflict"`
	BasePathConflict      *BasePathConflictInfo   `json:"basePathConflict,omitempty"`
	ResourcePathConflicts []*InternalPathConflict `json:"resourcePathConflicts,omitempty"`
	// MissingOGCPaths is advisory: the paths are still accepted as given.
	MissingOGCPaths []string `json:"missingOgcPaths,omitempty"`
}
