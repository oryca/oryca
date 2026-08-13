package sync

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oryca/oryca/gateway/logger"

	"github.com/redis/go-redis/v9"
)

const eventChBufferSize = 256

// RedisListener reads sync events the control plane publishes on a Redis channel.
// Every pod gets every message and nothing is stored, so a pod that is down simply
// misses them. HTTP polling covers that gap.
type RedisListener struct {
	client  *redis.Client
	channel string
	events  chan Event
}

func NewRedisListener(client *redis.Client, channel string) *RedisListener {
	return &RedisListener{
		client:  client,
		channel: channel,
		events:  make(chan Event, eventChBufferSize),
	}
}

func (l *RedisListener) Listen(ctx context.Context) <-chan Event {
	logger.GoLoop("sync: redis listener", func() { l.consumeLoop(ctx) })
	return l.events
}

// consumeLoop re-subscribes if the channel closes. go-redis handles reconnects itself,
// so a Redis outage only drops the gateway back to polling.
func (l *RedisListener) consumeLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		sub := l.client.Subscribe(ctx, l.channel)
		ch := sub.Channel()
		logger.Info("sync: redis listener subscribed channel=" + l.channel)

		for msg := range ch {
			l.handleMessage([]byte(msg.Payload))
		}

		_ = sub.Close()
		if ctx.Err() != nil {
			return
		}
		// channel ปิดโดยไม่ได้ shutdown — รอสั้นๆ แล้ว subscribe ใหม่ กัน busy loop
		logger.Error("sync: redis listener: subscription closed, resubscribing")
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (l *RedisListener) handleMessage(body []byte) {
	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil {
		logger.Error("sync: redis listener: bad event payload: " + err.Error())
		return
	}
	select {
	case l.events <- ev:
	default:
		// applier ตามไม่ทัน — ข้าม event นี้ TTL polling จะ sync ให้เองในรอบถัดไป (ดู main.go)
		logger.Error("sync: redis listener: event channel full, dropping event type=" + string(ev.Type))
	}
}
