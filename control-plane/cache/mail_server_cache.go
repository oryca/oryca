package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const keyMailServerDefault = "mail-server:default"

type MailServerCache struct {
	client  *redis.Client
	ttl     time.Duration
	sfGroup singleflight.Group
}

func NewMailServerCache(client *redis.Client, ttl time.Duration) *MailServerCache {
	return &MailServerCache{client: client, ttl: ttl}
}

// Get คืน default mail server จาก cache ถ้ามี, nil ถ้า miss
func (c *MailServerCache) Get(ctx context.Context) (*model.MailServer, error) {
	val, err := c.client.Get(ctx, keyMailServerDefault).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var ms model.MailServer
	if err := json.Unmarshal([]byte(val), &ms); err != nil {
		return nil, err
	}
	return &ms, nil
}

// Set เก็บ default mail server ลง cache พร้อม TTL
func (c *MailServerCache) Set(ctx context.Context, ms *model.MailServer) error {
	b, err := json.Marshal(ms)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, keyMailServerDefault, b, c.ttl).Err()
}

// Delete ลบ cache (ใช้ก่อน set default ใหม่)
func (c *MailServerCache) Delete(ctx context.Context) error {
	return c.client.Del(ctx, keyMailServerDefault).Err()
}

// SF คืน singleflight group สำหรับป้องกัน thundering herd
func (c *MailServerCache) SF() *singleflight.Group {
	return &c.sfGroup
}
