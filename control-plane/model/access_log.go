package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// AccessLog is one proxied request as the gateway published it on the usage-log
// stream. The shape mirrors oryca-gateway/logger/log_model.go — keep both sides
// in sync when changing.
type AccessLog struct {
	ID        bson.ObjectID `json:"id" bson:"_id,omitempty"`
	Time      time.Time     `json:"time" bson:"time"`
	TraceID   string        `json:"traceId,omitempty" bson:"traceId,omitempty"`
	UserID    string        `json:"userId,omitempty" bson:"userId,omitempty"`
	ApiKeyID  string        `json:"apiKeyId,omitempty" bson:"apiKeyId,omitempty"`
	ServiceID string        `json:"serviceId,omitempty" bson:"serviceId,omitempty"`

	Method     string `json:"method,omitempty" bson:"method,omitempty"`
	Path       string `json:"path,omitempty" bson:"path,omitempty"`
	Host       string `json:"host,omitempty" bson:"host,omitempty"`
	IP         string `json:"ip,omitempty" bson:"ip,omitempty"`
	StatusCode int    `json:"statusCode" bson:"statusCode"`
	Size       int64  `json:"size,omitempty" bson:"size,omitempty"`
	DurationMs int64  `json:"durationMs" bson:"durationMs"`
}

// AccessLogQuery is the filter shared by every dashboard endpoint. Scoping is
// applied on top of it in the repository — a caller cannot widen its own scope
// by sending a UserID it does not own.
type AccessLogQuery struct {
	From      time.Time
	To        time.Time
	UserID    string
	ServiceID string
	// Limit/Cursor paginate the raw log list only.
	Limit      int
	CursorTime *time.Time
	CursorID   *bson.ObjectID
}

// DashboardBucket is one point on the request-volume chart.
type DashboardBucket struct {
	Time      time.Time `json:"time"`
	Count     int64     `json:"count"`
	AvgTimeMs float64   `json:"avgTimeMs"`
	Status2xx int64     `json:"status2xx"`
	Status4xx int64     `json:"status4xx"`
	Status5xx int64     `json:"status5xx"`
}

// DashboardStats is the bucketed series plus the interval it was bucketed at.
type DashboardStats struct {
	Interval string             `json:"interval"` // "minute" | "hour" | "day"
	Buckets  []*DashboardBucket `json:"buckets"`
}

// DashboardServiceUsage is one row of the top-services table.
type DashboardServiceUsage struct {
	ServiceID string `json:"serviceId"`
	Name      string `json:"name,omitempty"`
	Count     int64  `json:"count"`
}

// DashboardSummary totals the whole selected range.
type DashboardSummary struct {
	Total       int64                    `json:"total"`
	AvgTimeMs   float64                  `json:"avgTimeMs"`
	Status2xx   int64                    `json:"status2xx"`
	Status4xx   int64                    `json:"status4xx"`
	Status5xx   int64                    `json:"status5xx"`
	TopServices []*DashboardServiceUsage `json:"topServices"`
}

// AccessLogPage is a cursor-paginated slice of raw logs. NextCursor is empty on
// the last page.
type AccessLogPage struct {
	Data       []*AccessLog `json:"data"`
	NextCursor string       `json:"nextCursor,omitempty"`
}
