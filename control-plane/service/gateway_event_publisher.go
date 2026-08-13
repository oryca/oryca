package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/oryca/oryca/control-plane/logger"

	"github.com/redis/go-redis/v9"
)

// Sync event type constants — wire contract shared with oryca-gateway/sync/event.go.
// Keep both sides in sync when changing.
const (
	SyncEventTypeReloadServices = "reload_services"
	SyncEventTypeApiKey         = "apikey"
	// SyncEventTypeUser carries a full model.UserFreshnessCache payload — single-user
	// authorization-field changes (Update/Create/Verify/Delete).
	SyncEventTypeUser = "user"
	// SyncEventTypeUserInvalidate carries a model.UserIDsPayload (ids only, no data) —
	// bulk operations (AssignUsers/RemoveUsers/BulkDelete) that touch many users in one
	// Mongo call must publish exactly one of these, never one SyncEventTypeUser per user.
	SyncEventTypeUserInvalidate = "user_invalidate"
)

type syncEventEnvelope struct {
	Type    string          `json:"type"`
	Version int64           `json:"version"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

const (
	gatewayEventPublishChSize = 2000
	gatewayEventPublishTTL    = 5 * time.Second
)

// GatewayEventPublisher pushes sync events to the gateway over a Redis channel
// (gateway/sync/redis_listener.go reads them).
//
// Publish never blocks the request path and never fails loudly: the gateway polls
// anyway, so a dropped event only costs a little freshness. A nil client turns the
// whole thing into a no-op.
type GatewayEventPublisher struct {
	client  *redis.Client
	channel string

	publishCh chan []byte
	shutdown  chan struct{}
	wg        sync.WaitGroup
}

func NewGatewayEventPublisher(client *redis.Client, channel string) *GatewayEventPublisher {
	p := &GatewayEventPublisher{
		client:    client,
		channel:   channel,
		publishCh: make(chan []byte, gatewayEventPublishChSize),
		shutdown:  make(chan struct{}),
	}
	if client == nil || channel == "" {
		logger.Info("gateway_event_publisher: not configured — real-time gateway sync disabled, HTTP polling only")
		return p
	}
	p.wg.Add(1)
	go p.publishLoop()
	return p
}

// Publish marshals payload, wraps it in the sync-event envelope (with a UnixMilli
// version the gateway uses to drop out-of-order deliveries), and queues it for async
// delivery. eventType must be one of the SyncEventType* constants. payload may be nil
// for thin-signal events (e.g. SyncEventTypeReloadServices) that carry no data — the
// gateway re-pulls the real thing itself via its existing HTTP+ETag path.
func (p *GatewayEventPublisher) Publish(eventType string, payload any) {
	if p.client == nil || p.channel == "" {
		return
	}
	var body json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			logger.Error("gateway_event_publisher: marshal payload failed type=" + eventType + " err=" + err.Error())
			return
		}
		body = b
	}
	env := syncEventEnvelope{Type: eventType, Version: time.Now().UnixMilli(), Payload: body}
	b, err := json.Marshal(env)
	if err != nil {
		logger.Error("gateway_event_publisher: marshal envelope failed type=" + eventType + " err=" + err.Error())
		return
	}
	select {
	case p.publishCh <- b:
	default:
		logger.Error("gateway_event_publisher: publish channel full, dropping event type=" + eventType)
	}
}

// publishLoop เป็น goroutine เดียวที่ยิงเข้า Redis — fire-and-forget ไม่รอ subscriber
func (p *GatewayEventPublisher) publishLoop() {
	defer p.wg.Done()
	for {
		select {
		case <-p.shutdown:
			for {
				select {
				case body := <-p.publishCh:
					p.publish(body)
				default:
					return
				}
			}
		case body := <-p.publishCh:
			p.publish(body)
		}
	}
}

func (p *GatewayEventPublisher) publish(body []byte) {
	// ctx มี timeout ของตัวเอง ไม่ผูกกับ request ที่ยิงมา — publish เป็น background งานเดียว
	ctx, cancel := context.WithTimeout(context.Background(), gatewayEventPublishTTL)
	defer cancel()

	if err := p.client.Publish(ctx, p.channel, body).Err(); err != nil {
		// event หายไปเงียบๆ — HTTP polling ฝั่ง gateway คุมอยู่แล้ว
		logger.Error("gateway_event_publisher: publish failed: " + err.Error())
	}
}

func (p *GatewayEventPublisher) Close() {
	if p.client == nil || p.channel == "" {
		return
	}
	close(p.shutdown)
	p.wg.Wait()
}
