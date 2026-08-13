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

// copyUpstreamHeader คัดลอก header จาก upstream เข้า response — X-Content-Type-Options กับ
// Referrer-Policy เป็น singleton header ที่ SecureWithConfig middleware เซ็ต default ไว้ก่อน
// handler นี้ทำงานเสมอ ถ้า upstream ส่งค่าของตัวเองมาด้วยต้อง Set() ทับ (ไม่ใช่ Add()) ไม่งั้น
// client จะได้ header ซ้ำสองบรรทัด/สองค่าขัดกันในบรรทัดเดียว ผิด HTTP spec — ให้ค่าที่ upstream
// ตั้งใจกำหนดเองชนะเสมอ (เช่น Referrer-Policy: no-referrer ของ service ที่เข้มกว่า default)
func copyUpstreamHeader(dst http.Header, k, v string) {
	if strings.EqualFold(k, echo.HeaderXContentTypeOptions) || strings.EqualFold(k, echo.HeaderReferrerPolicy) {
		dst.Set(k, v)
		return
	}
	dst.Add(k, v)
}

// RateLimiter เช็ค sliding window ทุก tier — memberHint ใช้เป็น member ใน Redis zset กัน request พร้อมกันชนกัน
type RateLimiter interface {
	Allow(ctx context.Context, userID, packageID, serviceID, resourcePath, memberHint string, tiers []cache.RateLimitTier) (allowed bool, limit int, remaining int, retryAfterSec int, resetSec int, err error)
}

// bounded pool สำหรับ publish log/usage หลังส่ง response — กัน retry-sleep ปน TotalMs และกัน goroutine พุ่งตอน Redis ช้า
const (
	asyncPublishWorkers   = 8
	asyncPublishQueueSize = 2000
)

