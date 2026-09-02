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
	Host        string // the client request's real host — Go moves it out of the header map onto req.Host
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

// MaxErrorBodyBytes caps how much of an error body (5xx/408/429) is buffered into RAM. A huge
// error page is truncated
const MaxErrorBodyBytes = 64 << 10 // 64KB

var hopByHopHeaders = []string{
	"Connection", "Transfer-Encoding", "TE", "Trailer",
	"Upgrade", "Keep-Alive", "Proxy-Authorization", "Proxy-Authenticate",
}

var reSourceURLParam = regexp.MustCompile(`\{([^}]+)\}`)

// resolveSourcePath fills the {param} placeholders in the source URL's path from the trie match,
// then appends the remaining path (the suffix wildcard). The trailing slash is trimmed only when
// there is a remaining path, purely to avoid a double slash; with no remaining path the source
// URL's trailing slash has to stay, because some upstreams (Kong, for one) route on it and answer
// 404 without it.
func resolveSourcePath(sourcePath string, pathParams map[string]string, remaining string) string {
	resolvedPath := sourcePath
	for name, val := range pathParams {
		resolvedPath = strings.ReplaceAll(resolvedPath, "{"+name+"}", val)
	}
	// any {param} still left is filled from pathParams in order, as a fallback
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

// mergeUpstreamQuery joins the source URL's query string (first) with the client's (second). The
// client's auth params are stripped, because the upstream only ever uses the source URL's auth.
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

// BuildUpstreamURL builds the target URL from the source URL, the path params and the remaining
// path. The source URL may carry a query string of its own (?api_key=xxx), which has to be split
// off before the path is appended.
func BuildUpstreamURL(source *model.Source, resource *model.ResourcePath, pathParams map[string]string, remaining string, rawQuery string) string {
	parsed, err := url.Parse(source.URL)
	if err != nil {
		// fallback: use the source URL as it is
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

		// copy the client's headers first (hop-by-hop and auth headers are stripped)
		for k, vv := range req.Headers {
			upReq.Header[k] = vv
		}
		for _, h := range hopByHopHeaders {
			upReq.Header.Del(h)
		}
		upReq.Header.Del("Authorization")
		upReq.Header.Del("X-Api-Key")
		upReq.Header.Del("Accept-Encoding")

		// then the source's own auth (assigned directly, so the case is not normalised)
		for _, kv := range req.Source.Headers {
			upReq.Header[kv.Key] = []string{kv.Value}
		}

		// inject forwarding headers
		upReq.Header["X-Forwarded-For"] = append(upReq.Header["X-Forwarded-For"], clientIP)
		upReq.Header["X-Forwarded-Host"] = []string{req.Host}
		upReq.Header["X-Real-Ip"] = []string{clientIP}
		upReq.Header["X-Request-Id"] = []string{requestID}

		// stop Go sending the wrong Host header
		upReq.Host = ""

		upResp, err := s.client.Do(upReq)
		if err != nil {
			return err
		}

		// counts as a failure: 5xx, 408, 429. The body is buffered up to a ceiling so the connection
		// goes back to the pool, then passed through unchanged (RFC 9110. A gateway does not rewrite
		// the status)
		if breaker.IsUpstreamFailure(upResp.StatusCode) {
			body, _ := io.ReadAll(io.LimitReader(upResp.Body, MaxErrorBodyBytes))
			upResp.Body.Close()
			// the body may have been truncated. Content-Length has to match the bytes actually sent, or
			// the client waits forever
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

	// upstream error. Resp still holds something worth passing through
	if _, ok := err.(*breaker.UpstreamError); ok {
		return resp, err
	}

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// stripHopByHop removes headers that must not reach the client response
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
	// also strip whatever the Connection header names
	if conn := h.Get("Connection"); conn != "" {
		for _, f := range strings.Split(conn, ",") {
			h.Del(strings.TrimSpace(f))
		}
	}
}

// upstreamInternalHeaders are headers that give away the upstream proxy or server's internals.
// They should not reach the client: they are its implementation detail, not the gateway's.
var upstreamInternalHeaders = []string{"Server", "Via", "Age"}

// StripUpstreamInternalHeaders removes what the client has no business seeing:
// - Server/Via: which infrastructure is upstream (nginx, kong)
// - Vary: the gateway sets the CORS Vary itself through Echo middleware, so this avoids a duplicate
// - Age: the gateway computes and sets it after the strip
// - X-Kong-*: Kong's internal headers, of no use to a client
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

// stripAuthParams removes the auth query params that must not reach the upstream
func stripAuthParams(rawQuery string) string {
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	q.Del("api_key")
	q.Del("token")
	return q.Encode()
}

// ResolvedHost returns the host from X-Forwarded-Host, or from the Host header
func ResolvedHost(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		return h
	}
	return r.Host
}

// resolveForwardedProto works out the scheme from the request
func ResolvedProto(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// parseUpstreamHost returns the upstream URL's host, for the circuit breaker
func ParseUpstreamHost(upstreamURL string) string {
	u, err := url.Parse(upstreamURL)
	if err != nil {
		return upstreamURL
	}
	return u.Host
}
