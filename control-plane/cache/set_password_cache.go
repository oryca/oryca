package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefixSetPassword = "set-password:"

type SetPasswordCache struct {
	client *redis.Client
}

func NewSetPasswordCache(client *redis.Client) *SetPasswordCache {
	return &SetPasswordCache{client: client}
}

func (c *SetPasswordCache) Set(ctx context.Context, token, userID string, ttl time.Duration) error {
	return c.client.Set(ctx, keyPrefixSetPassword+token, userID, ttl).Err()
}

func (c *SetPasswordCache) Get(ctx context.Context, token string) (string, error) {
	return c.client.Get(ctx, keyPrefixSetPassword+token).Result()
}

func (c *SetPasswordCache) Delete(ctx context.Context, token string) error {
	return c.client.Del(ctx, keyPrefixSetPassword+token).Err()
}
