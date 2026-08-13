package service

import (
	"context"
	"sync"
	"time"

	"github.com/oryca/oryca/control-plane/logger"
)

// notifyWG นับ background notification goroutine ที่ยังไม่เสร็จ
var notifyWG sync.WaitGroup

// runNotifyBackground รันแจ้งเตือนแบบ background ไม่ block response
// ใช้ WithoutCancel เพราะ request ctx จะถูก cancel ทันทีที่ handler return
func runNotifyBackground(ctx context.Context, timeout time.Duration, fn func(context.Context)) {
	notifyWG.Add(1)
	go func() {
		defer notifyWG.Done()
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()
		fn(bgCtx)
	}()
}

// WaitForBackgroundNotifications รอ notification goroutine ที่ค้างอยู่ให้เสร็จ (เรียกตอน shutdown)
func WaitForBackgroundNotifications(ctx context.Context) {
	waitGroupWithDeadline(ctx, &notifyWG)
}

// waitGroupWithDeadline แยกจาก WaitForBackgroundNotifications เพื่อให้ test ใช้ WaitGroup ของตัวเองได้
func waitGroupWithDeadline(ctx context.Context, wg *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		logger.Warn("shutdown: timed out waiting for background notifications to drain")
	}
}
