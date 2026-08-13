// Package app wires the gateway together and runs it.
//
// It lives here rather than in package main so that the whole server can be
// embedded in another program — a distribution that adds its own handlers, or a
// test that needs the real thing — while cmd/oryca-gateway stays a three-line
// entry point.
package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/oryca/oryca/gateway/breaker"
	"github.com/oryca/oryca/gateway/cache"
	"github.com/oryca/oryca/gateway/config"
	redisconn "github.com/oryca/oryca/gateway/connection/redis"
	"github.com/oryca/oryca/gateway/event"
	"github.com/oryca/oryca/gateway/handler"
	"github.com/oryca/oryca/gateway/logger"
	"github.com/oryca/oryca/gateway/service"
	gwsync "github.com/oryca/oryca/gateway/sync"
	"github.com/oryca/oryca/gateway/tool"
	"github.com/oryca/oryca/gateway/trie"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Run starts the server and blocks until it is asked to shut down.
func Run() {
	cfg := config.Load()
	logger.Init("oryca-gateway")

	// Redis — gateway's own DB (config, api-keys)
	redisClient, err := redisconn.Connect(redisconn.Options{
		Addr:         cfg.RedisAddress,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		PoolSize:     cfg.RedisPoolSize,
		MinIdleConns: cfg.RedisMinIdleConns,
		DialTimeout:  cfg.RedisDialTimeout,
		ReadTimeout:  cfg.RedisReadTimeout,
		WriteTimeout: cfg.RedisWriteTimeout,
	})
	if err != nil {
		logger.Error("failed to connect to Redis: " + err.Error())
		os.Exit(1)
	}
	logger.Info("Redis connected: " + cfg.RedisAddress + " db=" + strconv.Itoa(cfg.RedisDB))

	// Redis แยกสำหรับ UpstreamCache (response cache) — คนละ client จาก redisClient ข้างบนโดย
	// เจตนา แม้ตอนนี้ CacheRedis* จะ fallback มาชี้ instance/DB เดียวกันเสมอ (ยังไม่มี instance
	// แยกจริง) พอมี instance ที่สองแค่ตั้ง ORYCA_GW_CACHE_REDIS_ADDRESS ก็ย้ายได้ทันทีไม่ต้องแก้โค้ด
	cacheRedisClient, err := redisconn.Connect(redisconn.Options{
		Addr:         cfg.CacheRedisAddress,
		Password:     cfg.CacheRedisPassword,
		DB:           cfg.CacheRedisDB,
		PoolSize:     cfg.RedisPoolSize,
		MinIdleConns: cfg.RedisMinIdleConns,
		DialTimeout:  cfg.RedisDialTimeout,
		ReadTimeout:  cfg.RedisReadTimeout,
		WriteTimeout: cfg.RedisWriteTimeout,
	})
	if err != nil {
		logger.Error("failed to connect to cache Redis: " + err.Error())
		os.Exit(1)
	}
	logger.Info("Cache Redis connected: " + cfg.CacheRedisAddress + " db=" + strconv.Itoa(cfg.CacheRedisDB))

	c := cache.New(redisClient)
	bgCtx, bgCancel := context.WithCancel(context.Background())

	// SyncProvider — pulls services/sources/api-keys/JWKS from control-plane
	provider := gwsync.NewHTTPSyncProvider(gwsync.HTTPSyncConfig{
		BaseURL:        cfg.CPBaseURL,
		InternalSecret: cfg.InternalSecret,
		Timeout:        10 * time.Second,
	})

	// Trie — load warm-start data from Redis, then sync from provider
	ht := trie.NewHybrid(c, provider)
	if err := ht.Load(bgCtx); err != nil {
		logger.Error("failed to load trie from cache: " + err.Error())
	}
	ht.DoSync(bgCtx)
	ht.StartPolling(bgCtx, time.Duration(cfg.ServicePollInterval)*time.Second)

	// Per-pod in-memory user-freshness cache (packageId/enabled/verified/expiredAt) —
	// fetch-on-demand, not preloaded like the trie (60k+ users would make a full preload
	// a pure regression). See cache.UserFreshnessCache's doc comment.
	userFreshnessCache := cache.NewUserFreshnessCache(provider, time.Duration(cfg.UserCacheTTL)*time.Second, cfg.UserCacheRefreshConcurrency)

	// Api-key polling — separate interval. Also piggybacks each key's Owner snapshot
	// into userFreshnessCache for free — no extra control-plane calls — since this bulk
	// pull already carries packageId/enabled/verified/expiredAt per owner.
	startApiKeyPolling(bgCtx, provider, c, userFreshnessCache, time.Duration(cfg.ApiKeyPollInterval)*time.Second)

	// Real-time invalidation — soft dependency: an unreachable Redis or an empty channel
	// just means the gateway stays on HTTP polling alone (see service.SyncEventService).
	if cfg.SyncChannel != "" {
		listener := gwsync.NewRedisListener(redisClient, cfg.SyncChannel)
		service.NewSyncEventService(ht, c, userFreshnessCache).Start(bgCtx, listener)
	}

	// Auth service — fetch JWKS via provider (uses X-Internal-Key internally)
	authSvc := service.NewAuthService(provider, c, userFreshnessCache)
	if err := authSvc.FetchJWKS(bgCtx); err != nil {
		logger.Error("failed to fetch JWKS: " + err.Error())
		os.Exit(1)
	}
	authSvc.StartJWKSRefresh(bgCtx, 5*time.Minute)

	// Proxy service
	cb := breaker.NewPerDomainBreaker(breaker.CircuitBreakerConfig{
		MaxRequests:         cfg.CBMaxRequests,
		Interval:            cfg.CBInterval,
		Timeout:             cfg.CBTimeout,
		ConsecutiveFailures: cfg.CBConsecutiveFailures,
		IdleEvictAfter:      cfg.CBIdleEvictAfter,
	})
	cb.StartIdleEviction(bgCtx)
	proxySvc := service.NewProxyService(service.ProxyConfig{
		MaxIdleConns:          cfg.UpstreamMaxIdleConns,
		MaxIdleConnsPerHost:   cfg.UpstreamMaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.UpstreamMaxConnsPerHost,
		IdleConnTimeout:       cfg.UpstreamIdleConnTimeout,
		TLSHandshakeTimeout:   cfg.UpstreamTLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.UpstreamResponseHeaderTimeout,
		DialTimeout:           cfg.UpstreamDialTimeout,
		KeepAlive:             cfg.UpstreamKeepAlive,
	}, cb)

	// สร้าง publisher adapter — ตอนนี้ใช้ Redis Stream, เปลี่ยนเป็น Kafka ได้โดยไม่แตะ handler
	pub := event.NewRedisStreamPublisher(redisClient)
	upstreamCache := cache.NewUpstreamCache(cacheRedisClient, cfg.CacheDefaultTTL).WithMemory(cfg.CacheMemoryMB)
	h := handler.New(ht, proxySvc, authSvc, pub, cfg.PublicURL).WithRateLimiter(c).WithUpstreamCache(upstreamCache)

	// Echo
	e := echo.New()
	e.HideBanner = true

	allowOrigins := strings.Split(cfg.AllowOrigin, ",")
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: allowOrigins,
		AllowMethods: []string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin, echo.HeaderAccept,
			echo.HeaderContentType, echo.HeaderAuthorization,
			"X-Api-Key", "X-Request-Id", "Range", "If-Match", "Prefer",
		},
		ExposeHeaders: []string{
			"Content-Range", "X-Request-Id", "X-Upstream-Latency-Ms",
			"RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset", "Retry-After",
		},
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit(cfg.MaxRequestBody))
	// เปิดแค่ 2 header ที่ตรวจแล้วว่าไม่กระทบ upstream ในระบบเลย (Content-Type ทุกตัวที่เช็ค
	// ตรงกับเนื้อหาจริง 100%) — เจตนาไม่เปิด XSSProtection/XFrameOptions/HSTS/CSP: มี resource
	// เป็น static HTML ปนอยู่ (X-Frame-Options DENY จะพัง iframe embedding ถ้ามีคนใช้อยู่) และ
	// ยังไม่ยืนยัน TLS termination topology ชัดเจน (HSTS ตั้งผิดที่ทำให้ client ต่อ HTTP ไม่ได้อีก
	// เลยจนกว่า max-age หมด แก้คืนยาก) — ควรตั้งที่ ingress/LB แทนถ้าต้องการ
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		ContentTypeNosniff: "nosniff",
		ReferrerPolicy:     "strict-origin-when-cross-origin",
	}))

	gw := e.Group("/gateway/api")
	gw.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	gw.Any("/resources/*", h.Proxy)

	logger.Go("http server", func() {
		addr := cfg.AppHost + ":" + cfg.AppPort
		logger.Info("oryca-gateway starting on " + addr)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			logger.Error("server error: " + err.Error())
			os.Exit(1)
		}
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
	bgCancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()

	if err := e.Shutdown(shutCtx); err != nil {
		logger.Error("echo shutdown error: " + err.Error())
	}

	// Echo หยุดรับ request ใหม่แล้ว — รอ log/usage event ที่ค้างใน async publish queue ให้จบ
	// ก่อนปิด Redis จริง ไม่งั้น event ที่ยัง publish ไม่เสร็จตอน SIGTERM จะหายไปเงียบๆ
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 10*time.Second)
	h.DrainAsyncPublish(drainCtx)
	drainCancel()

	if err := redisClient.Close(); err != nil {
		logger.Error("redis close error: " + err.Error())
	}

	logger.Info("oryca-gateway stopped")
}

