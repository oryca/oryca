package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const upstreamCacheKeyPrefix = "upstream:cache:"

const contentTypeHeader = "Content-Type"

// upstreamCacheableStatus คือ status code ที่ cache ได้ตาม RFC 9111 Section 3
var upstreamCacheableStatus = map[int]bool{
	http.StatusOK:               true,
	http.StatusMovedPermanently: true,
	http.StatusNotFound:         true,
}

type UpstreamCacheEntry struct {
	StatusCode int                 `json:"sc"`
	Headers    map[string][]string `json:"h"`
	Body       []byte              `json:"b,omitempty"`
	ETag       string              `json:"et,omitempty"`
	TTLSec     int                 `json:"ttl"` // original TTL สำหรับคำนวณ Age
}

// Magic header to distinguish custom binary format from JSON
var binaryMagic = []byte{0x0b, 0x1a, 0x0c, 0x01}

// MarshalBinary serializes UpstreamCacheEntry to a custom binary format
func (e *UpstreamCacheEntry) MarshalBinary() ([]byte, error) {
	headersJSON, err := json.Marshal(e.Headers)
	if err != nil {
		return nil, err
	}

	etagBytes := []byte(e.ETag)

	totalSize := len(binaryMagic) + 4 + 4 + 2 + len(etagBytes) + 4 + len(headersJSON) + len(e.Body)
	buf := make([]byte, totalSize)

	// Copy magic header
	copy(buf[0:4], binaryMagic)

	binary.BigEndian.PutUint32(buf[4:8], uint32(e.StatusCode))
	binary.BigEndian.PutUint32(buf[8:12], uint32(e.TTLSec))

	idx := 12
	// ETag
	binary.BigEndian.PutUint16(buf[idx:idx+2], uint16(len(etagBytes)))
	idx += 2
	copy(buf[idx:idx+len(etagBytes)], etagBytes)
	idx += len(etagBytes)

	// Headers JSON
	binary.BigEndian.PutUint32(buf[idx:idx+4], uint32(len(headersJSON)))
	idx += 4
	copy(buf[idx:idx+len(headersJSON)], headersJSON)
	idx += len(headersJSON)

	// Body
	copy(buf[idx:], e.Body)

	return buf, nil
}

// UnmarshalBinary deserializes UpstreamCacheEntry from custom binary format (falls back to JSON)
func (e *UpstreamCacheEntry) UnmarshalBinary(data []byte) error {
	// Fallback to JSON if magic header is not present
	if len(data) < len(binaryMagic) || !bytes.Equal(data[0:len(binaryMagic)], binaryMagic) {
		return json.Unmarshal(data, e)
	}

	if len(data) < 18 { // magic(4) + status(4) + ttl(4) + etagLen(2) + headersLen(4)
		return errors.New("binary data too short")
	}

	e.StatusCode = int(binary.BigEndian.Uint32(data[4:8]))
	e.TTLSec = int(binary.BigEndian.Uint32(data[8:12]))

	idx := 12
	// ETag
	etagLen := int(binary.BigEndian.Uint16(data[idx : idx+2]))
	idx += 2
	if idx+etagLen > len(data) {
		return errors.New("invalid etag length")
	}
	e.ETag = string(data[idx : idx+etagLen])
	idx += etagLen

	// Headers JSON
	headersLen := int(binary.BigEndian.Uint32(data[idx : idx+4]))
	idx += 4
	if idx+headersLen > len(data) {
		return errors.New("invalid headers length")
	}
	if err := json.Unmarshal(data[idx:idx+headersLen], &e.Headers); err != nil {
		return err
	}
	idx += headersLen

	// Body
	e.Body = make([]byte, len(data)-idx)
	copy(e.Body, data[idx:])

	return nil
}

type UpstreamCache struct {
	redis      *redis.Client
	defaultTTL time.Duration
	mem        *memoryCache
}

func NewUpstreamCache(rc *redis.Client, defaultTTLSec int) *UpstreamCache {
	return &UpstreamCache{
		redis:      rc,
		defaultTTL: time.Duration(defaultTTLSec) * time.Second,
	}
}

func (c *UpstreamCache) WithMemory(memoryMB int) *UpstreamCache {
	if memoryMB > 0 {
		c.mem = newMemoryCache(int64(memoryMB) << 20)
	}
	return c
}

// UpstreamCacheKey สร้าง SHA-256 key จาก method + upstream URL
func UpstreamCacheKey(method, upstreamURL string) string {
	h := sha256.Sum256([]byte(method + ":" + upstreamURL))
	return upstreamCacheKeyPrefix + fmt.Sprintf("%x", h)
}

