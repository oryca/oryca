// Package consumer moves the gateway's request logs off Redis and into MongoDB,
// where the dashboard queries them.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	streamUsageLog = "stream:usage-log"
	consumerGroup  = "cp-log-consumer"

	readCount = 100
	readBlock = 2 * time.Second

	// flushSize/flushInterval. เขียนเมื่อครบจำนวนหรือครบเวลา แล้วแต่อะไรถึงก่อน
	flushSize     = 100
	flushInterval = time.Second

	// เอา message ที่ค้างใน PEL ของ consumer ที่ตายไปแล้วมาทำต่อ
	reclaimIdle     = 60 * time.Second
	reclaimInterval = 30 * time.Second
	writeTimeout    = 10 * time.Second
)

// LogStore is the sink the consumer writes to. Narrow on purpose so a future
// sink (OpenSearch, ClickHouse, …) can replace it without touching the gateway
// or this consumer's Redis handling.
type LogStore interface {
	InsertMany(ctx context.Context, docs []*model.AccessLog) error
}

// LogConsumer reads the gateway's log stream in batches and acks only after the
// batch is stored. A crash before the ack leaves messages pending for the reclaim
// loop, so logs are never lost quietly.
type LogConsumer struct {
	client   *redis.Client
	store    LogStore
	name     string
	shutdown chan struct{}
	wg       sync.WaitGroup
}

func NewLogConsumer(client *redis.Client, store LogStore, name string) *LogConsumer {
	return &LogConsumer{
		client:   client,
		store:    store,
		name:     name,
		shutdown: make(chan struct{}),
	}
}

// Start creates the consumer group if missing and runs the read and reclaim
// loops in the background. ctx becomes those loops' lifetime, so it must outlive
// the consumer. Cancelling it stops consuming; Stop is the intended way to end.
func (c *LogConsumer) Start(ctx context.Context) error {
	if err := c.client.XGroupCreateMkStream(ctx, streamUsageLog, consumerGroup, "0").Err(); err != nil &&
		!strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}

	c.wg.Add(2)
	go func() {
		defer c.wg.Done()
		c.readLoop(ctx)
	}()
	go func() {
		defer c.wg.Done()
		c.reclaimLoop(ctx)
	}()
	logger.Info("log consumer: started group=" + consumerGroup + " consumer=" + c.name)
	return nil
}

// Stop signals both loops and waits for the in-flight batch to be written.
func (c *LogConsumer) Stop(ctx context.Context) {
	close(c.shutdown)
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		logger.Error("log consumer: shutdown timed out with a batch still in flight")
	}
}

func (c *LogConsumer) stopping() bool {
	select {
	case <-c.shutdown:
		return true
	default:
		return false
	}
}

// readLoop blocks on XREADGROUP and flushes whenever the batch is full or the
// flush interval elapses. So a trickle of traffic still lands within a second.
func (c *LogConsumer) readLoop(ctx context.Context) {
	batch := make([]*model.AccessLog, 0, flushSize)
	ids := make([]string, 0, flushSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		if c.stopping() || ctx.Err() != nil {
			c.flush(ctx, &batch, &ids)
			return
		}

		select {
		case <-ticker.C:
			c.flush(ctx, &batch, &ids)
		default:
		}

		streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: c.name,
			Streams:  []string{streamUsageLog, ">"},
			Count:    readCount,
			Block:    readBlock,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || ctx.Err() != nil {
				// ไม่มีข้อความในรอบ block นี้. Flush ของค้างแล้ววนต่อ
				c.flush(ctx, &batch, &ids)
				continue
			}
			logger.Error("log consumer: read failed: " + err.Error())
			c.flush(ctx, &batch, &ids)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				if doc := decodeMessage(msg); doc != nil {
					batch = append(batch, doc)
				}
				// message ที่ decode ไม่ได้ก็ต้อง ack ไม่งั้นค้าง PEL ตลอดกาล
				ids = append(ids, msg.ID)
			}
		}
		if len(batch) >= flushSize || len(ids) >= flushSize {
			c.flush(ctx, &batch, &ids)
		}
	}
}

