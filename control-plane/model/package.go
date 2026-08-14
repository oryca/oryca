package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type RateLimitTier struct {
	Limit     int `json:"limit" bson:"limit"`
	WindowSec int `json:"windowSec" bson:"windowSec"`
}

type PackageRateLimit struct {
	Enabled   bool             `json:"enabled" bson:"enabled"`
	Algorithm string           `json:"-" bson:"algorithm,omitempty"` // ซ่อนจาก API, default "sliding_window" เสมอ
	Tiers     []*RateLimitTier `json:"tiers,omitempty" bson:"tiers,omitempty"`
}

type PackageQuota struct {
	Enabled bool   `json:"enabled" bson:"enabled"`
	Limit   int    `json:"limit,omitempty" bson:"limit,omitempty"`
	Period  string `json:"period,omitempty" bson:"period,omitempty"` // "monthly" | "daily" | "hourly"
}

type PackageAccess struct {
	AllowedRoles []string `json:"allowedRoles,omitempty" bson:"allowedRoles,omitempty"`
}

type PathAccess struct {
	IPWhitelist []string `json:"-" bson:"ipWhitelist,omitempty"` // ซ่อนจาก API, เก็บใน DB ได้
}

type PackagePolicies struct {
	RateLimit *PackageRateLimit `json:"rateLimit,omitempty" bson:"rateLimit,omitempty"`
	Quota     *PackageQuota     `json:"quota,omitempty" bson:"quota,omitempty"`
	Access    *PackageAccess    `json:"access,omitempty" bson:"access,omitempty"`
}

type PathPolicies struct {
	RateLimit *PackageRateLimit `json:"rateLimit,omitempty" bson:"rateLimit,omitempty"`
	Access    *PathAccess       `json:"access,omitempty" bson:"access,omitempty"`
}

type PackagePath struct {
	Path     string        `json:"path" bson:"path"`
	Methods  []string      `json:"methods,omitempty" bson:"methods,omitempty"`
	Policies *PathPolicies `json:"policies,omitempty" bson:"policies,omitempty"`
}

type Package struct {
	ID           bson.ObjectID          `json:"id" bson:"_id,omitempty"`
	Name         string                 `json:"name" bson:"name"`
	Alias        string                 `json:"alias" bson:"alias"`
	Description  string                 `json:"description,omitempty" bson:"description,omitempty"`
	Enabled      *bool                  `json:"enabled,omitempty" bson:"enabled,omitempty"`
	Policies     *PackagePolicies       `json:"policies,omitempty" bson:"policies,omitempty"`
	Properties   map[string]interface{} `json:"properties,omitempty" bson:"properties,omitempty"`
	UserCount    int64                  `json:"userCount" bson:"userCount"`
	ServiceCount int64                  `json:"serviceCount" bson:"serviceCount"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
	DeletedAt *time.Time     `json:"deletedAt,omitempty" bson:"deletedAt,omitempty"`
	DeletedBy *bson.ObjectID `json:"deletedBy,omitempty" bson:"deletedBy,omitempty"`
}

type PackageCreate struct {
	Name        string                 `json:"name"`
	Alias       string                 `json:"alias"`
	Description string                 `json:"description"`
	Enabled     *bool                  `json:"enabled"`
	Policies    *PackagePolicies       `json:"policies"`
	Properties  map[string]interface{} `json:"properties"`
}

type PackageUpdate struct {
	Name        string                 `json:"name"`
	Alias       string                 `json:"alias"`
	Description string                 `json:"description"`
	Enabled     *bool                  `json:"enabled"`
	Policies    *PackagePolicies       `json:"policies"`
	Properties  map[string]interface{} `json:"properties"`
}

// PackageSvcLink is stored in package_services collection — links a package to a gateway service with per-path policies
type PackageSvcLink struct {
	ID        bson.ObjectID  `json:"id" bson:"_id,omitempty"`
	PackageID bson.ObjectID  `json:"packageId" bson:"packageId"`
	ServiceID bson.ObjectID  `json:"serviceId" bson:"serviceId"`
	Paths     []*PackagePath `json:"paths,omitempty" bson:"paths,omitempty"`

	CreatedAt *time.Time     `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
}

type PackageSummary struct {
	ID          bson.ObjectID `json:"id" bson:"_id"`
	Name        string        `json:"name,omitempty" bson:"name,omitempty"`
	Alias       string        `json:"alias,omitempty" bson:"alias,omitempty"`
	Description string        `json:"description,omitempty" bson:"description,omitempty"`
}

type ServiceSummary struct {
	ID          bson.ObjectID `json:"id" bson:"_id"`
	Name        string        `json:"name,omitempty" bson:"name,omitempty"`
	Description string        `json:"description,omitempty" bson:"description,omitempty"`
	Type        string        `json:"type,omitempty" bson:"type,omitempty"`
	BasePath    string        `json:"basePath,omitempty" bson:"basePath,omitempty"`
}

// PackageSvcLinkDetail enriches PackageSvcLink with gateway service and package info for API responses
type PackageSvcLinkDetail struct {
	ID        bson.ObjectID   `json:"id" bson:"_id"`
	Package   *PackageSummary `json:"package,omitempty" bson:"package,omitempty"`
	Service   *ServiceSummary `json:"service,omitempty" bson:"service,omitempty"`
	Paths     []*PackagePath  `json:"paths,omitempty" bson:"paths,omitempty"`
	CreatedAt *time.Time      `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	CreatedBy *bson.ObjectID  `json:"createdBy,omitempty" bson:"createdBy,omitempty"`
	UpdatedAt *time.Time      `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	UpdatedBy *bson.ObjectID  `json:"updatedBy,omitempty" bson:"updatedBy,omitempty"`
}

type PackageSvcLinkCreate struct {
	ServiceID string         `json:"serviceId"`
	Paths     []*PackagePath `json:"paths"`
}

type PackageSvcLinkUpdate struct {
	Paths []*PackagePath `json:"paths"`
}

type PackageServiceCheckPathsRequest struct {
	ServiceID string         `json:"serviceId"`
	Paths     []*PackagePath `json:"paths"`
}

type PackageServiceCheckPathsResult struct {
	Path   string `json:"path"`
	Valid  bool   `json:"valid"`
	Reason string `json:"reason,omitempty"`
}

type PackageServiceCheckPathsResponse struct {
	HasInvalid bool                              `json:"hasInvalid"`
	Results    []*PackageServiceCheckPathsResult `json:"results"`
}

type PackageUserAssign struct {
	UserIDs []string `json:"userIds"`
}

type PackageUserRemove struct {
	UserIDs []string `json:"userIds"`
}
