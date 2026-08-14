package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"
	"github.com/redis/go-redis/v9"
)

const keyPrefixApiKey = "apikey"

type ApiKeyCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewApiKeyCache(client *redis.Client, ttl time.Duration) *ApiKeyCache {
	return &ApiKeyCache{client: client, ttl: ttl}
}

func apiKeyCacheKey(rawKey string) string {
	return keyPrefixApiKey + ":" + tool.HashApiKey(rawKey)
}

// Get คืน ApiKey จาก cache โดยใช้ key value เป็น lookup key
func (c *ApiKeyCache) Get(ctx context.Context, keyValue string) (*model.ApiKey, error) {
	val, err := c.client.Get(ctx, apiKeyCacheKey(keyValue)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var cached model.ApiKeyCache
	if err := json.Unmarshal([]byte(val), &cached); err != nil {
		return nil, err
	}
	ak := &model.ApiKey{
		ApiKey:    cached.ApiKey,
		Enabled:   cached.Enabled,
		ExpiredAt: cached.ExpiredAt,
	}
	return ak, nil
}

// Set เก็บ ApiKey ลง cache โดยใช้ key value เป็น lookup key พร้อม embed owner info
func (c *ApiKeyCache) Set(ctx context.Context, ak *model.ApiKey, owner *model.User) error {
	payload := &model.ApiKeyCache{
		ID:        ak.ID.Hex(),
		Enabled:   ak.Enabled,
		ExpiredAt: ak.ExpiredAt,
	}
	if owner != nil {
		packageID := ""
		if owner.PackageID != nil {
			packageID = owner.PackageID.Hex()
		}
		payload.Owner = &model.ApiKeyOwnerCache{
			ID:        owner.ID.Hex(),
			Role:      owner.Role,
			PackageID: packageID,
			Verified:  owner.Verified,
			Enabled:   owner.Enabled,
			ExpiredAt: owner.ExpiredAt,
		}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, apiKeyCacheKey(ak.ApiKey), b, c.ttl).Err()
}

// apiKeyCacheBatchSize จำนวน key ต่อ Redis pipeline round-trip เดียวตอน full sync —
// เหตุผลเดียวกับ oryca-gateway/cache/api_key_cache.go: ลด round-trip จาก O(n) เหลือ
// O(n/batchSize) แทนการยิงทีละ key (ที่ 60k+ keys คือต่างกันหลักวินาทีกับหลักนาที)
const apiKeyCacheBatchSize = 500

// ApiKeyEntry คู่ ApiKey+owner หนึ่งรายการสำหรับ SetBatch
type ApiKeyEntry struct {
	ApiKey *model.ApiKey
	Owner  *model.User
}

// SetBatch เขียน api-key หลายตัวผ่าน Redis pipeline เป็น chunk แทนการยิงทีละตัว
// คืนจำนวนรายที่ set ไม่สำเร็จ (ถ้ามี) โดยรายอื่นใน batch เดียวกันยังสำเร็จตามปกติ
func (c *ApiKeyCache) SetBatch(ctx context.Context, entries []ApiKeyEntry) (failed int, err error) {
	for start := 0; start < len(entries); start += apiKeyCacheBatchSize {
		end := start + apiKeyCacheBatchSize
		if end > len(entries) {
			end = len(entries)
		}
		chunkFailed, chunkErr := c.setBatchChunk(ctx, entries[start:end])
		failed += chunkFailed
		if chunkErr != nil && err == nil {
			err = chunkErr
		}
	}
	return failed, err
}

func (c *ApiKeyCache) setBatchChunk(ctx context.Context, chunk []ApiKeyEntry) (failed int, err error) {
	pipe := c.client.Pipeline()
	cmds := make([]*redis.StatusCmd, len(chunk))
	for i, e := range chunk {
		payload := &model.ApiKeyCache{
			ID:        e.ApiKey.ID.Hex(),
			Enabled:   e.ApiKey.Enabled,
			ExpiredAt: e.ApiKey.ExpiredAt,
		}
		if e.Owner != nil {
			packageID := ""
			if e.Owner.PackageID != nil {
				packageID = e.Owner.PackageID.Hex()
			}
			payload.Owner = &model.ApiKeyOwnerCache{
				ID:        e.Owner.ID.Hex(),
				Role:      e.Owner.Role,
				PackageID: packageID,
				Verified:  e.Owner.Verified,
				Enabled:   e.Owner.Enabled,
				ExpiredAt: e.Owner.ExpiredAt,
			}
		}
		b, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			failed++
			continue
		}
		cmds[i] = pipe.Set(ctx, apiKeyCacheKey(e.ApiKey.ApiKey), b, c.ttl)
	}

	if _, execErr := pipe.Exec(ctx); execErr != nil {
		err = execErr
	}
	for _, cmd := range cmds {
		if cmd != nil && cmd.Err() != nil {
			failed++
		}
	}
	return failed, err
}

// Delete ลบ ApiKey ออกจาก cache
func (c *ApiKeyCache) Delete(ctx context.Context, keyValues []string) error {
	if len(keyValues) == 0 {
		return nil
	}
	keys := make([]string, len(keyValues))
	for i, v := range keyValues {
		keys[i] = apiKeyCacheKey(v)
	}
	return c.client.Del(ctx, keys...).Err()
}
