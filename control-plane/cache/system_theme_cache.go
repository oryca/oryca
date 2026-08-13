package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const keySystemThemeDefault = "system-theme:default"

type SystemThemeCache struct {
	client  *redis.Client
	ttl     time.Duration
	sfGroup singleflight.Group
}

func NewSystemThemeCache(client *redis.Client, ttl time.Duration) *SystemThemeCache {
	return &SystemThemeCache{client: client, ttl: ttl}
}

func (c *SystemThemeCache) Get(ctx context.Context) (*model.SystemTheme, error) {
	val, err := c.client.Get(ctx, keySystemThemeDefault).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var theme model.SystemTheme
	if err := json.Unmarshal([]byte(val), &theme); err != nil {
		return nil, err
	}
	return &theme, nil
}

func (c *SystemThemeCache) Set(ctx context.Context, theme *model.SystemTheme) error {
	b, err := json.Marshal(theme)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, keySystemThemeDefault, b, c.ttl).Err()
}

func (c *SystemThemeCache) Delete(ctx context.Context) error {
	return c.client.Del(ctx, keySystemThemeDefault).Err()
}

func (c *SystemThemeCache) SF() *singleflight.Group {
	return &c.sfGroup
}
