package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"github.com/oryca/oryca/gateway/cache"
	"github.com/oryca/oryca/gateway/event"
	"io"
	"net/http"

	"github.com/oryca/oryca/gateway/breaker"
	"github.com/oryca/oryca/gateway/logger"
	"github.com/oryca/oryca/gateway/model"
	"github.com/oryca/oryca/gateway/service"
	gwsync "github.com/oryca/oryca/gateway/sync"
	"github.com/oryca/oryca/gateway/tool"
	"github.com/oryca/oryca/gateway/transform"
	transformmodel "github.com/oryca/oryca/gateway/transform/model"
	"github.com/oryca/oryca/gateway/trie"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"golang.org/x/sync/singleflight"
)

var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024) // 32KB
		return &b
	},
}

const (
	headerRequestID    = "X-Request-Id"
	headerCacheControl = "Cache-Control"
	headerXCache       = "X-Cache"
	headerIfNoneMatch  = "If-None-Match"
	resourcesBasePath  = "/gateway/api/resources"
)

// copyUpstreamHeader copies upstream headers into the response. X-Content-Type-Options and
// Referrer-Policy are singleton headers, and the SecureWithConfig middleware has set a default for
// both by the time this runs. If the upstream sends its own value we have to Set() over it, not
// Add(), or the client gets the header twice, or two conflicting values on one line, against the
// HTTP spec. Whatever the upstream deliberately chose wins (a service asking for a stricter
// Referrer-Policy: no-referrer, for example).
func copyUpstreamHeader(dst http.Header, k, v string) {
	if strings.EqualFold(k, echo.HeaderXContentTypeOptions) || strings.EqualFold(k, echo.HeaderReferrerPolicy) {
		dst.Set(k, v)
		return
	}
	dst.Add(k, v)
}

// RateLimiter checks the sliding window for every tier. memberHint becomes the member in the Redis
// zset, so simultaneous requests do not overwrite each other.
type RateLimiter interface {
	Allow(ctx context.Context, userID, packageID, serviceID, resourcePath, memberHint string, tiers []cache.RateLimitTier) (allowed bool, limit int, remaining int, retryAfterSec int, resetSec int, err error)
}

// bounded pool for publishing log/usage after the response is sent. Keeps retry-sleep out of
// TotalMs, and stops goroutines piling up when Redis is slow.
const (
	asyncPublishWorkers   = 8
	asyncPublishQueueSize = 2000
)

type Handler struct {
	hybridTrie    *trie.HybridTrie
	proxySvc      *service.ProxyService
	authSvc       *service.AuthService
	publisher     event.Publisher // port for sending access logs
	rateLimiter   RateLimiter     // nil = rate limiting disabled
	publicURL     string
	upstreamCache *cache.UpstreamCache // nil = upstream caching disabled
	sf            singleflight.Group

	// asyncPublishCh. Queue of publish work that must never block the request path
	asyncPublishCh chan func()
	// pendingPublish counts queued work. Shutdown must drain it before closing Redis, or events vanish
	pendingPublish sync.WaitGroup
}

func New(ht *trie.HybridTrie, proxySvc *service.ProxyService, authSvc *service.AuthService, pub event.Publisher, publicURL string) *Handler {
	h := &Handler{
		hybridTrie:     ht,
		proxySvc:       proxySvc,
		authSvc:        authSvc,
		publisher:      pub,
		publicURL:      publicURL,
		asyncPublishCh: make(chan func(), asyncPublishQueueSize),
	}
	for i := 0; i < asyncPublishWorkers; i++ {
		go h.runAsyncPublishWorker()
	}
	return h
}

func (h *Handler) runAsyncPublishWorker() {
	for job := range h.asyncPublishCh {
		runPublishJob(job)
	}
}

// runPublishJob recovers a panic per-job instead of per-worker. One bad job must not
// kill the whole worker (and, unrecovered, the whole process) or leave pendingPublish
// permanently off by one.
func runPublishJob(job func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error(fmt.Sprintf("async publish job panicked: %v", r))
		}
	}()
	job()
}