func startApiKeyPolling(ctx context.Context, provider gwsync.SyncProvider, c *cache.Cache, userCache *cache.UserFreshnessCache, interval time.Duration) {
	syncApiKeys(ctx, provider, c, userCache)
	logger.GoLoop("sync: api-key poll loop", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(tool.JitteredInterval(interval)):
				syncApiKeys(ctx, provider, c, userCache)
			}
		}
	})
}

func syncApiKeys(ctx context.Context, provider gwsync.SyncProvider, c *cache.Cache, userCache *cache.UserFreshnessCache) {
	fetchStart := time.Now()
	keys, err := provider.GetApiKeys(ctx)
	if err != nil {
		logger.Error("sync: get api-keys failed: " + err.Error())
		return
	}
	if keys == nil { // 304 not modified
		return
	}
	if len(keys) == 0 {
		logger.Error("sync: provider returned 0 api-keys, preserving existing")
		return
	}
	fetchMs := time.Since(fetchStart).Milliseconds()

	// batch ผ่าน Redis pipeline แทนยิงทีละตัว — ที่ 60k+ keys การยิงทีละคนช้าเกินรอบ polling
	writeStart := time.Now()
	failed, err := c.SetApiKeysBatch(ctx, keys)
	writeMs := time.Since(writeStart).Milliseconds()
	if err != nil {
		logger.Error("sync: set api-keys batch failed: " + err.Error())
	}
	for _, id := range failed {
		logger.Error("sync: set api-key failed: " + id)
	}

	// Free pre-warm: this bulk pull already carries each key's Owner (packageId/
	// enabled/verified/expiredAt), so feed it into userFreshnessCache too — no extra
	// control-plane calls, covers anyone with an api-key ahead of their first request.
	for _, key := range keys {
		if key.Owner != nil {
			userCache.Set(key.Owner.ID, key.Owner)
		}
	}

	logger.Info("sync: api-keys synced (" + strconv.Itoa(len(keys)) + " keys, fetch=" +
		strconv.FormatInt(fetchMs, 10) + "ms write=" + strconv.FormatInt(writeMs, 10) + "ms)")
}
