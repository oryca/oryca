package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/oryca/oryca/gateway/model"
	gwsync "github.com/oryca/oryca/gateway/sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const apiKeyCacheTTL = 2 * time.Hour

func apiKeyRedisKey(rawKey string) string { return "apikey:" + hashApiKey(rawKey) }

// apiKeySyncBatchSize — chunk ละ 500 กัน pipeline ใหญ่เกินจนบล็อก Redis
const apiKeySyncBatchSize = 500

// SetApiKeysBatch เขียน api-key หลายตัวผ่าน Redis pipeline เป็น chunk แทนการยิงทีละตัว
// คืน id ของ api-key ที่ marshal/set ไม่สำเร็จ (ถ้ามี) ให้ผู้เรียก log ต่อได้เหมือนเดิม
func (c *Cache) SetApiKeysBatch(ctx context.Context, keys []*gwsync.ApiKeyPayload) (failedIDs []string, err error) {
	for start := 0; start < len(keys); start += apiKeySyncBatchSize {
		end := start + apiKeySyncBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		chunkFailed, chunkErr := c.setApiKeysChunk(ctx, keys[start:end])
		failedIDs = append(failedIDs, chunkFailed...)
		if chunkErr != nil && err == nil {
			err = chunkErr
		}
	}
	return failedIDs, err
}

func (c *Cache) setApiKeysChunk(ctx context.Context, chunk []*gwsync.ApiKeyPayload) (failedIDs []string, err error) {
	pipe := c.client.Pipeline()
	cmds := make([]*redis.StatusCmd, len(chunk))
	for i, ak := range chunk {
		raw, marshalErr := json.Marshal(toModelApiKey(ak))
		if marshalErr != nil {
			failedIDs = append(failedIDs, ak.ID)
			continue
		}
		cmds[i] = pipe.Set(ctx, apiKeyRedisKey(ak.ApiKey), raw, apiKeyCacheTTL)
	}

	if _, execErr := pipe.Exec(ctx); execErr != nil {
		err = execErr
	}
	for i, cmd := range cmds {
		if cmd != nil && cmd.Err() != nil {
			failedIDs = append(failedIDs, chunk[i].ID)
		}
	}
	return failedIDs, err
}

func toModelApiKey(ak *gwsync.ApiKeyPayload) *model.ApiKey {
	m := &model.ApiKey{
		ID:        ak.ID,
		ApiKey:    ak.ApiKey,
		Enabled:   ak.Enabled,
		ExpiredAt: ak.ExpiredAt,
	}
	if ak.Restriction != nil {
		m.Restriction = &model.ApiKeyRestriction{
			Type:  ak.Restriction.Type,
			Items: ak.Restriction.Items,
		}
	}
	if ak.Owner != nil {
		m.Owner = &model.User{
			ID:        ak.Owner.ID,
			Role:      ak.Owner.Role,
			PackageID: ak.Owner.PackageID,
			Verified:  ak.Owner.Verified,
			Enabled:   ak.Owner.Enabled,
			ExpiredAt: ak.Owner.ExpiredAt,
		}
	}
	return m
}

func hashApiKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (c *Cache) GetApiKey(ctx context.Context, rawKey string) (*model.ApiKey, error) {
	val, err := c.client.Get(ctx, apiKeyRedisKey(rawKey)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ak model.ApiKey
	if err := json.Unmarshal([]byte(val), &ak); err != nil {
		return nil, err
	}
	return &ak, nil
}
