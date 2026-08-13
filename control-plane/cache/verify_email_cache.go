package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefixVerifyEmail = "verify-email:"

type VerifyEmailCache struct {
	client *redis.Client
}

func NewVerifyEmailCache(client *redis.Client) *VerifyEmailCache {
	return &VerifyEmailCache{client: client}
}

func (c *VerifyEmailCache) Set(ctx context.Context, token, userID string, ttl time.Duration) error {
	return c.client.Set(ctx, keyPrefixVerifyEmail+token, userID, ttl).Err()
}

func (c *VerifyEmailCache) Get(ctx context.Context, token string) (string, error) {
	return c.client.Get(ctx, keyPrefixVerifyEmail+token).Result()
}

func (c *VerifyEmailCache) Delete(ctx context.Context, token string) error {
	return c.client.Del(ctx, keyPrefixVerifyEmail+token).Err()
}