// reclaimLoop takes over messages another consumer read but never acked (pod
// killed mid-batch). XAutoClaim hands them back to this consumer's PEL, and the
// next flush acks them.
func (c *LogConsumer) reclaimLoop(ctx context.Context) {
	ticker := time.NewTicker(reclaimInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.shutdown:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		start := "0-0"
		for {
			msgs, next, err := c.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
				Stream:   streamUsageLog,
				Group:    consumerGroup,
				Consumer: c.name,
				MinIdle:  reclaimIdle,
				Start:    start,
				Count:    readCount,
			}).Result()
			if err != nil {
				if !errors.Is(err, redis.Nil) && ctx.Err() == nil {
					logger.Error("log consumer: reclaim failed: " + err.Error())
				}
				break
			}
			if len(msgs) == 0 {
				break
			}

			batch := make([]*model.AccessLog, 0, len(msgs))
			ids := make([]string, 0, len(msgs))
			for _, msg := range msgs {
				if doc := decodeMessage(msg); doc != nil {
					batch = append(batch, doc)
				}
				ids = append(ids, msg.ID)
			}
			c.flush(ctx, &batch, &ids)
			logger.Info("log consumer: reclaimed " + strconv.Itoa(len(ids)) + " pending messages")

			if next == "0-0" || next == "" {
				break
			}
			start = next
		}
	}
}

// flush writes the batch then acks it. On a write error nothing is acked, so the
// messages stay pending and are retried by the reclaim loop rather than lost.
func (c *LogConsumer) flush(ctx context.Context, batch *[]*model.AccessLog, ids *[]string) {
	if len(*ids) == 0 {
		return
	}

	// ctx ของตัวเอง. Shutdown ที่ cancel ctx แม่ต้องไม่ทำให้ batch สุดท้ายหาย
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()

	if len(*batch) > 0 {
		if err := c.store.InsertMany(writeCtx, *batch); err != nil {
			logger.Error("log consumer: insert failed, leaving " + strconv.Itoa(len(*ids)) + " messages pending: " + err.Error())
			*batch = (*batch)[:0]
			*ids = (*ids)[:0]
			return
		}
	}
	if err := c.client.XAck(writeCtx, streamUsageLog, consumerGroup, *ids...).Err(); err != nil {
		// เขียนลง Mongo ไปแล้วแต่ ack ไม่ผ่าน. Reclaim จะหยิบมาใหม่ อาจได้ log ซ้ำ
		// ยอมรับได้เพราะ dashboard เป็นสถิติรวม ไม่ได้ใช้ตัดสินใจแบบ exactly-once
		logger.Error("log consumer: ack failed: " + err.Error())
	}
	*batch = (*batch)[:0]
	*ids = (*ids)[:0]
}

// streamLog mirrors the JSON the gateway publishes (oryca-gateway/logger/log_model.go).
type streamLog struct {
	Time      *time.Time `json:"time"`
	TraceID   string     `json:"traceId"`
	UserID    string     `json:"userId"`
	ApiKeyID  string     `json:"apiKeyId"`
	ServiceID string     `json:"serviceId"`
	Request   *struct {
		Host   string `json:"host"`
		IP     string `json:"ip"`
		Method string `json:"method"`
		Path   string `json:"path"`
	} `json:"request"`
	Response *struct {
		StatusCode *int   `json:"statusCode"`
		Size       *int64 `json:"size"`
		Duration   *int64 `json:"duration"`
	} `json:"response"`
}

// decodeMessage turns one stream entry into a document, or nil if it is not
// something we can store. The caller still acks those so they do not pile up.
func decodeMessage(msg redis.XMessage) *model.AccessLog {
	raw, ok := msg.Values["log"].(string)
	if !ok {
		logger.Error("log consumer: message " + msg.ID + " has no log field")
		return nil
	}
	var sl streamLog
	if err := json.Unmarshal([]byte(raw), &sl); err != nil {
		logger.Error("log consumer: message " + msg.ID + " is not valid JSON: " + err.Error())
		return nil
	}

	doc := &model.AccessLog{
		ID:        bson.NewObjectID(),
		TraceID:   sl.TraceID,
		UserID:    sl.UserID,
		ApiKeyID:  sl.ApiKeyID,
		ServiceID: sl.ServiceID,
	}
	if sl.Time != nil {
		doc.Time = sl.Time.UTC()
	} else {
		doc.Time = time.Now().UTC()
	}
	if sl.Request != nil {
		doc.Host = sl.Request.Host
		doc.IP = sl.Request.IP
		doc.Method = sl.Request.Method
		doc.Path = sl.Request.Path
	}
	if sl.Response != nil {
		if sl.Response.StatusCode != nil {
			doc.StatusCode = *sl.Response.StatusCode
		}
		if sl.Response.Size != nil {
			doc.Size = *sl.Response.Size
		}
		if sl.Response.Duration != nil {
			doc.DurationMs = *sl.Response.Duration
		}
	}
	return doc
}