// enqueueAsyncPublish queues work without blocking. A full queue drops and logs instead of holding the request
func (h *Handler) enqueueAsyncPublish(job func()) {
	h.pendingPublish.Add(1)
	wrapped := func() {
		defer h.pendingPublish.Done()
		job()
	}
	select {
	case h.asyncPublishCh <- wrapped:
	default:
		h.pendingPublish.Done()
		logger.Error("async publish queue full — dropping publish job")
	}
}

// DrainAsyncPublish waits for queued publishes to finish, or for ctx to expire. Call it at
// shutdown, after the server stops accepting requests and before Redis closes.
func (h *Handler) DrainAsyncPublish(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		h.pendingPublish.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		logger.Error("async publish drain timed out — some log/usage events may be dropped on shutdown")
	}
}

// WithRateLimiter injects a RateLimiter into the Handler (optional. Nil disables rate limiting)
func (h *Handler) WithRateLimiter(rl RateLimiter) *Handler {
	h.rateLimiter = rl
	return h
}

// WithUpstreamCache enables the response cache on the upstream proxy path
func (h *Handler) WithUpstreamCache(uc *cache.UpstreamCache) *Handler {
	h.upstreamCache = uc
	return h
}

func (h *Handler) Proxy(c echo.Context) error {
	req := c.Request()
	handlerStart := timeNow()

	// log fields. Filled in along the way, flushed in the defer
	var (
		requestID      string
		userID         string
		apiKeyID       string
		serviceID      string
		packageID      string
		upstreamURL    string
		statusCode     int
		responseSize   int64
		upstreamStatus int
		upstreamMs     int64
		cacheStatus    = "BYPASS" // BYPASS=not eligible for cache, MISS=eligible but no hit, HIT=served from cache

		// step-level timing. Shows where the duration went once upstream is subtracted
		authMs         int64
		rateLimitMs    int64
		cacheCheckMs   int64
		singleflightMs int64
		bodyReadMs     int64
		postUpstreamMs int64
	)

	clientIP := c.RealIP()

	// the defer guarantees every request is logged, whichever return it leaves by
	defer func() {
		logFields := logger.ProxyLogFields{
			TraceID:        requestID,
			UserID:         userID,
			ApiKeyID:       apiKeyID,
			ServiceID:      serviceID,
			PackageID:      packageID,
			ClientIP:       clientIP,
			Host:           req.Host,
			Method:         req.Method,
			Path:           req.URL.Path,
			StatusCode:     statusCode,
			ResponseSize:   responseSize,
			TotalMs:        timeNow().Sub(handlerStart).Milliseconds(),
			UpstreamURL:    upstreamURL,
			UpstreamStatus: upstreamStatus,
			UpstreamMs:     upstreamMs,
			CacheStatus:    cacheStatus,
			AuthMs:         authMs,
			RateLimitMs:    rateLimitMs,
			CacheCheckMs:   cacheCheckMs,
			SingleflightMs: singleflightMs,
			BodyReadMs:     bodyReadMs,
			PostUpstreamMs: postUpstreamMs,
		}
		logger.ProxyLog(logFields)

		// publish to stream:usage-log. Async, because retry-sleep must not land in the TotalMs already computed
		if logJSON, err := logger.BuildProxyLogJSON(logFields); err == nil {
			h.enqueueAsyncPublish(func() {
				h.publishWithRetry("publish usage-log failed", func(ctx context.Context) error {
					return h.publisher.PublishLog(ctx, logJSON)
				})
			})
		}
	}()

	// trie lookup. Strip /gateway/api/resources prefix
	triePath := strings.TrimPrefix(req.URL.Path, resourcesBasePath)
	if triePath == "" {
		triePath = "/"
	}
	match := h.hybridTrie.FindBestMatch(triePath)
	if match == nil {
		statusCode = http.StatusNotFound
		return c.JSON(statusCode, model.Exception{
			Code:   tool.CodeNotFound,
			Status: statusCode,
			Detail: "Route not found",
		})
	}

	svc := match.Service
	resource := match.Resource
	serviceID = svc.ID

	if !svc.Enabled {
		statusCode = http.StatusServiceUnavailable
		return c.JSON(statusCode, model.Exception{
			Code:   tool.CodeServiceDisabled,
			Status: statusCode,
			Detail: "Service is disabled",
		})
	}

	if !methodAllowed(req.Method, resource.Methods) {
		statusCode = http.StatusMethodNotAllowed
		return c.JSON(statusCode, model.Exception{
			Code:   tool.CodeMethodNotAllowed,
			Status: statusCode,
			Detail: "Method not allowed",
		})
	}

	// auth + user validation (skipped when the service is public)
	authStart := timeNow()
	var user *model.User
	if !svc.IsPublic {
		u, errResp := h.authenticate(c, svc)
		if errResp != nil {
			authMs = timeNow().Sub(authStart).Milliseconds()
			statusCode = c.Response().Status
			return nil // response already written by authenticate
		}
		user = u
	}
	authMs = timeNow().Sub(authStart).Milliseconds()

	if user != nil {
		userID = user.ID
		packageID = user.PackageID
	}
	apiKeyID, _ = c.Get("apiKeyId").(string)

	// Request ID. Needed before the rate limit, as the unique member in the sliding window
	requestID = req.Header.Get(headerRequestID)
	requestID = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, requestID)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	c.Response().Header().Set(headerRequestID, requestID)

	// rate limit check. Authenticated users use packageID tier; public services use "public" tier keyed by IP
	rateLimitStart := timeNow()
	if h.rateLimiter != nil {
		var rateLimitID, rateLimitPackage string
		if user != nil {
			rateLimitID, rateLimitPackage = user.ID, user.PackageID
		} else {
			rateLimitID, rateLimitPackage = clientIP, "public"
		}
		if tiers := toRateLimitTiers(resource.RateLimit, rateLimitPackage); len(tiers) > 0 {
			allowed, rlLimit, rlRemaining, retryAfterSec, resetSec, err := h.rateLimiter.Allow(
				req.Context(), rateLimitID, rateLimitPackage, svc.ID, resource.Path, requestID, tiers,
			)
			if err != nil {
				logger.Error("rate limit check failed: " + err.Error())
				// fail-open: on a Redis error, let it through rather than take availability down
			} else if !allowed {
				rateLimitMs = timeNow().Sub(rateLimitStart).Milliseconds()
				statusCode = http.StatusTooManyRequests
				c.Response().Header().Set("RateLimit-Limit", strconv.Itoa(rlLimit))
				c.Response().Header().Set("RateLimit-Remaining", "0")
				c.Response().Header().Set("RateLimit-Reset", strconv.Itoa(retryAfterSec))
				c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
				return c.JSON(statusCode, model.Exception{
					Code:   tool.CodeTooManyRequests,
					Status: statusCode,
					Detail: "Rate limit exceeded",
				})
			} else {
				c.Response().Header().Set("RateLimit-Limit", strconv.Itoa(rlLimit))
				c.Response().Header().Set("RateLimit-Remaining", strconv.Itoa(rlRemaining))
				c.Response().Header().Set("RateLimit-Reset", strconv.Itoa(resetSec))
			}
		}
	}
	rateLimitMs = timeNow().Sub(rateLimitStart).Milliseconds()

	source, ok := h.hybridTrie.GetSource(resource.SourceAlias)
	if !ok || source == nil {
		statusCode = http.StatusServiceUnavailable
		return c.JSON(statusCode, model.Exception{
			Code:   tool.CodeSourceNotConfigured,
			Status: statusCode,
			Detail: "Source not configured",
		})
	}

	// Static source. Return the body straight from config, no proxying
	if source.Type == "static" {
		if source.Body == "" {
			statusCode = http.StatusServiceUnavailable
			return c.JSON(statusCode, model.Exception{
				Code:   tool.CodeSourceNotConfigured,
				Status: statusCode,
				Detail: "Static source body not configured",
			})
		}
		ct := source.ContentType
		if ct == "" {
			ct = "application/json"
		}
		statusCode = http.StatusOK
		responseSize = int64(len(source.Body))
		c.Response().Header().Set(headerCacheControl, "no-store")
		return c.Blob(statusCode, ct, []byte(source.Body))
	}

	upstreamURL = service.BuildUpstreamURL(source, resource, match.PathParams, match.Remaining, req.URL.RawQuery)

	// look the transform config up once, for the ETag the client sees, the buffer-mode
	// decision, and the apply itself. It has to be read before the cache check, because a
	// 304 is decided against the client-facing ETag, which the ruleset is part of
	tcfg := h.hybridTrie.FindTransformConfig(svc.ID, resource.Path, req.Method)
	transformFingerprint := tcfg.Fingerprint()

	// upstream cache check (GET/HEAD only). CacheKey is reused by singleflight at Step 8
	cd := parseCacheDirective(req.Header.Get(headerCacheControl))
	var cacheKey string
	// Range requests bypass cache and singleflight. The key ignores Range, so it would coalesce the
	// wrong span (the PMTiles case)
	if h.upstreamCache != nil && cache.IsCacheableRequest(req.Method) && req.Header.Get("Range") == "" {
		cacheKey = cache.UpstreamCacheKey(req.Method, upstreamURL)
	}
	if cacheKey != "" && !cd.noStore {
		cacheStatus = "MISS"
		if !cd.noCache {
			cacheCheckStart := timeNow()
			entry, entryAge, hit := h.upstreamCache.Get(req.Context(), cacheKey)
			cacheCheckMs = timeNow().Sub(cacheCheckStart).Milliseconds()
			if hit {
				// client max-age directive: an entry older than the client accepts counts as a miss
				if cd.maxAge < 0 || entryAge <= cd.maxAge {
					// how much freshness is left. RFC 9111 §5.1. Used by both the 304 and the normal branch
					remaining := entry.TTLSec - entryAge
					if remaining < 0 {
						remaining = 0
					}

					// the entry holds the untransformed body, so its ETag is not what the
					// client is holding. Compare against the validator we actually handed out
					clientETag := cache.ClientETag(entry.ETag, transformFingerprint)

					// answer 304 first. The client receives not one byte of the body (RFC 9110 §15.4.5)
					if req.Header.Get(headerIfNoneMatch) != "" && req.Header.Get(headerIfNoneMatch) == clientETag {
						statusCode = http.StatusNotModified
						upstreamStatus = entry.StatusCode
						cacheStatus = "HIT"
						c.Response().Header().Set(headerRequestID, requestID)
						c.Response().Header().Set(headerXCache, "HIT")
						c.Response().Header().Set("Age", strconv.Itoa(entryAge))
						c.Response().Header().Set(headerCacheControl, publicCacheControl(remaining))
						if clientETag != "" {
							c.Response().Header().Set("ETag", clientETag)
						}
						return c.NoContent(http.StatusNotModified)
					}

					statusCode = entry.StatusCode
					upstreamStatus = entry.StatusCode
					cacheStatus = "HIT"

					service.StripHopByHop(http.Header(entry.Headers))
					service.StripCORSHeaders(http.Header(entry.Headers))
					for k, vv := range entry.Headers {
						for _, v := range vv {
							copyUpstreamHeader(c.Response().Header(), k, v)
						}
					}
					c.Response().Header().Set(headerRequestID, requestID)
					c.Response().Header().Set(headerXCache, "HIT")
					c.Response().Header().Set("Age", strconv.Itoa(entryAge))
					c.Response().Header().Set(headerCacheControl, publicCacheControl(remaining))
					if clientETag != "" {
						c.Response().Header().Set("ETag", clientETag)
					}

					hitBody := entry.Body
					if entry.StatusCode >= 200 && entry.StatusCode < 300 && transformableCT(http.Header(entry.Headers).Get("Content-Type")) {
						if transformedBody, transformHeaders := h.applyTransform(entry.Body, req, tcfg, svc.BasePath); transformedBody != nil {
							for k, v := range transformHeaders {
								c.Response().Header().Set(k, v)
							}
							c.Response().Header().Del("Content-Length")
							hitBody = transformedBody
						}
					}

					// gzip only on the way out. The cache always holds it raw; responseSize = bytes before compression
					if shouldGzipResponse(req, c.Response().Header(), entry.StatusCode, int64(len(hitBody))) {
						prepareGzipHeaders(c.Response().Header())
						c.Response().WriteHeader(entry.StatusCode)
						responseSize = writeGzip(c, hitBody)
						return nil
					}
					c.Response().WriteHeader(entry.StatusCode)
					buf := bufPool.Get().(*[]byte)
					n, _ := io.CopyBuffer(c.Response().Writer, bytes.NewReader(hitBody), *buf)
					bufPool.Put(buf)
					responseSize = n
					return nil
				}
			}
		}
	}

	// proxy via circuit breaker. Singleflight can only coalesce when cacheKey != "" (GET/HEAD)
	proxyCallStart := timeNow()
	var ur upstreamResult
	if cacheKey != "" {
		v, _, _ := h.sf.Do(cacheKey, func() (any, error) {
			return h.fetchFromUpstream(req, upstreamURL, clientIP, requestID, source, maxBufferedUpstreamBytes), nil
		})
		ur = v.(upstreamResult)
		// a stream has one reader. A follower that missed the claim refetches as a plain stream (large
		// bodies, which are rare)
		if ur.stream != nil && !ur.claimStream() {
			ur = h.fetchFromUpstream(req, upstreamURL, clientIP, requestID, source, 0)
			if ur.stream != nil {
				ur.claimStream()
			}
		}
	} else if tcfg != nil {
		// a transform needs the whole body. Buffer it, with a ceiling
		ur = h.fetchFromUpstream(req, upstreamURL, clientIP, requestID, source, maxBufferedUpstreamBytes)
		ur.claimStream()
	} else {
		// no cache and no transform. Stream straight through, memory flat whatever the response size
		ur = h.fetchFromUpstream(req, upstreamURL, clientIP, requestID, source, 0)
		ur.claimStream()
	}
	upstreamMs = ur.ms
	bodyReadMs = ur.readMs
	// singleflightMs = this step minus upstreamMs/bodyReadMs. What is left is sf.Do/CB overhead, or a
	// follower waiting on the leader
	singleflightMs = timeNow().Sub(proxyCallStart).Milliseconds() - upstreamMs - bodyReadMs
	if singleflightMs < 0 {
		singleflightMs = 0
	}

	if ur.err == breaker.ErrOpenState {
		statusCode = http.StatusServiceUnavailable
		return c.JSON(statusCode, model.Exception{
			Code:   tool.CodeCircuitOpen,
			Status: statusCode,
			Detail: "Upstream circuit open",
		})
	}
	if ur.statusCode == 0 {
		statusCode = http.StatusBadGateway
		return c.JSON(statusCode, model.Exception{
			Code:   tool.CodeBadGateway,
			Status: statusCode,
			Detail: "Upstream did not respond",
		})
	}

	// process and write response. PostUpstreamMs covers transform + cache.Set up to WriteHeader
	postUpstreamStart := timeNow()
	statusCode = ur.statusCode
	upstreamStatus = ur.statusCode

	// ur.headers is shared through singleflight. Do not mutate it (the strip happened in fetchFromUpstream)
	for k, vv := range ur.headers {
		for _, v := range vv {
			copyUpstreamHeader(c.Response().Header(), k, v)
		}
	}
	c.Response().Header().Set(headerRequestID, requestID)
	c.Response().Header().Set("X-Upstream-Latency-Ms", strconv.FormatInt(upstreamMs, 10))
	c.Response().Header().Set(headerXCache, "MISS")

	// only transform a fully buffered text body. Stream mode and binary always skip
	rawBody := ur.body
	if ur.stream == nil && ur.statusCode >= 200 && ur.statusCode < 300 {
		if tcfg != nil && transformableCT(ur.headers.Get("Content-Type")) {
			tctx := &transformmodel.TransformContext{
				BaseURL:   h.resolvedOrigin(req) + resourcesBasePath + svc.BasePath,
				AuthToken: resolveAuthToken(req),
				AuthType:  resolveAuthType(req),
			}
			eng := transform.NewHybridEngine(transform.FromPayload(tcfg), tctx)
			if result, err := eng.Apply(rawBody); err == nil && result.Found {
				for k, v := range result.Headers {
					c.Response().Header().Set(k, v)
				}
				// body size changed after transform. Drop upstream Content-Length so nginx doesn't see mismatch
				c.Response().Header().Del("Content-Length")
				rawBody = result.Body
			} else if err != nil {
				logger.Error("transform apply failed: " + err.Error())
			}
		}
	}

	// store in the cache. Stream mode, or a body over the ceiling, is not stored (one large blob
	// blocks all of Redis)
	// varyBlocksCache: the upstream really does content negotiation (a Vary beyond Accept-Encoding),
	// so store nothing rather than risk serving the wrong variant to another client
	if ur.stream == nil && cacheKey != "" && !cd.noStore && !ur.varyBlocksCache && cache.IsCacheableSize(len(ur.body)) && h.upstreamCache.IsCacheableResponse(ur.statusCode, ur.headers) {
		ttl := h.upstreamCache.ExtractTTL(ur.headers)
		// no ETag from the upstream → build one from the body, so browsers get If-None-Match/304 on every service
		etag := ur.headers.Get("ETag")
		if etag == "" {
			etag = cache.SyntheticETag(ur.body)
		}
		// the entry keeps the upstream's own validator for the untransformed body it stores,
		// while the client gets one that also covers the rules applied on the way out
		c.Response().Header().Set("ETag", cache.ClientETag(etag, transformFingerprint))
		entry := &cache.UpstreamCacheEntry{
			StatusCode: ur.statusCode,
			Headers:    map[string][]string(ur.headers),
			Body:       ur.body,
			ETag:       etag,
		}
		if setErr := h.upstreamCache.Set(req.Context(), cacheKey, entry, ttl); setErr != nil {
			logger.Info("upstream cache set failed: " + setErr.Error())
		} else {
			// stored. Tell the client this entry is fresh from origin, and how long it stays cacheable
			c.Response().Header().Set("Age", "0")
			if c.Response().Header().Get(headerCacheControl) == "" {
				c.Response().Header().Set(headerCacheControl, publicCacheControl(int(ttl.Seconds())))
			}
		}
	}
	postUpstreamMs = timeNow().Sub(postUpstreamStart).Milliseconds()

	// gzip only for text content types the client accepts. N always comes back as bytes before
	// compression (the log uses it)
	bodyLen := int64(len(rawBody))
	if ur.stream != nil {
		bodyLen = -1 // stream mode — the size is not known up front
	}
	useGzip := shouldGzipResponse(req, c.Response().Header(), ur.statusCode, bodyLen)
	if useGzip {
		prepareGzipHeaders(c.Response().Header())
	}
	c.Response().WriteHeader(ur.statusCode)
	var n int64
	switch {
	case ur.stream != nil:
		// write the buffered prefix (if any), then stream the rest. Memory stays flat
		n = writeStreamedBody(c, rawBody, ur.stream, useGzip)
	case useGzip:
		n = writeGzip(c, rawBody)
	default:
		buf := bufPool.Get().(*[]byte)
		n, _ = io.CopyBuffer(c.Response().Writer, bytes.NewReader(rawBody), *buf)
		bufPool.Put(buf)
	}
	responseSize = n

	return nil
}

