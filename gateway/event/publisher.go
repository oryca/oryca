package event

import (
	"context"
)

// Publisher คือ port สำหรับส่ง event ออกจาก gateway
// adapter ที่รองรับ: Redis Stream, Kafka (อนาคต)
type Publisher interface {
	// PublishLog ส่ง structured log JSON ทุก proxy request ลง stream:usage-log
	// body คือ JSON bytes ที่ได้จาก logger.BuildProxyLogJSON()
	PublishLog(ctx context.Context, body []byte) error
}
