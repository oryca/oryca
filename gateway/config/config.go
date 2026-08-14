package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppHost string
	AppPort string

	RedisAddress      string
	RedisPassword     string
	RedisDB           int
	RedisPoolSize     int
	RedisMinIdleConns int
	RedisDialTimeout  time.Duration
	RedisReadTimeout  time.Duration
	RedisWriteTimeout time.Duration

	// CacheRedis*. Connection ของ UpstreamCache (response cache) แยกจาก RedisAddress ที่ใช้
	// สำหรับ auth/rate-limit โดยเจตนา เพื่อให้ย้าย cache ไป Redis instance ต่างหากได้แค่
	// ตั้ง env ใหม่ ไม่ต้องแก้โค้ด. ตอนนี้ default fallback ไปที่ Redis ตัวเดียวกับด้านบนเสมอ
	// (ยังไม่มี instance แยกจริง) พอมี instance ที่สองค่อยตั้ง ORYCA_GW_CACHE_REDIS_ADDRESS แยก
	CacheRedisAddress  string
	CacheRedisPassword string
	CacheRedisDB       int

	AllowOrigin string

	// MaxRequestBody. เพดานความปลอดภัย (safety net) ไม่ใช่ limit ธุรกิจ ตั้งสูงพอไม่กระทบ
	// GeoJSON feature ขนาดใหญ่ที่ map API ต้องรับได้จริง กันแค่ payload ผิดปกติ (หลาย GB)
	// รูปแบบตาม Echo BodyLimit เช่น "500M", "1G"
	MaxRequestBody string

	CPBaseURL           string
	PublicURL           string
	InternalSecret      string
	ServicePollInterval int
	ApiKeyPollInterval  int
	// SyncChannel คือ Redis pub/sub channel ที่ control-plane push real-time invalidation มา
	// ว่างเมื่อไหร่ = ปิด real-time เหลือ HTTP polling อย่างเดียว (ยังทำงานได้ปกติ)
	SyncChannel string

	// UserCacheTTL is the safety-net staleness window for UserFreshnessCache (seconds) —
	// how long a background-refresh-worthy entry is served before it is refreshed again.
	UserCacheTTL int
	// UserCacheRefreshConcurrency bounds how many background user-freshness refetches run
	// at once. Protects control-plane from a burst of many distinct cache misses at once
	// (e.g. right after a pod restart under real traffic).
	UserCacheRefreshConcurrency int

	CacheDefaultTTL int
	CacheMemoryMB   int // ขนาด L0 in-memory LRU ต่อ pod (MB), 0 = ปิด

	UpstreamMaxIdleConns          int
	UpstreamMaxIdleConnsPerHost   int
	UpstreamMaxConnsPerHost       int
	UpstreamIdleConnTimeout       time.Duration
	UpstreamTLSHandshakeTimeout   time.Duration
	UpstreamResponseHeaderTimeout time.Duration
	UpstreamDialTimeout           time.Duration
	UpstreamKeepAlive             time.Duration

	// CB* tune the per-upstream circuit breaker (breaker.PerDomainBreaker). Defaults match
	// what was previously hardcoded, so an unset env keeps existing behavior unchanged.
	CBMaxRequests         int
	CBInterval            time.Duration
	CBTimeout             time.Duration
	CBConsecutiveFailures int
	// CBIdleEvictAfter. A breaker untouched for this long is dropped from memory so
	// per-path cardinality can't grow unbounded; 0 disables eviction.
	CBIdleEvictAfter time.Duration
}

