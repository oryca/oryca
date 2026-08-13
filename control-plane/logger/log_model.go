package logger

import "time"

type Log struct {
	Time     *time.Time `json:"time"`
	Level    string     `json:"level"`
	Message  string     `json:"message"`
	Hostname string     `json:"hostname,omitempty"`
	Service  string     `json:"service,omitempty"`
	TraceID  string     `json:"traceId,omitempty"`
	UserID   string     `json:"userId,omitempty"`
	ApiKeyID string     `json:"apiKeyId,omitempty"`

	Request  *Request    `json:"request,omitempty"`
	Response *Response   `json:"response,omitempty"`
	Target   *TargetInfo `json:"target,omitempty"`
}

type Request struct {
	Host    string            `json:"host,omitempty"`
	IP      string            `json:"ip,omitempty"`
	Method  string            `json:"method,omitempty"`
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Query   map[string]string `json:"query,omitempty"`
}

type Response struct {
	StatusCode *int   `json:"statusCode,omitempty"`
	Size       *int64 `json:"size,omitempty"`
	Duration   *int64 `json:"duration,omitempty"`
}

type TargetInfo struct {
	Request  *TargetRequest  `json:"request,omitempty"`
	Response *TargetResponse `json:"response,omitempty"`
}

type TargetRequest struct {
	URL    string `json:"url,omitempty"`
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
	ClientIP       string
	Host           string
	Method         string
	Path           string
	Query          map[string]string
	Headers        map[string]string
	StatusCode     int
	ResponseSize   int64
	TotalMs        int64
	UpstreamURL    string
	UpstreamStatus int
	UpstreamMs     int64
}
