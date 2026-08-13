package cache

import (
	"context"
	"encoding/json"
	"github.com/oryca/oryca/gateway/model"
	"time"
)

const keyGatewayServicePaths = "gateway_service:paths"
const serviceCacheTTL = 24 * time.Hour

// SaveServices writes all services to Redis, replacing the previous snapshot atomically via pipeline.
func (c *Cache) SaveServices(ctx context.Context, services []*model.GatewayService) error {
	if len(services) == 0 {
		return nil
	}
	pipe := c.client.Pipeline()
	pipe.Del(ctx, keyGatewayServicePaths)
	for _, svc := range services {
		raw, err := json.Marshal(svc)
		if err != nil {
			continue
		}
		pipe.HSet(ctx, keyGatewayServicePaths, svc.ID, raw)
	}
	pipe.Expire(ctx, keyGatewayServicePaths, serviceCacheTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *Cache) LoadAllServices(ctx context.Context) ([]*model.GatewayService, error) {
	result, err := c.client.HGetAll(ctx, keyGatewayServicePaths).Result()
	if err != nil {
		return nil, err
	}

	services := make([]*model.GatewayService, 0, len(result))
	for _, raw := range result {
		var svc model.GatewayService
		if err := json.Unmarshal([]byte(raw), &svc); err != nil {
			continue
		}
		services = append(services, &svc)
	}
	return services, nil
}