func Load() *Config {
	_ = godotenv.Load()

	redisAddress := mustGetEnv("ORYCA_GW_REDIS_ADDRESS")
	redisPassword := getEnv("ORYCA_GW_REDIS_PASSWORD", "")
	redisDB := getEnvInt("ORYCA_GW_REDIS_DB", 7)

	return &Config{
		AppHost:     getEnv("ORYCA_GW_HOST", "0.0.0.0"),
		AppPort:     getEnv("ORYCA_GW_PORT", "9002"),
		AllowOrigin: getEnv("ORYCA_GW_ALLOW_ORIGIN", "*"),

		MaxRequestBody: getEnv("ORYCA_GW_MAX_REQUEST_BODY", "500M"),

		CPBaseURL:           mustGetEnv("ORYCA_GW_CP_BASE_URL"),
		PublicURL:           getEnv("ORYCA_GW_PUBLIC_URL", ""),
		InternalSecret:      mustGetEnv("ORYCA_GW_INTERNAL_SECRET"),
		ServicePollInterval: getEnvInt("ORYCA_GW_SERVICE_POLL_INTERVAL", 60),
		ApiKeyPollInterval:  getEnvInt("ORYCA_GW_APIKEY_POLL_INTERVAL", 30),
		SyncChannel:         getEnv("ORYCA_GW_SYNC_CHANNEL", "oryca:sync-events"),

		UserCacheTTL:                getEnvInt("ORYCA_GW_USER_CACHE_TTL", 90),
		UserCacheRefreshConcurrency: getEnvInt("ORYCA_GW_USER_CACHE_REFRESH_CONCURRENCY", 25),

		RedisAddress:      redisAddress,
		RedisPassword:     redisPassword,
		RedisDB:           redisDB,
		RedisPoolSize:     getEnvInt("ORYCA_GW_REDIS_POOL_SIZE", 100),
		RedisMinIdleConns: getEnvInt("ORYCA_GW_REDIS_MIN_IDLE_CONNS", 10),
		RedisDialTimeout:  getEnvDuration("ORYCA_GW_REDIS_DIAL_TIMEOUT", 5) * time.Second,
		RedisReadTimeout:  getEnvDuration("ORYCA_GW_REDIS_READ_TIMEOUT", 3) * time.Second,
		RedisWriteTimeout: getEnvDuration("ORYCA_GW_REDIS_WRITE_TIMEOUT", 3) * time.Second,

		CacheRedisAddress:  getEnv("ORYCA_GW_CACHE_REDIS_ADDRESS", redisAddress),
		CacheRedisPassword: getEnv("ORYCA_GW_CACHE_REDIS_PASSWORD", redisPassword),
		CacheRedisDB:       getEnvInt("ORYCA_GW_CACHE_REDIS_DB", redisDB),

		CacheDefaultTTL: getEnvInt("ORYCA_GW_CACHE_DEFAULT_TTL", 300),
		CacheMemoryMB:   getEnvInt("ORYCA_GW_CACHE_MEMORY_MB", 256),

		UpstreamMaxIdleConns:          getEnvInt("ORYCA_GW_UPSTREAM_MAX_IDLE_CONNS", 1000),
		UpstreamMaxIdleConnsPerHost:   getEnvInt("ORYCA_GW_UPSTREAM_MAX_IDLE_CONNS_PER_HOST", 100),
		UpstreamMaxConnsPerHost:       getEnvInt("ORYCA_GW_UPSTREAM_MAX_CONNS_PER_HOST", 200),
		UpstreamIdleConnTimeout:       getEnvDuration("ORYCA_GW_UPSTREAM_IDLE_CONN_TIMEOUT", 90) * time.Second,
		UpstreamTLSHandshakeTimeout:   getEnvDuration("ORYCA_GW_UPSTREAM_TLS_HANDSHAKE_TIMEOUT", 10) * time.Second,
		UpstreamResponseHeaderTimeout: getEnvDuration("ORYCA_GW_UPSTREAM_RESPONSE_HEADER_TIMEOUT", 30) * time.Second,
		UpstreamDialTimeout:           getEnvDuration("ORYCA_GW_UPSTREAM_DIAL_TIMEOUT", 30) * time.Second,
		UpstreamKeepAlive:             getEnvDuration("ORYCA_GW_UPSTREAM_KEEP_ALIVE", 30) * time.Second,

		CBMaxRequests:         getEnvInt("ORYCA_GW_CB_MAX_REQUESTS", 5),
		CBInterval:            getEnvDuration("ORYCA_GW_CB_INTERVAL_SEC", 60) * time.Second,
		CBTimeout:             getEnvDuration("ORYCA_GW_CB_TIMEOUT_SEC", 30) * time.Second,
		CBConsecutiveFailures: getEnvInt("ORYCA_GW_CB_CONSECUTIVE_FAILURES", 5),
		CBIdleEvictAfter:      getEnvDuration("ORYCA_GW_CB_IDLE_EVICT_MIN", 30) * time.Minute,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("[CONFIG] required env not set: %s", key)
	}
	return val
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultSec int) time.Duration {
	return time.Duration(getEnvInt(key, defaultSec))
}