// cacheDirective holds the directives from the request's Cache-Control header
type cacheDirective struct {
	noCache bool
	noStore bool
	maxAge  int // -1 = not specified
}

// parseCacheDirective turns a Cache-Control request header into a cacheDirective
func parseCacheDirective(h string) cacheDirective {
	d := cacheDirective{maxAge: -1}
	for _, part := range strings.Split(h, ",") {
		tok := strings.TrimSpace(strings.ToLower(part))
		switch {
		case tok == "no-cache":
			d.noCache = true
		case tok == "no-store":
			d.noStore = true
		case strings.HasPrefix(tok, cacheMaxAgeDirective):
			if n, err := strconv.Atoi(strings.TrimPrefix(tok, cacheMaxAgeDirective)); err == nil {
				d.maxAge = n
			}
		}
	}
	return d
}

const cacheMaxAgeDirective = "max-age="

// publicCacheControl builds the Cache-Control header for a shared cache (upstream path)
func publicCacheControl(sec int) string {
	return "public, " + cacheMaxAgeDirective + strconv.Itoa(sec)
}

// resolvedOrigin returns the scheme+host for {{oryca_gateway_url}}. It prefers publicURL from
// config, because X-Forwarded-Proto from a load balancer may not be set; without it, guess from
// the request.
func (h *Handler) resolvedOrigin(req *http.Request) string {
	if h.publicURL != "" {
		return strings.TrimRight(h.publicURL, "/")
	}
	return service.ResolvedProto(req) + "://" + service.ResolvedHost(req)
}

