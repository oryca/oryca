package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestWaitForBackgroundNotifications_WaitsForCompletion verifies WaitForBackgroundNotifications
// blocks until an in-flight background goroutine actually finishes, given enough time budget.
func TestWaitForBackgroundNotifications_WaitsForCompletion(t *testing.T) {
	var finished atomic.Bool

	runNotifyBackground(context.Background(), time.Second, func(bgCtx context.Context) {
		time.Sleep(50 * time.Millisecond)
		finished.Store(true)
	})

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	WaitForBackgroundNotifications(waitCtx)

	assert.True(t, finished.Load(), "expected background work to complete before Wait returns")
}

// TestWaitForBackgroundNotifications_ManyConcurrent จำลองโหลดสูง. หลาย goroutine พร้อมกัน
// ต้องรอครบทุกตัวจริงๆ ไม่ใช่แค่ตัวแรก/ตัวสุดท้าย
func TestWaitForBackgroundNotifications_ManyConcurrent(t *testing.T) {
	const n = 200
	var completed atomic.Int64

	for i := 0; i < n; i++ {
		runNotifyBackground(context.Background(), time.Second, func(bgCtx context.Context) {
			completed.Add(1)
		})
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	WaitForBackgroundNotifications(waitCtx)

	assert.EqualValues(t, n, completed.Load())
}

// TestWaitGroupWithDeadline_RespectsDeadline verifies the core wait-with-timeout logic gives up
// once its own ctx deadline passes, instead of blocking forever on a slow/stuck goroutine
// (important for graceful shutdown. Must not hang the process past the orchestrator's grace period).
//
// ใช้ *sync.WaitGroup ของตัวเอง (ไม่ใช่ notifyWG ตัว global) เพราะ test นี้ตั้งใจให้ timeout ก่อนงานจริง
// จะเสร็จ ซึ่งจะทิ้ง monitor goroutine ค้างไว้พักหนึ่ง. ถ้าใช้ WaitGroup ร่วมกับ test อื่นจะไปชนกับ
// Add() ของ test ถัดไปได้ (sync.WaitGroup ไม่รองรับการ reuse ข้าม "wave" ที่ยัง wait ค้างอยู่)
func TestWaitGroupWithDeadline_RespectsDeadline(t *testing.T) {
	var wg sync.WaitGroup
	workDone := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(500 * time.Millisecond)
		close(workDone)
	}()

	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	waitGroupWithDeadline(waitCtx, &wg)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 200*time.Millisecond, "expected Wait to return promptly on deadline, not block for the full goroutine duration")

	<-workDone // ปล่อยให้งานจริงจบแบบ deterministic ก่อนออกจาก test (local wg เท่านั้น ไม่กระทบ test อื่น)
}
