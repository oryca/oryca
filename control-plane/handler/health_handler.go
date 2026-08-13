package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type pinger interface {
	Ping(ctx context.Context) error
}

// mongoPinger และ redisPinger เป็น adapter ให้ mongo/redis client implement pinger
type mongoPinger struct{ client *mongo.Client }

func (m *mongoPinger) Ping(ctx context.Context) error {
	return m.client.Ping(ctx, nil)
}

type redisPinger struct{ client *redis.Client }

func (r *redisPinger) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

type HealthHandler struct {
	mongo pinger
	redis pinger
}

func NewHealthHandler(mongoClient *mongo.Client, redisClient *redis.Client) *HealthHandler {
	return &HealthHandler{
		mongo: &mongoPinger{mongoClient},
		redis: &redisPinger{redisClient},
	}
}

func statusCode(err error) int {
	if err != nil {
		return http.StatusServiceUnavailable
	}
	return http.StatusOK
}

func (h *HealthHandler) Check(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	mongoErr := h.mongo.Ping(ctx)
	redisErr := h.redis.Ping(ctx)

	status := &model.HealthStatus{
		Database: model.HealthDetail{Default: statusCode(mongoErr)},
		Redis:    model.HealthDetail{Default: statusCode(redisErr)},
	}

	code := http.StatusOK
	if mongoErr != nil || redisErr != nil {
		code = http.StatusServiceUnavailable
	}

	return c.JSON(code, status)
}
