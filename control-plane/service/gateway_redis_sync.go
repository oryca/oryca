package service

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"

	"github.com/redis/go-redis/v9"
)

const (
	redisKeyGatewayServicePaths = "gateway_service:paths"
	redisKeySourceRegistry      = "source:registry"
	redisChanGatewaySync        = "gateway_service:sync"
)

type syncAction string

const (
	syncActionUpsert syncAction = "upsert"
	syncActionDelete syncAction = "delete"
	syncActionReload syncAction = "reload"
)

type syncMessage struct {
	Action      syncAction `json:"action"`
	ServiceID   string     `json:"serviceId,omitempty"`
	SourceAlias string     `json:"sourceAlias,omitempty"`
	PodID       string     `json:"podId"`
}

type gatewayServicePayload struct {
	ID         string                    `json:"id"`
	Name       string                    `json:"name"`
	BasePath   string                    `json:"basePath"`
	Enabled    bool                      `json:"enabled"`
	IsPublic   bool                      `json:"isPublic"`
	PackageIDs []string                  `json:"packageIds,omitempty"`
	Resources  []*gatewayResourcePayload `json:"resources"`
}

type gatewayResourcePayload struct {
	Path        string   `json:"path"`
	Methods     []string `json:"methods"`
	SourceAlias string   `json:"sourceAlias"`
}

type gatewaySourcePayload struct {
	Alias       string                  `json:"alias"`
	Type        string                  `json:"type"`
	Protocol    string                  `json:"protocol,omitempty"`
	URL         string                  `json:"url,omitempty"`
	Headers     []*model.SourceKeyValue `json:"headers,omitempty"`
	ContentType string                  `json:"contentType,omitempty"`
	Body        string                  `json:"body,omitempty"`
}

// gatewaySyncPublisher is the narrow slice of GatewayEventPublisher this needs — the
// real-time reload_services thin signal, alongside the existing Redis Pub/Sub publish
// below (which the gateway has never subscribed to — see g.publish).
type gatewaySyncPublisher interface {
	Publish(eventType string, payload any)
}

type GatewayRedisSync struct {
	redis     *redis.Client
	podID     string
	ttl       time.Duration
	publisher gatewaySyncPublisher
}

func NewGatewayRedisSync(redis *redis.Client, ttl time.Duration, publisher gatewaySyncPublisher) *GatewayRedisSync {
	hostname, _ := os.Hostname()
	return &GatewayRedisSync{redis: redis, podID: "control-plane:" + hostname, ttl: ttl, publisher: publisher}
}

// buildServicePayload assembles the Redis hash payload from the service document itself —
// callers always have the full *model.GatewayService in hand already, so this never depends
// on reading back whatever (if anything) is currently stored in Redis.
func buildServicePayload(svc *model.GatewayService, packageIDs []string) *gatewayServicePayload {
	enabled := svc.Enabled != nil && *svc.Enabled
	isPublic := svc.IsPublic != nil && *svc.IsPublic

	resources := make([]*gatewayResourcePayload, 0, len(svc.ResourcePaths))
	for _, rp := range svc.ResourcePaths {
		resources = append(resources, &gatewayResourcePayload{
			Path:        rp.Path,
			Methods:     rp.Methods,
			SourceAlias: rp.SourceAlias,
		})
	}

	return &gatewayServicePayload{
		ID:         svc.ID.Hex(),
		Name:       svc.Name,
		BasePath:   svc.BasePath,
		Enabled:    enabled,
		IsPublic:   isPublic,
		PackageIDs: packageIDs,
		Resources:  resources,
	}
}