// IsCacheableRequest ตรวจว่า HTTP method นี้ cache ได้ตาม RFC 9111
func IsCacheableRequest(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// MaxCacheableBodyBytes เพดานขนาด body ต่อ entry ใน Redis — ก้อนใหญ่ทำ Redis บล็อก
// กระทบ auth/rate-limit ที่ใช้ Redis เดียวกัน ของใหญ่ให้ passthrough แทน
// 10MB รองรับ geo response ขนาดกลาง — ของใหญ่กว่านี้รอ gzip cache + S3-backed
const MaxCacheableBodyBytes = 10 << 20 // 10MB

// IsCacheableSize ตรวจว่า body เล็กพอที่จะเก็บใน Redis
func IsCacheableSize(bodyLen int) bool {
	return bodyLen <= MaxCacheableBodyBytes
}

// nonCacheableContentTypes คือ Content-Type ที่ไม่ควร cache — tile/binary formats
// ที่มี URL unique ต่อ z/x/y ทำให้ entries สะสมเยอะมากและกิน memory Redis
// image/* ครอบ png/jpeg/webp/tiff/gif ทั้งหมด
// protobuf variants ครอบ vector tile format ที่ไม่ใช่ mapbox standard
var nonCacheableContentTypes = map[string]bool{
	"application/vnd.mapbox-vector-tile": true,
	"application/x-protobuf":             true,
	"application/vnd.google.protobuf":    true,
}

const nonCacheableContentTypeImagePrefix = "image/"

// isImageOrTileContentType ตรวจว่า Content-Type เป็น image หรือ vector tile format
func isImageOrTileContentType(contentType string) bool {
	ct := strings.ToLower(strings.SplitN(contentType, ";", 2)[0])
	return strings.HasPrefix(ct, nonCacheableContentTypeImagePrefix) || nonCacheableContentTypes[ct]
}

// IsCacheableResponse ตรวจว่า upstream response นี้ควร cache หรือไม่
// ไม่ cache ถ้า: status ไม่อยู่ใน default cacheable set, มี no-store, private, หรือเป็น tile/image
func (c *UpstreamCache) IsCacheableResponse(statusCode int, headers http.Header) bool {
	if !upstreamCacheableStatus[statusCode] {
		return false
	}
	cc := strings.ToLower(headers.Get("Cache-Control"))
	if strings.Contains(cc, "no-store") || strings.Contains(cc, "private") {
		return false
	}
	// ไม่ cache tile และ image — URL unique ต่อ z/x/y ทำให้ Redis entries เยอะเกินไป
	if isImageOrTileContentType(headers.Get(contentTypeHeader)) {
		return false
	}
	return true
}

// ExtractTTL อ่าน s-maxage → max-age จาก response Cache-Control, fallback defaultTTL
// s-maxage มี priority สูงกว่า max-age สำหรับ shared cache (RFC 9111 Section 5.2.2.9)
func (c *UpstreamCache) ExtractTTL(headers http.Header) time.Duration {
	cc := headers.Get("Cache-Control")
	if v := cacheControlInt(cc, "s-maxage"); v >= 0 {
		return time.Duration(v) * time.Second
	}
	if v := cacheControlInt(cc, "max-age"); v >= 0 {
		return time.Duration(v) * time.Second
	}
	return c.defaultTTL
}

func cacheControlInt(cc, directive string) int {
	prefix := directive + "="
	for _, part := range strings.Split(cc, ",") {
		tok := strings.TrimSpace(strings.ToLower(part))
		if strings.HasPrefix(tok, prefix) {
			if n, err := strconv.Atoi(strings.TrimPrefix(tok, prefix)); err == nil && n >= 0 {
				return n
			}
		}
	}
	return -1
}

// Get ดึง cache entry คืน (entry, age, hit)
// age = จำนวนวินาทีที่ entry อยู่ใน cache แล้ว
func (c *UpstreamCache) Get(ctx context.Context, key string) (*UpstreamCacheEntry, int, bool) {
	// L0 memory cache check
	if c.mem != nil {
		if entry, age, hit := c.mem.get(key); hit {
			return entry, age, true
		}
	}

	// L1 Redis check
	pipe := c.redis.Pipeline()
	getCmd := pipe.Get(ctx, key)
	ttlCmd := pipe.TTL(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, 0, false
	}
	val, err := getCmd.Result()
	if err != nil {
		return nil, 0, false
	}

	var entry UpstreamCacheEntry
	if err := entry.UnmarshalBinary([]byte(val)); err != nil {
		return nil, 0, false
	}

	age := 0
	if remaining, ttlErr := ttlCmd.Result(); ttlErr == nil && remaining >= 0 {
		age = entry.TTLSec - int(remaining.Seconds())
		if age < 0 {
			age = 0
		}
	}

	// Save back to L0 memory cache
	if c.mem != nil {
		c.saveL0(key, &entry, age)
	}

	return &entry, age, true
}

func (c *UpstreamCache) saveL0(key string, entry *UpstreamCacheEntry, age int) {
	if c.mem == nil {
		return
	}
	l0 := *entry
	l0.Headers = make(map[string][]string, len(entry.Headers))
	for k, v := range entry.Headers {
		l0.Headers[k] = v
	}
	c.mem.set(key, l0, time.Duration(entry.TTLSec-age)*time.Second)
}

// headersToStripOnStore คือ headers ที่ GW จัดการเองหรือเปิดเผยข้อมูล internal — ไม่ควรเก็บไว้ใน entry
var headersToStripOnStore = []string{"Age", "Date", "Via", "Server", "Vary"}

// Set เก็บ entry ลง Redis พร้อม mirror ลง L0 memory cache
func (c *UpstreamCache) Set(ctx context.Context, key string, entry *UpstreamCacheEntry, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}
	toStore := *entry
	toStore.TTLSec = int(ttl.Seconds())

	toStore.Headers = sanitizeHeadersForStore(toStore.Headers)

	b, err := toStore.MarshalBinary()
	if err != nil {
		return err
	}

	if err := c.redis.Set(ctx, key, b, ttl).Err(); err != nil {
		return err
	}

	if c.mem != nil {
		c.mem.set(key, toStore, ttl)
	}

	return nil
}

// SyntheticETag คำนวณ ETag แบบรวดเร็วโดยใช้ SHA-256 จากข้อมูลดิบ
func SyntheticETag(body []byte) string {
	h := sha256.Sum256(body)
	return fmt.Sprintf(`W/"gw-%x"`, h)
}
