package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const keyConfiguration = "system:configuration"

type ConfigurationCache struct {
	client  *redis.Client
	ttl     time.Duration
	sfGroup singleflight.Group
}

func NewConfigurationCache(client *redis.Client, ttl time.Duration) *ConfigurationCache {
	return &ConfigurationCache{client: client, ttl: ttl}
}

// Get returns the cached configuration. Returns nil, nil on cache miss.
func (c *ConfigurationCache) Get(ctx context.Context) (*model.Configuration, error) {
	val, err := c.client.Get(ctx, keyConfiguration).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg model.Configuration
	if err := json.Unmarshal([]byte(val), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Set stores configuration in cache with TTL.
func (c *ConfigurationCache) Set(ctx context.Context, cfg *model.Configuration) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, keyConfiguration, b, c.ttl).Err()
}

// Delete removes the configuration cache entry.
func (c *ConfigurationCache) Delete(ctx context.Context) error {
	return c.client.Del(ctx, keyConfiguration).Err()
}

// SF returns the singleflight group for thundering-herd prevention.
func (c *ConfigurationCache) SF() *singleflight.Group {
	return &c.sfGroup
}
