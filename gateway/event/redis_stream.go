package event

import (
	"context"

	"github.com/redis/go-redis/v9"
)

const streamUsageLog = "stream:usage-log"

// RedisStreamPublisher คือ adapter ที่ implement Publisher ผ่าน Redis Stream
// ใช้ XADD ซึ่งเร็ว < 1ms เหมาะสำหรับ hot path ของ gateway
type RedisStreamPublisher struct {
	client *redis.Client
}

func NewRedisStreamPublisher(client *redis.Client) Publisher {
	return &RedisStreamPublisher{client: client}
}

// PublishLog ส่ง structured log JSON ลง stream:usage-log
// MaxLen 500_000 เพราะรับทุก request. ปรับตาม memory Redis
func (p *RedisStreamPublisher) PublishLog(ctx context.Context, body []byte) error {
	return p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamUsageLog,
		MaxLen: 500_000,
		Approx: true,
		Values: map[string]any{
			"log": string(body),
		},
	}).Err()
}
