package logger

import (
	"encoding/json"
	"time"
)

type Log struct {
	Time        *time.Time `json:"time"`
	Level       string     `json:"level"`
	Message     string     `json:"message"`
	Hostname    string     `json:"hostname,omitempty"`
	Service     string     `json:"service,omitempty"`
	TraceID     string     `json:"traceId,omitempty"`
	UserID      string     `json:"userId,omitempty"`
	ApiKeyID    string     `json:"apiKeyId,omitempty"`
	ServiceID   string     `json:"serviceId,omitempty"`
	PackageID   string     `json:"packageId,omitempty"`
	CacheStatus string     `json:"cacheStatus,omitempty"` // HIT | MISS | BYPASS

	Request  *Request    `json:"request,omitempty"`
	Response *Response   `json:"response,omitempty"`
	Target   *TargetInfo `json:"target,omitempty"`
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
		Time:        &now,
		Level:       level,
		Message:     "proxy",
		Hostname:    hostname,
		Service:     "oryca-gateway",
		TraceID:     f.TraceID,
		UserID:      f.UserID,
		ApiKeyID:    f.ApiKeyID,
		ServiceID:   f.ServiceID,
		PackageID:   f.PackageID,
		CacheStatus: f.CacheStatus,
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
			Timing: &ResponseTiming{
				AuthMs:         &f.AuthMs,
				RateLimitMs:    &f.RateLimitMs,
				CacheCheckMs:   &f.CacheCheckMs,
				SingleflightMs: &f.SingleflightMs,
				BodyReadMs:     &f.BodyReadMs,
				PostUpstreamMs: &f.PostUpstreamMs,
			},
		},
	}

	// ใส่ target เฉพาะเมื่อยิงออกไปจริง cache HIT หรือ 401 จะไม่มี upstream ให้เทียบ
	if f.UpstreamStatus > 0 {
		upstreamStatus := f.UpstreamStatus
		upstreamMs := f.UpstreamMs
		entry.Target = &TargetInfo{
			Request: &TargetRequest{Method: f.Method},
			Response: &TargetResponse{
				StatusCode: &upstreamStatus,
				Duration:   &upstreamMs,
			},
		}
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
	// Timing แยกขั้นของ duration ฝั่ง gateway เอง ไม่รวมเวลาที่ upstream ใช้
	// ตอบว่าเวลาที่เหลือหลังหัก target.response.duration ไปอยู่ขั้นไหน
	Timing *ResponseTiming `json:"timing,omitempty"`
}

type ResponseTiming struct {
	AuthMs         *int64 `json:"authMs,omitempty"`
	RateLimitMs    *int64 `json:"rateLimitMs,omitempty"`
	CacheCheckMs   *int64 `json:"cacheCheckMs,omitempty"`
	SingleflightMs *int64 `json:"singleflightMs,omitempty"`
	BodyReadMs     *int64 `json:"bodyReadMs,omitempty"`
	PostUpstreamMs *int64 `json:"postUpstreamMs,omitempty"`
}

// TargetInfo คือฝั่ง upstream ใช้เทียบว่า latency มาจากต้นทางหรือจาก gateway
// ไม่มีเลยเมื่อ request ไม่ได้ยิงออกไป เช่น cache HIT หรือตายที่ auth ก่อน
type TargetInfo struct {
	Request  *TargetRequest  `json:"request,omitempty"`
	Response *TargetResponse `json:"response,omitempty"`
}

type TargetRequest struct {
	Method string `json:"method,omitempty"`
}

type TargetResponse struct {
	StatusCode *int   `json:"statusCode,omitempty"`
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
	AuthMs         int64
	RateLimitMs    int64
	CacheCheckMs   int64
	SingleflightMs int64
	BodyReadMs     int64
	PostUpstreamMs int64
}