// applyTransform runs the transform engine over rawBody. Returns nil when there is no config, or nothing matches
func (h *Handler) applyTransform(rawBody []byte, req *http.Request, tcfg *gwsync.TransformConfigPayload, basePath string) (body []byte, headers map[string]string) {
	if tcfg == nil {
		return nil, nil
	}
	tctx := &transformmodel.TransformContext{
		BaseURL:   h.resolvedOrigin(req) + resourcesBasePath + basePath,
		AuthToken: resolveAuthToken(req),
		AuthType:  resolveAuthType(req),
	}
	eng := transform.NewHybridEngine(transform.FromPayload(tcfg), tctx)
	result, err := eng.Apply(rawBody)
	if err != nil || !result.Found {
		if err != nil {
			logger.Error("transform apply failed: " + err.Error())
		}
		return nil, nil
	}
	return result.Body, result.Headers
}

// maxBufferedUpstreamBytes is the RAM ceiling for buffering to cache or transform. Past it, stream
// straight to the client instead
const maxBufferedUpstreamBytes = 32 << 20 // 32MB

type upstreamResult struct {
	statusCode int
	headers    http.Header
	body       []byte // the whole body, or the prefix when stream != nil
	// stream != nil. Must not be cached or transformed, and has a single reader (claimStream first)
	stream  io.ReadCloser
	claimed *int32
	ms      int64
	readMs  int64 // time spent reading the body into the buffer — separate from ms, which stops at the headers
	err     error
	// varyBlocksCache = the upstream sent a Vary that is more than Accept-Encoding (Accept,
	// Accept-Language). It has to be read off the raw header before the strip, because
	// StripUpstreamInternalHeaders always removes Vary (to avoid duplicating CORS's Vary: Origin).
	varyBlocksCache bool
}

