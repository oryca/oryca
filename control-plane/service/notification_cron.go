package service

import (
	"context"
	"time"

	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"

	"github.com/robfig/cron/v3"
)

const (
	cronExpiringBatchSize = 500
	expiringWindow        = 7 * 24 * time.Hour
	oneDayWindow          = 24 * time.Hour
	cronLockTTL           = 15 * time.Minute

	cronLockKeyApiKeyExpiring = "lock:cron:apikey_expiring"

	// รันตี 1 ทุกวัน (เวลา server). Range check 7 วันรองรับกรณี cron ข้ามรอบ
	cronSpecApiKeyExpiring = "0 1 * * *"
)

type cronApiKeyRepo interface {
	FindExpiring(ctx context.Context, from, to time.Time, limit, offset int) ([]*model.ApiKey, error)
}

type cronLock interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (token string, ok bool, err error)
	Release(ctx context.Context, key, token string) error
}

type cronNotifier interface {
	Create(ctx context.Context, in *model.NotificationCreate) error
}

// NotificationCron รัน job รายวันสแกนหาเงื่อนไขที่ต้องแจ้งเตือน (เวลาผ่านไปเฉยๆ ก็ต้องแจ้ง ไม่มี event มา trigger)
// distributed lock ให้ pod เดียวรันตอน scale. Idempotency ที่แท้จริงมาจาก dedupKey (unique index)
type NotificationCron struct {
	c          *cron.Cron
	apiKeyRepo cronApiKeyRepo
	lock       cronLock
	notifier   cronNotifier
}

func NewNotificationCron(apiKeyRepo cronApiKeyRepo, lock cronLock, notifier cronNotifier) *NotificationCron {
	return &NotificationCron{
		c:          cron.New(),
		apiKeyRepo: apiKeyRepo,
		lock:       lock,
		notifier:   notifier,
	}
}

// Start ลงตาราง job แล้วเริ่ม scheduler (non-blocking)
func (nc *NotificationCron) Start() error {
	if _, err := nc.c.AddFunc(cronSpecApiKeyExpiring, nc.runApiKeyExpiring); err != nil {
		return err
	}
	nc.c.Start()
	return nil
}

// Stop หยุด scheduler แล้วรอ job ที่กำลังรันให้จบ (graceful) หรือจนกว่า ctx หมดเวลา. เรียกตอน shutdown
func (nc *NotificationCron) Stop(ctx context.Context) {
	stopped := nc.c.Stop()
	select {
	case <-stopped.Done():
	case <-ctx.Done():
		logger.Warn("cron: timed out waiting for running job to finish")
	}
}

// withLock รัน fn ภายใต้ distributed lock. Pod เดียวทำ, ปล่อย lock เมื่อจบ (ข้ามถ้า pod อื่นถืออยู่)
func (nc *NotificationCron) withLock(lockKey string, fn func(ctx context.Context)) {
	ctx, cancel := context.WithTimeout(context.Background(), cronLockTTL)
	defer cancel()

	token, ok, err := nc.lock.Acquire(ctx, lockKey, cronLockTTL)
	if err != nil {
		logger.Error("cron " + lockKey + ": acquire lock failed: " + err.Error())
		return
	}
	if !ok {
		return
	}
	defer func() {
		relCtx, relCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer relCancel()
		if relErr := nc.lock.Release(relCtx, lockKey, token); relErr != nil {
			logger.Error("cron " + lockKey + ": release lock failed: " + relErr.Error())
		}
	}()
	fn(ctx)
}

// ---- apikey_expiring ----

func (nc *NotificationCron) runApiKeyExpiring() {
	nc.withLock(cronLockKeyApiKeyExpiring, nc.scanApiKeyExpiring)
}

func (nc *NotificationCron) scanApiKeyExpiring(ctx context.Context) {
	now := time.Now().UTC()
	until := now.Add(expiringWindow)
	offset := 0
	for {
		keys, err := nc.apiKeyRepo.FindExpiring(ctx, now, until, cronExpiringBatchSize, offset)
		if err != nil {
			logger.Error("cron apikey_expiring: query failed: " + err.Error())
			return
		}
		for _, k := range keys {
			nc.notifyApiKeyExpiring(ctx, k)
		}
		if len(keys) < cronExpiringBatchSize {
			return
		}
		offset += cronExpiringBatchSize
	}
}

func (nc *NotificationCron) notifyApiKeyExpiring(ctx context.Context, k *model.ApiKey) {
	if k.OwnerBy == nil || k.ExpiredAt == nil {
		return
	}
	// dedup ต่อ (key, วันหมดอายุ). แจ้งครั้งเดียวต่อ deadline, ทน cron รันซ้ำ/ข้ามรอบ, renew แล้วแจ้งใหม่ได้
	dedupKey := "apikey_expiring:" + k.ID.Hex() + ":" + k.ExpiredAt.UTC().Format("2006-01-02")
	if err := nc.notifier.Create(ctx, &model.NotificationCreate{
		UserID:   *k.OwnerBy,
		Topic:    model.NotificationTopicApiKeyExpiring,
		Level:    model.NotificationLevelWarning,
		Title:    "API Key Expiring Soon",
		Message:  "Your API key \"" + k.Name + "\" will expire on " + k.ExpiredAt.UTC().Format("2006-01-02") + ".",
		DedupKey: dedupKey,
		Detail: map[string]any{
			"apiKeyId":  k.ID.Hex(),
			"keyAlias":  k.Name,
			"expiredAt": k.ExpiredAt,
		},
		CreatedBy: model.SystemObjectID,
	}); err != nil {
		logger.Error("apikey_expiring notification failed apiKeyId=" + k.ID.Hex() + " err=" + err.Error())
	}
}
