package model

type GatewayService struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	BasePath      string          `json:"basePath"`
	Enabled       bool            `json:"enabled"`
	IsPublic      bool            `json:"isPublic"`
	PackageIDs    []string        `json:"packageIds"`
	ResourcePaths []*ResourcePath `json:"resources"`
}

// RateLimitTier คือ 1 tier ของ sliding window — limit requests per windowSec
type RateLimitTier struct {
	Limit     int `json:"limit"`
	WindowSec int `json:"windowSec"`
}

type ResourcePath struct {
	Path              string                     `json:"path"`
	Methods           []string                   `json:"methods"`
	SourceAlias       string                     `json:"sourceAlias"`
	MaxRespBytes      int64                      `json:"maxRespBytes,omitempty"`
	TransformConfigID string                     `json:"transformConfigId,omitempty"`
	RateLimit         map[string][]RateLimitTier `json:"rateLimit,omitempty"` // packageID → tiers
}

type Source struct {
	Alias       string            `json:"alias"`
	Type        string            `json:"type"`               // "upstream" | "static"
	Protocol    string            `json:"protocol,omitempty"` // "http" | "https"
	URL         string            `json:"url,omitempty"`      // upstream only
	Headers     []*SourceKeyValue `json:"headers,omitempty"`
	ContentType string            `json:"contentType,omitempty"` // static only
	Body        string            `json:"body,omitempty"`        // static only
}

type SourceKeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
