package sync

import "time"

// RateLimitTierPayload คือ 1 tier ของ sliding window rate limit ที่ sync มาจาก CP
type RateLimitTierPayload struct {
	Limit     int `json:"limit"`
	WindowSec int `json:"windowSec"`
}

// RateLimitPayload rate limit config ต่อ package สำหรับ resource path
type RateLimitPayload struct {
	Tiers []RateLimitTierPayload `json:"tiers"`
}

type ResourcePayload struct {
	Path        string                       `json:"path"`
	Methods     []string                     `json:"methods"`
	SourceAlias string                       `json:"sourceAlias"`
	RateLimit   map[string]*RateLimitPayload `json:"rateLimit,omitempty"` // packageID → rate limit
}

type ServicePayload struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	BasePath   string             `json:"basePath"`
	Enabled    bool               `json:"enabled"`
	IsPublic   bool               `json:"isPublic"`
	PackageIDs []string           `json:"packageIds,omitempty"`
	Resources  []*ResourcePayload `json:"resources"`
}

type SourceKeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type SourcePayload struct {
	Alias       string            `json:"alias"`
	Type        string            `json:"type"`
	Protocol    string            `json:"protocol,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     []*SourceKeyValue `json:"headers,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	Body        string            `json:"body,omitempty"`
}

// ApiKeyOwnerPayload is also reused as the gateway-facing user-freshness snapshot
// (GET /internal/users/:id response). Control-plane's model.UserFreshnessCache is a
// subset of these same fields (no Role), which unmarshal cleanly as zero values here
// since auth overlay only reads PackageID/Verified/Enabled/ExpiredAt.
type ApiKeyOwnerPayload struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	PackageID string     `json:"packageId,omitempty"`
	Verified  *bool      `json:"verified,omitempty"`
	Enabled   *bool      `json:"enabled,omitempty"`
	ExpiredAt *time.Time `json:"expiredAt,omitempty"`
}

// UserInvalidatePayload is the thin bulk-invalidation signal for EventTypeUserInvalidate.
// Ids only, no data, mirroring control-plane's model.UserIDsPayload.
type UserInvalidatePayload struct {
	UserIDs []string `json:"userIds"`
}

type ApiKeyPayload struct {
	ID        string              `json:"id"`
	ApiKey    string              `json:"apiKey"`
	Enabled   *bool               `json:"enabled,omitempty"`
	ExpiredAt *time.Time          `json:"expiredAt,omitempty"`
	Owner     *ApiKeyOwnerPayload `json:"owner,omitempty"`
}

type TransformMatchPayload struct {
	Path    string   `json:"path"`
	Methods []string `json:"methods,omitempty"`
}

type TransformRulePayload struct {
	Type       string                      `json:"type"`
	Target     string                      `json:"target"`
	Action     string                      `json:"action"`
	Path       string                      `json:"path,omitempty"`
	XPath      string                      `json:"xpath,omitempty"`
	HeaderName string                      `json:"headerName,omitempty"`
	Conditions []TransformConditionPayload `json:"conditions,omitempty"`
	Params     TransformParamsPayload      `json:"params,omitempty"`
}

type TransformConditionPayload struct {
	Field     string        `json:"field"`
	Equals    []interface{} `json:"equals,omitempty"`
	NotEquals []interface{} `json:"notEquals,omitempty"`
}

type TransformParamsPayload struct {
	Find      string      `json:"find,omitempty"`
	Replace   string      `json:"replace,omitempty"`
	Regex     string      `json:"regex,omitempty"`
	Value     interface{} `json:"value,omitempty"`
	Separator string      `json:"separator,omitempty"`
	From      string      `json:"from,omitempty"`
	To        string      `json:"to,omitempty"`
	Field     string      `json:"field,omitempty"`
}

type TransformConfigPayload struct {
	ID        string                 `json:"id"`
	ServiceID string                 `json:"serviceId"`
	Match     TransformMatchPayload  `json:"match"`
	Enabled   bool                   `json:"enabled"`
	Rules     []TransformRulePayload `json:"rules"`
}