// claimStream claims the stream atomically. Singleflight shares the result, but a stream can only
// be read once
func (r *upstreamResult) claimStream() bool {
	return r.claimed != nil && atomicCASInt32(r.claimed)
}

func atomicCASInt32(p *int32) bool {
	return atomic.CompareAndSwapInt32(p, 0, 1)
}

// varyBlocksCache reports whether the upstream's Vary header names anything beyond Accept-Encoding.
// Accept-Encoding does not matter, because the gateway gzips per request itself (the cache always
// holds the body raw). Any other Vary (Accept, Accept-Language) means the upstream really does
// negotiate content, and our cache key does not vary by request header at all. Caching on would
// serve the wrong variant to a client that sent different headers, so we decline to cache.
func varyBlocksCache(varyHeader string) bool {
	if varyHeader == "" {
		return false
	}
	for _, v := range strings.Split(varyHeader, ",") {
		if !strings.EqualFold(strings.TrimSpace(v), "Accept-Encoding") {
			return true
		}
	}
	return false
}

// fetchFromUpstream calls the upstream. maxBuffer > 0 buffers into memory (over the limit it returns
// prefix + stream); <= 0 streams throughout
func (h *Handler) fetchFromUpstream(req *http.Request, upstreamURL, clientIP, requestID string, source *model.Source, maxBuffer int64) upstreamResult {
	start := timeNow()
	proxyResp, err := h.proxySvc.Do(&service.ProxyRequest{
		Ctx:         req.Context(),
		Method:      req.Method,
		UpstreamURL: upstreamURL,
		Headers:     req.Header.Clone(),
		Host:        req.Host,
		Body:        req.Body,
		Source:      source,
	}, clientIP, requestID)
	ms := timeNow().Sub(start).Milliseconds()

	if err != nil {
		// UpstreamError = the upstream really answered with an error status. Pass it through (Do already
		// counted the CB failure)
		var ue *breaker.UpstreamError
		if !errors.As(err, &ue) || proxyResp == nil {
			return upstreamResult{err: err, ms: ms}
		}
	}
	if proxyResp == nil {
		return upstreamResult{ms: ms}
	}

	// read Vary off the raw header, always before the strip. StripUpstreamInternalHeaders (below)
	// removes Vary every time so it cannot collide with CORS's Vary: Origin, and if we do not look
	// now there is no way to find out later whether the upstream negotiated content at all.
	hasNonEncodingVary := varyBlocksCache(proxyResp.Headers.Get("Vary"))

	// strip once, before sharing through singleflight. After this, headers must not be mutated
	service.StripHopByHop(proxyResp.Headers)
	service.StripCORSHeaders(proxyResp.Headers)
	service.StripUpstreamInternalHeaders(proxyResp.Headers)

	if maxBuffer <= 0 {
		return upstreamResult{
			statusCode:      proxyResp.StatusCode,
			headers:         proxyResp.Headers,
			stream:          proxyResp.Body,
			claimed:         new(int32),
			ms:              ms,
			varyBlocksCache: hasNonEncodingVary,
		}
	}

	readStart := timeNow()
	var body []byte
	var readErr error
	limitReader := io.LimitReader(proxyResp.Body, maxBuffer+1)

	var buf bytes.Buffer
	if proxyResp.ContentLength > 0 && proxyResp.ContentLength <= maxBuffer {
		buf.Grow(int(proxyResp.ContentLength))
	} else {
		buf.Grow(32 * 1024) // chunked, size unknown — reserve 32KB to avoid early reallocs
	}
	tmpBuf := bufPool.Get().(*[]byte)
	_, readErr = io.CopyBuffer(&buf, limitReader, *tmpBuf)
	bufPool.Put(tmpBuf)
	if readErr == nil {
		body = buf.Bytes()
	}
	readMs := timeNow().Sub(readStart).Milliseconds()
	if readErr != nil {
		proxyResp.Body.Close()
		return upstreamResult{err: readErr, ms: ms, readMs: readMs}
	}
	if int64(len(body)) > maxBuffer {
		// body over the buffer limit. Return the prefix and stream the rest (do not close body)
		return upstreamResult{
			statusCode:      proxyResp.StatusCode,
			headers:         proxyResp.Headers,
			body:            body,
			stream:          proxyResp.Body,
			claimed:         new(int32),
			ms:              ms,
			readMs:          readMs,
			varyBlocksCache: hasNonEncodingVary,
		}
	}
	proxyResp.Body.Close()
	return upstreamResult{
		statusCode:      proxyResp.StatusCode,
		headers:         proxyResp.Headers,
		body:            body,
		ms:              ms,
		readMs:          readMs,
		varyBlocksCache: hasNonEncodingVary,
	}
}

