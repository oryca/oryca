package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	AppHost         string
	AppPort         string
	DBUrl           string
	DBName          string
	DBMaxPool       string
	DBMinPool       string
	DBSocketTimeout string
	AllowOrigin     string
	JWTPrivateKey   string
	JWTPublicKey    string

	RedisAddress      string
	RedisPassword     string
	RedisDB           string
	RedisExpires      string
	RedisRoutingTTL   string
	RedisPoolSize     string
	RedisMinIdleConns string
	RedisDialTimeout  string
	RedisReadTimeout  string
	RedisWriteTimeout string

	// HTTP server hardening (วินาที ยกเว้น BodyLimit เป็นขนาด เช่น "10M")
	BodyLimit         string
	ReadHeaderTimeout string
	ReadTimeout       string
	IdleTimeout       string

	InternalSecret     string
	InternalSecretPrev string

	SyncChannel string

	// SeedDir อ่านไฟล์ seed จาก directory แทนชุดที่ embed มากับ binary (ว่าง = ใช้ embed)
	SeedDir      string
	RootEmail    string
	RootPassword string

	// LogRetentionDays คือ TTL ของ collection access_logs — เปลี่ยนค่าแล้ว bootstrap collMod ให้เอง
	LogRetentionDays  int
	LogConsumerEnable bool
}

func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		AppHost:         mustGetEnv("ORYCA_API_HOST"),
		AppPort:         mustGetEnv("ORYCA_API_PORT"),
		DBUrl:           mustGetEnv("ORYCA_API_DB_URL"),
		DBName:          mustGetEnv("ORYCA_API_DB_NAME"),
		DBMaxPool:       getEnv("ORYCA_API_DB_MAX_POOL_SIZE", "50"),
		DBMinPool:       getEnv("ORYCA_API_DB_MIN_POOL_SIZE", "10"),
		DBSocketTimeout: getEnv("ORYCA_API_DB_SOCKET_TIMEOUT", "20"),
		AllowOrigin:     getEnv("ORYCA_API_ALLOW_ORIGIN", "*"),
		JWTPrivateKey:   getEnv("ORYCA_API_JWT_PRIVATE_KEY", "auth-private.key"),
		JWTPublicKey:    getEnv("ORYCA_API_JWT_PUBLIC_KEY", "auth-public.key"),

		RedisAddress:      mustGetEnv("ORYCA_API_REDIS_ADDRESS"),
		RedisPassword:     getEnv("ORYCA_API_REDIS_PASSWORD", ""),
		RedisDB:           getEnv("ORYCA_API_REDIS_DB", "0"),
		RedisExpires:      getEnv("ORYCA_API_REDIS_EXPIRES", "300"),
		RedisRoutingTTL:   getEnv("ORYCA_API_REDIS_ROUTING_TTL", "86400"),
		RedisPoolSize:     getEnv("ORYCA_API_REDIS_POOL_SIZE", "50"),
		RedisMinIdleConns: getEnv("ORYCA_API_REDIS_MIN_IDLE_CONNS", "10"),
		RedisDialTimeout:  getEnv("ORYCA_API_REDIS_DIAL_TIMEOUT", "5"),
		RedisReadTimeout:  getEnv("ORYCA_API_REDIS_READ_TIMEOUT", "3"),
		RedisWriteTimeout: getEnv("ORYCA_API_REDIS_WRITE_TIMEOUT", "3"),

		BodyLimit:         getEnv("ORYCA_API_BODY_LIMIT", "10M"),
		ReadHeaderTimeout: getEnv("ORYCA_API_READ_HEADER_TIMEOUT", "10"),
		ReadTimeout:       getEnv("ORYCA_API_READ_TIMEOUT", "60"),
		IdleTimeout:       getEnv("ORYCA_API_IDLE_TIMEOUT", "120"),

		InternalSecret:     getEnv("ORYCA_INTERNAL_SECRET", ""),
		InternalSecretPrev: getEnv("ORYCA_INTERNAL_SECRET_PREV", ""),

		SyncChannel: getEnv("ORYCA_CP_SYNC_CHANNEL", "oryca:sync-events"),

		LogRetentionDays:  getEnvInt("ORYCA_API_LOG_RETENTION_DAYS", 14),
		LogConsumerEnable: getEnv("ORYCA_API_LOG_CONSUMER_ENABLED", "true") != "false",

		SeedDir:      getEnv("ORYCA_API_SEED_DIR", ""),
		RootEmail:    getEnv("ORYCA_API_ROOT_EMAIL", ""),
		RootPassword: getEnv("ORYCA_API_ROOT_PASSWORD", ""),
	}
}

// getEnv อ่าน env พร้อม fallback default
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getEnvInt อ่าน env เป็น int — ว่างหรือแปลงไม่ได้ใช้ค่า default
func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

// mustGetEnv หยุด process ถ้าไม่มี env ที่จำเป็น
func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("[CONFIG] required env not set: %s", key)
	}
	return val
}
