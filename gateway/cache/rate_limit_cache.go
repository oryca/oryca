package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func nowUnixMs() int64 { return time.Now().UnixMilli() }

// RateLimitTier คือ 1 tier ของ sliding window rate limit — ใช้ทั้งใน cache และ handler
type RateLimitTier struct {
	Limit     int
	WindowSec int
}

// RateLimitKey สร้าง Redis key สำหรับ sliding window ต่อ user/package/service/path/window
// รวม packageID เพื่อแยก counter เมื่อ user อยู่ใน package ต่างกัน
// รวม windowSec เพื่อให้ tier ต่างกัน (window ต่างกัน) ใช้ key ต่างกันโดยอัตโนมัติ
func RateLimitKey(userID, packageID, serviceID, resourcePath string, windowSec int) string {
	return fmt.Sprintf("ratelimit:%s:%s:%s:%s:%d", userID, packageID, serviceID, resourcePath, windowSec)
}

// slidingWindowScript ตรวจสอบและเพิ่ม request เข้า sliding window แบบ atomic
// ตรวจ BEFORE add เพื่อหลีกเลี่ยง rollback
// KEYS[1] = rate limit key
// ARGV[1] = now (unix ms)
// ARGV[2] = window (ms)
// ARGV[3] = max requests
// ARGV[4] = unique member (request identifier)
// Returns: {allowed (1/0), current_count, reset_ms}
// reset_ms = เวลา (ms) ที่เหลือจนกว่า oldest entry จะหมดอายุออกจาก window
var slidingWindowScript = redis.NewScript(`
local now    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local max    = tonumber(ARGV[3])
local member = ARGV[4]
local ttlSec = math.ceil(window / 1000) + 1

redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
local count = tonumber(redis.call('ZCARD', KEYS[1]))

-- คำนวณ reset_ms จาก oldest entry ที่ยังอยู่ใน window
local function reset_ms_from_oldest()
  local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
  if #oldest > 0 then
    local oldest_score = tonumber(oldest[2])
    local ms = oldest_score + window - now
    if ms < 0 then ms = 0 end
    return ms
  end
  return window
end

if count >= max then
  redis.call('EXPIRE', KEYS[1], ttlSec)
  return {0, count, reset_ms_from_oldest()}
end

redis.call('ZADD', KEYS[1], now, member)
redis.call('EXPIRE', KEYS[1], ttlSec)
return {1, count + 1, reset_ms_from_oldest()}
`)

// AllowRequest ตรวจสอบ 1 tier ของ sliding window บน Redis
// คืน allowed=true ถ้า request ผ่าน, current count, และ resetMs (ms ที่เหลือจนกว่า oldest entry หมดอายุ)
func (c *Cache) AllowRequest(ctx context.Context, key string, limit, windowSec int, member string) (allowed bool, current int64, resetMs int64, err error) {
	nowMs := strconv.FormatInt(nowUnixMs(), 10)
	windowMs := strconv.FormatInt(int64(windowSec)*1000, 10)
	maxStr := strconv.Itoa(limit)

	res, err := slidingWindowScript.Run(ctx, c.client, []string{key}, nowMs, windowMs, maxStr, member).Int64Slice()
	if err != nil {
		return false, 0, 0, err
	}
	return res[0] == 1, res[1], res[2], nil
}

// Allow implements handler.RateLimiter — ตรวจทุก tier, deny ถ้า tier ใด tier หนึ่งเกิน limit
// memberHint ควรเป็น requestID (UUID) เพื่อให้ member ใน sorted set unique ทุก request
// fail-open: Redis error → allow (caller จัดการ)
func (c *Cache) Allow(ctx context.Context, userID, packageID, serviceID, resourcePath, memberHint string, tiers []RateLimitTier) (allowed bool, limit int, remaining int, retryAfterSec int, resetSec int, err error) {
	if len(tiers) == 0 {
		return true, 0, 0, 0, 0, nil
	}
	member := memberHint
	if member == "" {
		member = strconv.FormatInt(nowUnixMs(), 10) + ":" + userID
	}
	for _, tier := range tiers {
		key := RateLimitKey(userID, packageID, serviceID, resourcePath, tier.WindowSec)
		ok, current, resetMs, rerr := c.AllowRequest(ctx, key, tier.Limit, tier.WindowSec, member)
		if rerr != nil {
			return true, 0, 0, 0, 0, rerr // fail-open
		}
		// resetMs → วินาที ปัดขึ้นเสมอ (อย่างน้อย 1 วินาที)
		rs := int((resetMs + 999) / 1000)
		if rs < 1 {
			rs = 1
		}
		if !ok {
			return false, tier.Limit, 0, rs, rs, nil
		}
		rem := tier.Limit - int(current)
		if limit == 0 || rem < remaining {
			limit = tier.Limit
			remaining = rem
			resetSec = rs
		}
	}
	return true, limit, remaining, 0, resetSec, nil
}