// writeStreamedBody writes the prefix then streams the rest. Returns bytes before compression (the
// log uses it), and always closes the stream.
func writeStreamedBody(c echo.Context, prefix []byte, rest io.ReadCloser, useGzip bool) int64 {
	defer rest.Close()

	var w io.Writer = c.Response().Writer
	var gz *gzip.Writer
	if useGzip {
		gz = gzipPool.Get().(*gzip.Writer)
		gz.Reset(c.Response().Writer)
		w = gz
	}

	var total int64
	if len(prefix) > 0 {
		n, err := w.Write(prefix)
		total += int64(n)
		if err != nil {
			if gz != nil {
				_ = gz.Close()
				gzipPool.Put(gz)
			}
			return total
		}
	}
	buf := bufPool.Get().(*[]byte)
	n, _ := io.CopyBuffer(w, rest, *buf)
	bufPool.Put(buf)
	if gz != nil {
		_ = gz.Close()
		gzipPool.Put(gz)
	}
	return total + n
}

// publishWithRetry runs publishFn with up to 3 attempts (backoff 0, 500ms, 1s). This used to be
// copied in three places (access log, plus the proxy and cache-HIT usage events); it lives here so
// the retry policy cannot drift between them. logPrefix has to keep each call site's original
// wording exactly, in case a log-based alert or dashboard already greps for that string.
func (h *Handler) publishWithRetry(logPrefix string, publishFn func(ctx context.Context) error) {
	delays := []time.Duration{0, 500 * time.Millisecond, time.Second}
	for i, d := range delays {
		if d > 0 {
			time.Sleep(d)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := publishFn(ctx)
		cancel()
		if err == nil {
			return
		}
		logger.Error(logPrefix + " (attempt " + strconv.Itoa(i+1) + "): " + err.Error())
	}
}
func methodAllowed(method string, allowed []string) bool {
	method = strings.ToUpper(method)
	for _, m := range allowed {
		if strings.ToUpper(m) == method {
			return true
		}
	}
	return false
}
