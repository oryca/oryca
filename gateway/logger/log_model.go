package logger

import (
	"encoding/json"
	"time"
)

type Log struct {
	Time      *time.Time `json:"time"`
	Level     string     `json:"level"`
	Message   string     `json:"message"`
	Hostname  string     `json:"hostname,omitempty"`
	Service   string     `json:"service,omitempty"`
	TraceID   string     `json:"traceId,omitempty"`
	UserID    string     `json:"userId,omitempty"`
	ApiKeyID  string     `json:"apiKeyId,omitempty"`
	ServiceID string     `json:"serviceId,omitempty"`

	Request  *Request  `json:"request,omitempty"`
	Response *Response `json:"response,omitempty"`
}

// BuildProxyLogJSON สร้าง structured log JSON จาก ProxyLogFields
// ใช้ publish ลง stream:usage-log ให้ control-plane เก็บลง MongoDB
func BuildProxyLogJSON(f ProxyLogFields) ([]byte, error) {
	now := time.Now().UTC()
	statusCode := f.StatusCode
	respDuration := f.TotalMs

	level := "INFO"
	if f.StatusCode >= 500 {
		level = "ERROR"
	}

	entry := &Log{
		Time:      &now,
		Level:     level,
		Message:   "proxy",
		Hostname:  hostname,
		Service:   "oryca-gateway",
		TraceID:   f.TraceID,
		UserID:    f.UserID,
		ApiKeyID:  f.ApiKeyID,
		ServiceID: f.ServiceID,
		Request: &Request{
			Host:   f.Host,
			IP:     f.ClientIP,
			Method: f.Method,
			Path:   f.Path,
		},
		Response: &Response{
			StatusCode: &statusCode,
			Duration:   &respDuration,
			Size:       &f.ResponseSize,
		},
	}
	return json.Marshal(entry)
}

type Request struct {
	Host   string `json:"host,omitempty"`
	IP     string `json:"ip,omitempty"`
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
}

type Response struct {
	StatusCode *int   `json:"statusCode,omitempty"`
	Size       *int64 `json:"size,omitempty"`
	Duration   *int64 `json:"duration,omitempty"`
}

// ProxyLogFields ข้อมูลสำหรับ structured log ของ proxy request
type ProxyLogFields struct {
	TraceID        string
	UserID         string
	ApiKeyID       string
	ServiceID      string
	PackageID      string
	ClientIP       string
	Host           string
	Method         string
	Path           string
	StatusCode     int
	ResponseSize   int64
	TotalMs        int64
	UpstreamURL    string
	UpstreamStatus int
	UpstreamMs     int64
	CacheStatus    string // "HIT" | "MISS" | "BYPASS"

	// step-level timing breakdown ฝั่ง gateway เอง (ms). ไม่รวม UpstreamMs
	// ไม่ถูก serialize ลง log แต่เก็บไว้ให้ profile ตอน debug ผ่าน console ได้
	AuthMs         int64
	RateLimitMs    int64
	CacheCheckMs   int64
	SingleflightMs int64
	BodyReadMs     int64
	PostUpstreamMs int64
}
