package cache

import (
	"context"
	"encoding/json"
	"github.com/oryca/oryca/gateway/model"
	gwsync "github.com/oryca/oryca/gateway/sync"
	"time"
)

const keySourceRegistry = "source:registry"
const sourceCacheTTL = 24 * time.Hour

// SaveSources writes all sources to Redis, replacing the previous snapshot atomically via pipeline.
func (c *Cache) SaveSources(ctx context.Context, sources []*gwsync.SourcePayload) error {
	if len(sources) == 0 {
		return nil
	}
	pipe := c.client.Pipeline()
	pipe.Del(ctx, keySourceRegistry)
	for _, src := range sources {
		raw, err := json.Marshal(src)
		if err != nil {
			continue
		}
		pipe.HSet(ctx, keySourceRegistry, src.Alias, raw)
	}
	pipe.Expire(ctx, keySourceRegistry, sourceCacheTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *Cache) LoadAllSources(ctx context.Context) (map[string]*model.Source, error) {
	result, err := c.client.HGetAll(ctx, keySourceRegistry).Result()
	if err != nil {
		return nil, err
	}

	sources := make(map[string]*model.Source, len(result))
	for alias, raw := range result {
		var src model.Source
		if err := json.Unmarshal([]byte(raw), &src); err != nil {
			continue
		}
		src.Alias = alias
		sources[alias] = &src
	}
	return sources, nil
}
