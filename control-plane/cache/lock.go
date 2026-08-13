package cache

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// releaseLockScript ปล่อย lock เฉพาะเมื่อ token ยังตรง (atomic compare-and-delete)
// กันเผลอลบ lock ที่ pod อื่นจับต่อไปแล้วหลัง TTL ของเราหมด
var releaseLockScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`)

// DistributedLock คือ Redis-based lock สำหรับให้ pod เดียวรัน job ที่ไม่ควรทับซ้อน (เช่น cron รายวัน)
type DistributedLock struct {
	client *redis.Client
}

func NewDistributedLock(client *redis.Client) *DistributedLock {
	return &DistributedLock{client: client}
}

// Acquire จับ lock — คืน (token, true) ถ้าจับได้, ("", false) ถ้ามี pod อื่นถืออยู่
// ttl ต้องยาวพอครอบเวลา job ทั้งหมด (ถ้าหลุดก่อน job จบ pod อื่นอาจเริ่มทับได้)
func (l *DistributedLock) Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	token := uuid.NewString()
	_, err := l.client.SetArgs(ctx, key, token, redis.SetArgs{Mode: "NX", TTL: ttl}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", false, nil
		}
		return "", false, err
	}
	return token, true, nil
}

// Release ปล่อย lock เฉพาะถ้า token ยังเป็นของเรา
func (l *DistributedLock) Release(ctx context.Context, key, token string) error {
	return releaseLockScript.Run(ctx, l.client, []string{key}, token).Err()
}
