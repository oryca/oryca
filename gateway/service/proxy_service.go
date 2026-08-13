package service

import (
	"bytes"
	"context"
	"github.com/oryca/oryca/gateway/breaker"
	"github.com/oryca/oryca/gateway/model"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ProxyRequest struct {
	Ctx         context.Context
	Method      string
	UpstreamURL string
	Headers     http.Header
	Host        string // host จริงของ client request — Go ย้ายออกจาก header map ไปไว้ที่ req.Host
	Body        io.Reader
	Source      *model.Source
}

type ProxyResponse struct {
	StatusCode    int
	Headers       http.Header
	Body          io.ReadCloser
	ContentLength int64
}

type ProxyService struct {
	client *http.Client
	cb     *breaker.PerDomainBreaker
}

func NewProxyService(cfg ProxyConfig, cb *breaker.PerDomainBreaker) *ProxyService {
	transport := &http.Transport{
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		DisableCompression:    false,
		DialContext: (&net.Dialer{
			Timeout:   cfg.DialTimeout,
			KeepAlive: cfg.KeepAlive,
		}).DialContext,
	}
	return &ProxyService{
		client: &http.Client{
			Transport: transport,
			Timeout:   120 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		cb: cb,
	}
}

type ProxyConfig struct {
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	IdleConnTimeout       time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	DialTimeout           time.Duration
	KeepAlive             time.Duration
}

// MaxErrorBodyBytes เพดาน error body (5xx/408/429) ที่ buffer เข้า RAM — error page ยักษ์ถูกตัดทิ้ง
const MaxErrorBodyBytes = 64 << 10 // 64KB

var hopByHopHeaders = []string{
	"Connection", "Transfer-Encoding", "TE", "Trailer",
	"Upgrade", "Keep-Alive", "Proxy-Authorization", "Proxy-Authenticate",
}

var reSourceURLParam = regexp.MustCompile(`\{([^}]+)\}`)

// resolveSourcePath แทน {param} ใน path ของ source URL ด้วยค่าจริงจาก trie match
// แล้วต่อ remaining path (suffix wildcard) — trim trailing slash เฉพาะตอนมี remaining
// เพื่อกัน double slash เท่านั้น ถ้าไม่มี remaining ต้องคง trailing slash ของ source URL ไว้
// เพราะ upstream บางตัว (เช่น Kong) แยก route ตาม trailing slash จริง (ไม่มี slash = 404)
func resolveSourcePath(sourcePath string, pathParams map[string]string, remaining string) string {
	resolvedPath := sourcePath
	for name, val := range pathParams {
		resolvedPath = strings.ReplaceAll(resolvedPath, "{"+name+"}", val)
	}
	// แทน {param} ที่ยังเหลือด้วยค่าจาก pathParams ตาม order (fallback)
	resolvedPath = reSourceURLParam.ReplaceAllStringFunc(resolvedPath, func(s string) string {
		name := s[1 : len(s)-1]
		if v, ok := pathParams[name]; ok {
			return v
		}
		return s
	})
	if remaining == "" {
		return resolvedPath
	}
	return strings.TrimRight(resolvedPath, "/") + "/" + remaining
}

// mergeUpstreamQuery รวม query string ของ source URL (ก่อน) กับ client query (หลัง)
// strip auth params จาก client query เพราะ upstream ใช้ auth ของ source URL เท่านั้น
func mergeUpstreamQuery(sourceQ, rawQuery string) string {
	stripped := stripAuthParams(rawQuery)
	if stripped == "" {
		return sourceQ
	}
	if sourceQ == "" {
		return stripped
	}
	return sourceQ + "&" + stripped
}

// BuildUpstreamURL สร้าง URL ปลายทางจาก source URL + path params + remaining
// Source URL อาจมี query string ติดมา (เช่น ?api_key=xxx) — ต้องแยกออกก่อนต่อ path
func BuildUpstreamURL(source *model.Source, resource *model.ResourcePath, pathParams map[string]string, remaining string, rawQuery string) string {
	parsed, err := url.Parse(source.URL)
	if err != nil {
		// fallback: ใช้ source URL ตรง ๆ
		u := strings.TrimRight(source.URL, "/")
		if remaining != "" {
			u += "/" + remaining
		}
		if rawQuery != "" {
			u += "?" + rawQuery
		}
		return u
	}

	parsed.Path = resolveSourcePath(parsed.Path, pathParams, remaining)
	if rawQuery != "" {
		parsed.RawQuery = mergeUpstreamQuery(parsed.RawQuery, rawQuery)
	}

	return parsed.String()
}

func (s *ProxyService) Do(req *ProxyRequest, clientIP, requestID string) (*ProxyResponse, error) {
	var resp *ProxyResponse

	err := s.cb.Execute(req.UpstreamURL, func() error {
		upReq, err := http.NewRequestWithContext(req.Ctx, req.Method, req.UpstreamURL, req.Body)
		if err != nil {
			return err
		}

		// copy headers จาก client ก่อน (strip hop-by-hop + auth headers ที่ไม่ควร forward)
		for k, vv := range req.Headers {
			upReq.Header[k] = vv
		}
		for _, h := range hopByHopHeaders {
			upReq.Header.Del(h)
		}
		upReq.Header.Del("Authorization")
		upReq.Header.Del("X-Api-Key")
		upReq.Header.Del("Accept-Encoding")

		// inject upstream auth จาก source (ใช้ direct assignment ไม่ normalize case)
		for _, kv := range req.Source.Headers {
			upReq.Header[kv.Key] = []string{kv.Value}
		}

		// inject forwarding headers
		upReq.Header["X-Forwarded-For"] = append(upReq.Header["X-Forwarded-For"], clientIP)
		upReq.Header["X-Forwarded-Host"] = []string{req.Host}
		upReq.Header["X-Real-Ip"] = []string{clientIP}
		upReq.Header["X-Request-Id"] = []string{requestID}

		// ป้องกัน Go ส่ง Host header ผิด
		upReq.Host = ""

		upResp, err := s.client.Do(upReq)
		if err != nil {
			return err
		}

		// นับ failure: 5xx, 408, 429 — buffer body แบบมีเพดานเพื่อคืน connection ให้ pool
		// แล้ว pass-through ให้ caller ส่งต่อ client ตามจริง (RFC 9110 — gateway ไม่ rewrite status)
		if breaker.IsUpstreamFailure(upResp.StatusCode) {
			body, _ := io.ReadAll(io.LimitReader(upResp.Body, MaxErrorBodyBytes))
			upResp.Body.Close()
			// body อาจถูกตัดที่เพดาน — Content-Length ต้องตรงกับ bytes จริง ไม่งั้น client ค้างรอ
			upResp.Header.Set("Content-Length", strconv.Itoa(len(body)))
			resp = &ProxyResponse{
				StatusCode:    upResp.StatusCode,
				Headers:       upResp.Header,
				Body:          io.NopCloser(bytes.NewReader(body)),
				ContentLength: int64(len(body)),
			}
			return &breaker.UpstreamError{StatusCode: upResp.StatusCode}
		}

		resp = &ProxyResponse{
			StatusCode:    upResp.StatusCode,
			Headers:       upResp.Header,
			Body:          upResp.Body,
			ContentLength: upResp.ContentLength,
		}
		return nil
	})

	// circuit open
	if err == breaker.ErrOpenState {
		return nil, err
	}

	// upstream error — resp ยังมีข้อมูลให้ pass-through
	if _, ok := err.(*breaker.UpstreamError); ok {
		return resp, err
	}

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// stripHopByHop strip headers ก่อน copy ไปยัง client response
var corsHeaders = []string{
	"Access-Control-Allow-Origin",
	"Access-Control-Allow-Methods",
	"Access-Control-Allow-Headers",
	"Access-Control-Expose-Headers",
	"Access-Control-Allow-Credentials",
	"Access-Control-Max-Age",
}

func StripCORSHeaders(h http.Header) {
	for _, k := range corsHeaders {
		h.Del(k)
	}
}

func StripHopByHop(h http.Header) {
	for _, k := range hopByHopHeaders {
		h.Del(k)
	}
	// strip headers ที่ระบุใน Connection header
	if conn := h.Get("Connection"); conn != "" {
		for _, f := range strings.Split(conn, ",") {
			h.Del(strings.TrimSpace(f))
		}
	}
}

// upstreamInternalHeaders คือ headers ที่เปิดเผยข้อมูล internal ของ upstream proxy/server
// ไม่ควร forward ไปถึง client เพราะเป็น implementation detail ไม่ใช่ข้อมูลของ GW
var upstreamInternalHeaders = []string{"Server", "Via", "Age"}

// StripUpstreamInternalHeaders ลบ headers ที่ไม่ควรเปิดเผยให้ client เห็น:
// - Server/Via: ข้อมูล upstream infrastructure (nginx, kong)
// - Vary: GW จัดการ CORS Vary เองผ่าน Echo middleware — ลบป้องกัน duplicate
// - Age: GW คำนวณและ set เองหลังจาก strip
// - X-Kong-*: internal headers ของ Kong ที่ไม่เกี่ยวกับ client
func StripUpstreamInternalHeaders(h http.Header) {
	for _, k := range upstreamInternalHeaders {
		h.Del(k)
	}
	h.Del("Vary")
	for k := range h {
		if strings.HasPrefix(k, "X-Kong-") {
			h.Del(k)
		}
	}
}

// stripAuthParams ลบ auth query params ที่ไม่ควรส่งไปยัง upstream
func stripAuthParams(rawQuery string) string {
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	q.Del("api_key")
	q.Del("token")
	return q.Encode()
}

// ResolvedHost คืน host จาก X-Forwarded-Host หรือ Host header
func ResolvedHost(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		return h
	}
	return r.Host
}

// resolveForwardedProto ตรวจสอบ proto จาก request
func ResolvedProto(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// parseUpstreamHost คืน host จาก upstream URL สำหรับ circuit breaker
func ParseUpstreamHost(upstreamURL string) string {
	u, err := url.Parse(upstreamURL)
	if err != nil {
		return upstreamURL
	}
	return u.Host
}