func (g *GatewayRedisSync) writeServicePayload(ctx context.Context, payload *gatewayServicePayload) error {
	b, err := json.Marshal(payload)
	if err != nil {
		logger.Error("gateway_redis_sync: marshal service payload failed id=" + payload.ID + " err=" + err.Error())
		return err
	}

	if err := g.redis.HSet(ctx, redisKeyGatewayServicePaths, payload.ID, string(b)).Err(); err != nil {
		logger.Error("gateway_redis_sync: write service payload failed id=" + payload.ID + " err=" + err.Error())
		return err
	}
	g.redis.Expire(ctx, redisKeyGatewayServicePaths, g.ttl)

	return g.publish(ctx, syncMessage{
		Action:    syncActionUpsert,
		ServiceID: payload.ID,
		PodID:     g.podID,
	})
}

func (g *GatewayRedisSync) SyncService(ctx context.Context, svc *model.GatewayService) error {
	if g.redis == nil {
		logger.Error("gateway_redis_sync: redis client is nil, skipping SyncService")
		return nil
	}
	return g.writeServicePayload(ctx, buildServicePayload(svc, nil))
}

// SyncServicePackageIDs writes the service's package links straight from svc — it does not
// read back an existing Redis entry first, so it works even if this service was never synced
// at boot (e.g. missed by pagination) or Redis was flushed since.
func (g *GatewayRedisSync) SyncServicePackageIDs(ctx context.Context, svc *model.GatewayService, packageIDs []string) error {
	if g.redis == nil {
		return nil
	}
	return g.writeServicePayload(ctx, buildServicePayload(svc, packageIDs))
}

func (g *GatewayRedisSync) DeleteService(ctx context.Context, serviceID string) error {
	if g.redis == nil {
		logger.Error("gateway_redis_sync: redis client is nil, skipping DeleteService")
		return nil
	}
	if err := g.redis.HDel(ctx, redisKeyGatewayServicePaths, serviceID).Err(); err != nil {
		logger.Error("gateway_redis_sync: delete service failed id=" + serviceID + " err=" + err.Error())
		return err
	}
	logger.Info("gateway_redis_sync: deleted service id=" + serviceID)
	return g.publish(ctx, syncMessage{
		Action:    syncActionDelete,
		ServiceID: serviceID,
		PodID:     g.podID,
	})
}

func (g *GatewayRedisSync) SyncSource(ctx context.Context, src *model.GatewaySource) error {
	if g.redis == nil {
		logger.Error("gateway_redis_sync: redis client is nil, skipping SyncSource")
		return nil
	}
	payload := &gatewaySourcePayload{
		Alias:       src.Alias,
		Type:        src.Type,
		Protocol:    src.Protocol,
		URL:         src.URL,
		Headers:     src.Headers,
		ContentType: src.ContentType,
		Body:        src.Body,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		logger.Error("gateway_redis_sync: marshal failed: " + err.Error())
		return err
	}

	if err := g.redis.HSet(ctx, redisKeySourceRegistry, src.Alias, string(b)).Err(); err != nil {
		logger.Error("gateway_redis_sync: sync source failed alias=" + src.Alias + " err=" + err.Error())
		return err
	}
	g.redis.Expire(ctx, redisKeySourceRegistry, g.ttl)

	return g.publish(ctx, syncMessage{
		Action:      syncActionUpsert,
		SourceAlias: src.Alias,
		PodID:       g.podID,
	})
}

func (g *GatewayRedisSync) DeleteSource(ctx context.Context, alias string) error {
	if g.redis == nil {
		return nil
	}
	if err := g.redis.HDel(ctx, redisKeySourceRegistry, alias).Err(); err != nil {
		return err
	}
	return g.publish(ctx, syncMessage{
		Action:      syncActionDelete,
		SourceAlias: alias,
		PodID:       g.podID,
	})
}

func (g *GatewayRedisSync) publish(ctx context.Context, msg syncMessage) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	// thin signal — no payload, gateway re-pulls services/sources itself via its
	// existing HTTP+ETag path (see oryca-gateway's SyncEventService.Apply)
	if g.publisher != nil {
		g.publisher.Publish(SyncEventTypeReloadServices, nil)
	}
	return g.redis.Publish(ctx, redisChanGatewaySync, string(b)).Err()
}