type Handler struct {
	hybridTrie    *trie.HybridTrie
	proxySvc      *service.ProxyService
	authSvc       *service.AuthService
	publisher     event.Publisher // port สำหรับส่ง access log
	rateLimiter   RateLimiter     // nil = rate limiting disabled
	publicURL     string
	upstreamCache *cache.UpstreamCache // nil = upstream caching disabled
	sf            singleflight.Group

	// asyncPublishCh — queue งาน publish ที่ห้าม block request path
	asyncPublishCh chan func()
	// pendingPublish นับงานค้างคิว — shutdown ต้อง drain ก่อนปิด Redis ไม่งั้น event หายเงียบ
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

// runPublishJob recovers a panic per-job instead of per-worker — one bad job must not
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

// enqueueAsyncPublish ส่งงานเข้า queue แบบ non-blocking — queue เต็มให้ drop + log แทน block request
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

// DrainAsyncPublish รองาน publish ค้างคิวจนจบหรือ ctx timeout — เรียกตอน shutdown หลังหยุดรับ request ก่อนปิด Redis
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

// WithRateLimiter injects a RateLimiter into the Handler (optional — nil disables rate limiting)
func (h *Handler) WithRateLimiter(rl RateLimiter) *Handler {
	h.rateLimiter = rl
	return h
}

// WithUpstreamCache เปิดใช้ response cache สำหรับ upstream proxy path
func (h *Handler) WithUpstreamCache(uc *cache.UpstreamCache) *Handler {
	h.upstreamCache = uc
	return h
}

func (h *Handler) Proxy(c echo.Context) error {
	req := c.Request()
	handlerStart := timeNow()

	// log fields — เติมระหว่างทาง flush ใน defer
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
		cacheStatus    = "BYPASS" // BYPASS=ไม่เข้าเงื่อนไข cache, MISS=เข้าเงื่อนไขแต่ไม่ hit, HIT=ตอบจาก cache

		// step-level timing — ไล่ว่า duration หลังหัก upstream ไปอยู่ step ไหน
		authMs         int64
		rateLimitMs    int64
		cacheCheckMs   int64
		singleflightMs int64
		bodyReadMs     int64
		postUpstreamMs int64
	)

	clientIP := c.RealIP()

	// defer รับประกันว่าทุก request ถูก log ไม่ว่าจะออกทาง return ไหน
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

		// publish ลง stream:usage-log — async เพราะ retry-sleep ห้ามปนกับ TotalMs ที่คำนวณไปแล้ว
		if logJSON, err := logger.BuildProxyLogJSON(logFields); err == nil {
			h.enqueueAsyncPublish(func() {
				h.publishWithRetry("publish usage-log failed", func(ctx context.Context) error {
					return h.publisher.PublishLog(ctx, logJSON)
				})
			})
		}
	}()

	// Step 1: Trie lookup — strip /gateway/api/resources prefix
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

	// Step 2: Service validation
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

	// Step 3-6: Auth + user validation (ข้ามถ้า service เป็น public)
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

	// Request ID — ต้องมีก่อน rate limit เพื่อใช้เป็น unique member ใน sliding window
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

	// Step 6.5: Rate limit check — authenticated users use packageID tier; public services use "public" tier keyed by IP
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
				// fail-open: ถ้า Redis error ยังให้ผ่านเพื่อไม่กระทบ availability
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

	// Step 7: Source resolution
	source, ok := h.hybridTrie.GetSource(resource.SourceAlias)
	if !ok || source == nil {
		statusCode = http.StatusServiceUnavailable
		return c.JSON(statusCode, model.Exception{
			Code:   tool.CodeSourceNotConfigured,
			Status: statusCode,
			Detail: "Source not configured",
		})
	}

	// Static source — return body จาก config โดยตรง ไม่ proxy
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

	// Step 7.5: Upstream cache check (GET/HEAD only) — cacheKey ใช้ต่อที่ singleflight Step 8
	cd := parseCacheDirective(req.Header.Get(headerCacheControl))
	var cacheKey string
	// Range request ต้อง bypass cache+singleflight — key ไม่รวม Range จะ coalesce ผิดช่วง (เคส PMTiles)
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
				// client max-age directive: ถ้า cache เก่าเกินที่ client ยอมรับ ให้ miss
				if cd.maxAge < 0 || entryAge <= cd.maxAge {
					// remaining freshness เหลือเท่าไหร่ — RFC 9111 §5.1 — ใช้ทั้งกิ่ง 304 และกิ่งปกติ
					remaining := entry.TTLSec - entryAge
					if remaining < 0 {
						remaining = 0
					}

					// 304 ตอบก่อน — client ไม่ได้รับข้อมูลจริงสักไบต์เดียว (RFC 9110 §15.4.5)
					if req.Header.Get(headerIfNoneMatch) != "" && req.Header.Get(headerIfNoneMatch) == entry.ETag {
						statusCode = http.StatusNotModified
						upstreamStatus = entry.StatusCode
						cacheStatus = "HIT"
						c.Response().Header().Set(headerRequestID, requestID)
						c.Response().Header().Set(headerXCache, "HIT")
						c.Response().Header().Set("Age", strconv.Itoa(entryAge))
						c.Response().Header().Set(headerCacheControl, publicCacheControl(remaining))
						if entry.ETag != "" {
							c.Response().Header().Set("ETag", entry.ETag)
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
					if entry.ETag != "" {
						c.Response().Header().Set("ETag", entry.ETag)
					}

					hitBody := entry.Body
					if entry.StatusCode >= 200 && entry.StatusCode < 300 && transformableCT(http.Header(entry.Headers).Get("Content-Type")) {
						if transformedBody, transformHeaders := h.applyTransform(entry.Body, req, svc.ID, svc.BasePath, resource.Path, req.Method); transformedBody != nil {
							for k, v := range transformHeaders {
								c.Response().Header().Set(k, v)
							}
							c.Response().Header().Del("Content-Length")
							hitBody = transformedBody
						}
					}

					// gzip ตอนเขียนเท่านั้น — cache เก็บดิบเสมอ; responseSize = bytes ก่อนบีบ
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

	// Step 8: Proxy via circuit breaker — singleflight coalesce ได้เฉพาะ cacheKey != "" (GET/HEAD)
	// lookup transform config ครั้งเดียว ใช้ทั้งตัดสิน buffer mode และตอน apply
	tcfg := h.hybridTrie.FindTransformConfig(svc.ID, resource.Path, req.Method)

	proxyCallStart := timeNow()
	var ur upstreamResult
	if cacheKey != "" {
		v, _, _ := h.sf.Do(cacheKey, func() (any, error) {
			return h.fetchFromUpstream(req, upstreamURL, clientIP, requestID, source, maxBufferedUpstreamBytes), nil
		})
		ur = v.(upstreamResult)
		// stream ใช้ได้คนเดียว — follower ที่จองไม่ทันต้อง fetch ใหม่แบบ stream ตรง (เคส body ใหญ่ ซึ่งเกิดน้อย)
		if ur.stream != nil && !ur.claimStream() {
			ur = h.fetchFromUpstream(req, upstreamURL, clientIP, requestID, source, 0)
			if ur.stream != nil {
				ur.claimStream()
			}
		}
	} else if tcfg != nil {
		// มี transform ต้องใช้ทั้ง body — buffer แบบมีเพดาน
		ur = h.fetchFromUpstream(req, upstreamURL, clientIP, requestID, source, maxBufferedUpstreamBytes)
		ur.claimStream()
	} else {
		// ไม่ cache ไม่ transform — stream ตรง memory คงที่ไม่ขึ้นกับขนาด response
		ur = h.fetchFromUpstream(req, upstreamURL, clientIP, requestID, source, 0)
		ur.claimStream()
	}
	upstreamMs = ur.ms
	bodyReadMs = ur.readMs
	// singleflightMs = เวลา step นี้หัก upstreamMs/bodyReadMs — เหลือ overhead sf.Do/CB หรือ follower รอ leader
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

	// Step 9: Process and write response — postUpstreamMs ครอบ transform + cache.Set จนก่อน WriteHeader
	postUpstreamStart := timeNow()
	statusCode = ur.statusCode
	upstreamStatus = ur.statusCode

	// ur.headers แชร์ผ่าน singleflight — ห้าม mutate (strip ทำแล้วใน fetchFromUpstream)
	for k, vv := range ur.headers {
		for _, v := range vv {
			copyUpstreamHeader(c.Response().Header(), k, v)
		}
	}
	c.Response().Header().Set(headerRequestID, requestID)
	c.Response().Header().Set("X-Upstream-Latency-Ms", strconv.FormatInt(upstreamMs, 10))
	c.Response().Header().Set(headerXCache, "MISS")

	// transform เฉพาะ body ที่ buffer ครบและเป็น text — stream mode/binary ข้ามเสมอ
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
				// body size changed after transform — drop upstream Content-Length so nginx doesn't see mismatch
				c.Response().Header().Del("Content-Length")
				rawBody = result.Body
			} else if err != nil {
				logger.Error("transform apply failed: " + err.Error())
			}
		}
	}

	// store ลง cache — stream mode หรือ body เกินเพดานไม่เก็บ (ก้อนใหญ่ทำ Redis บล็อกทั้งระบบ)
	// varyBlocksCache: upstream ทำ content negotiation จริง (Vary อื่นนอกจาก Accept-Encoding)
	// ไม่เก็บเลยแทนที่จะเสี่ยง serve variant ผิดให้ client อื่น
	if ur.stream == nil && cacheKey != "" && !cd.noStore && !ur.varyBlocksCache && cache.IsCacheableSize(len(ur.body)) && h.upstreamCache.IsCacheableResponse(ur.statusCode, ur.headers) {
		ttl := h.upstreamCache.ExtractTTL(ur.headers)
		// upstream ไม่ส่ง ETag → สร้างเองจาก body เพื่อให้ browser ใช้ If-None-Match/304 ได้ทุก service
		etag := ur.headers.Get("ETag")
		if etag == "" {
			etag = cache.SyntheticETag(ur.body)
			c.Response().Header().Set("ETag", etag)
		}
		entry := &cache.UpstreamCacheEntry{
			StatusCode: ur.statusCode,
			Headers:    map[string][]string(ur.headers),
			Body:       ur.body,
			ETag:       etag,
		}
		if setErr := h.upstreamCache.Set(req.Context(), cacheKey, entry, ttl); setErr != nil {
			logger.Info("upstream cache set failed: " + setErr.Error())
		} else {
			// store สำเร็จ — บอก client ว่า entry นี้ fresh จาก origin และ cache ได้อีกนานเท่าไหร่
			c.Response().Header().Set("Age", "0")
			if c.Response().Header().Get(headerCacheControl) == "" {
				c.Response().Header().Set(headerCacheControl, publicCacheControl(int(ttl.Seconds())))
			}
		}
	}
	postUpstreamMs = timeNow().Sub(postUpstreamStart).Milliseconds()

	// gzip เฉพาะ text CT + client รองรับ — n ที่ได้กลับเป็น bytes ก่อนบีบเสมอ (log ใช้ค่านี้)
	bodyLen := int64(len(rawBody))
	if ur.stream != nil {
		bodyLen = -1 // stream mode ไม่รู้ขนาดล่วงหน้า
	}
	useGzip := shouldGzipResponse(req, c.Response().Header(), ur.statusCode, bodyLen)
	if useGzip {
		prepareGzipHeaders(c.Response().Header())
	}
	c.Response().WriteHeader(ur.statusCode)
	var n int64
	switch {
	case ur.stream != nil:
		// เขียน prefix ที่ buffer ไว้ (ถ้ามี) แล้ว stream ส่วนที่เหลือ — memory คงที่
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

// cacheDirective เก็บ directive จาก Cache-Control header ของ request
type cacheDirective struct {
	noCache bool
	noStore bool
	maxAge  int // -1 = ไม่ได้ระบุ
}

// parseCacheDirective แปลง Cache-Control request header เป็น cacheDirective
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

// publicCacheControl สร้าง Cache-Control header สำหรับ shared cache (upstream path)
func publicCacheControl(sec int) string {
	return "public, " + cacheMaxAgeDirective + strconv.Itoa(sec)
}

// resolvedOrigin คืน scheme+host สำหรับ {{oryca_gateway_url}} — ใช้ publicURL จาก config ก่อน
// เพราะ X-Forwarded-Proto จาก LB อาจตั้งไม่ครบ, ไม่มีค่อยเดาจาก request
func (h *Handler) resolvedOrigin(req *http.Request) string {
	if h.publicURL != "" {
		return strings.TrimRight(h.publicURL, "/")
	}
	return service.ResolvedProto(req) + "://" + service.ResolvedHost(req)
}

// applyTransform รัน transform engine กับ rawBody — คืน nil ถ้าไม่มี config หรือไม่ match
func (h *Handler) applyTransform(rawBody []byte, req *http.Request, svcID, basePath, resourcePath, method string) (body []byte, headers map[string]string) {
	tcfg := h.hybridTrie.FindTransformConfig(svcID, resourcePath, method)
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

// maxBufferedUpstreamBytes เพดาน buffer ใน RAM เพื่อ cache/transform — เกินนี้ stream ตรงถึง client แทน
const maxBufferedUpstreamBytes = 32 << 20 // 32MB

type upstreamResult struct {
	statusCode int
	headers    http.Header
	body       []byte // body ทั้งก้อน หรือ prefix เมื่อ stream != nil
	// stream != nil — ห้าม cache/transform และใช้ได้คนเดียว (claimStream ก่อนเสมอ)
	stream  io.ReadCloser
	claimed *int32
	ms      int64
	readMs  int64 // เวลาอ่าน body เข้า buffer — แยกจาก ms ที่นับถึงแค่ได้ headers
	err     error
	// varyBlocksCache = upstream ส่ง Vary ที่ไม่ใช่แค่ Accept-Encoding มา (เช่น Accept,
	// Accept-Language) ต้องเช็คจาก header ดิบก่อน strip เพราะ headers ด้านบนถูก
	// StripUpstreamInternalHeaders ลบ Vary ทิ้งไปแล้วเสมอ (กัน duplicate กับ CORS's Vary: Origin)
	varyBlocksCache bool
}

// claimStream จอง stream แบบ atomic — singleflight แชร์ result แต่ stream อ่านได้ครั้งเดียว
func (r *upstreamResult) claimStream() bool {
	return r.claimed != nil && atomicCASInt32(r.claimed)
}

func atomicCASInt32(p *int32) bool {
	return atomic.CompareAndSwapInt32(p, 0, 1)
}

// varyBlocksCache ตรวจว่า Vary header จาก upstream มีค่าอื่นนอกจาก Accept-Encoding ไหม —
// Accept-Encoding ไม่กระทบเพราะ gateway gzip เองต่อ request อยู่แล้ว (cache เก็บ body ดิบเสมอ)
// ส่วน Vary อื่น (เช่น Accept, Accept-Language) แปลว่า upstream ทำ content negotiation จริง —
// cache key ของเราไม่ได้แยกตาม request header เลย ถ้ายัง cache ต่อไปจะ serve variant ผิดให้
// client คนอื่นที่ส่ง header ต่างกัน จึงต้องกัน (ไม่ cache) แทนที่จะเสี่ยง
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

// fetchFromUpstream ยิง upstream — maxBuffer > 0 buffer เข้า memory (เกิน limit คืน prefix + stream), <= 0 stream ล้วน
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
		// UpstreamError = upstream ตอบ error status จริง — pass-through ต่อ (CB นับ failure แล้วใน Do)
		var ue *breaker.UpstreamError
		if !errors.As(err, &ue) || proxyResp == nil {
			return upstreamResult{err: err, ms: ms}
		}
	}
	if proxyResp == nil {
		return upstreamResult{ms: ms}
	}

	// เช็คจาก Vary ดิบก่อน strip เสมอ — StripUpstreamInternalHeaders (ด้านล่าง) ลบ Vary ทิ้ง
	// ทุกครั้งกันชนกับ CORS's Vary: Origin ถ้าไม่เช็คตอนนี้จะไม่มีทางรู้ทีหลังเลยว่า upstream
	// เคยทำ content negotiation จริงไหม
	hasNonEncodingVary := varyBlocksCache(proxyResp.Headers.Get("Vary"))

	// strip ครั้งเดียวก่อน share ผ่าน singleflight — หลังจากนี้ห้าม mutate headers
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
		buf.Grow(32 * 1024) // chunked/ไม่รู้ขนาด — จอง 32KB กัน realloc ช่วงแรก
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
		// body ใหญ่เกิน buffer limit — คืน prefix + stream ส่วนที่เหลือ (ไม่ปิด body)
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

// writeStreamedBody เขียน prefix แล้ว stream ส่วนที่เหลือ — คืน bytes ก่อนบีบ (log ใช้ค่านี้), ปิด stream ให้เสมอ
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

// publishWithRetry รัน publishFn พร้อม retry สูงสุด 3 ครั้ง (backoff 0, 500ms, 1s) — เดิมโค้ดนี้
// ถูกคัดลอกซ้ำ 3 จุด (access log + proxy/cache-HIT usage event) รวมไว้ที่เดียวเพื่อไม่ให้
// retry policy เพี้ยนกันระหว่างจุด — logPrefix ต้องคงข้อความเดิมของแต่ละจุดไว้เป๊ะ เผื่อมี
// log-based alert/dashboard ที่ grep string นั้นอยู่แล้ว
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
