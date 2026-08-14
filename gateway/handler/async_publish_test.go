package handler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// flakyPublisher fails the first N calls before succeeding. Simulates a Redis
// stream that flaps briefly under load, which is what triggers the
// retry-with-sleep loops in Proxy().
type flakyPublisher struct {
	failLogCalls int32
	logCalls     atomic.Int32
}

func (p *flakyPublisher) PublishLog(ctx context.Context, body []byte) error {
	if p.logCalls.Add(1) <= p.failLogCalls {
		return context.DeadlineExceeded
	}
	return nil
}

// TestEnqueueAsyncPublish_DoesNotBlockCaller verifies the core property behind
// the fix: a publish job that internally sleeps (retry backoff after a
// simulated Redis flap) must not add that sleep time to the caller's wall
// time. Before the fix, this same retry loop ran inline in Proxy() and its
// sleep(500ms)+sleep(1s) was double-counted into the logged response.duration
// even though the HTTP response had already been flushed to the client.
func TestEnqueueAsyncPublish_DoesNotBlockCaller(t *testing.T) {
	pub := &flakyPublisher{failLogCalls: 2} // fails twice, succeeds on 3rd (matches delays len=3)
	h := New(nil, nil, nil, pub, "")

	const jobs = 50
	done := make(chan struct{}, jobs)

	start := time.Now()
	for i := 0; i < jobs; i++ {
		h.enqueueAsyncPublish(func() {
			delays := []time.Duration{0, 500 * time.Millisecond, time.Second}
			for _, d := range delays {
				if d > 0 {
					time.Sleep(d)
				}
				if err := pub.PublishLog(context.Background(), []byte("{}")); err == nil {
					break
				}
			}
			done <- struct{}{}
		})
	}
	elapsed := time.Since(start)

	// Enqueueing 50 jobs whose bodies sleep up to 1.5s each must return
	// near-instantly. This is the caller-facing guarantee that keeps
	// retry backoff out of the measured request duration.
	if elapsed > 50*time.Millisecond {
		t.Fatalf("enqueueAsyncPublish blocked the caller for %v, want near-instant return", elapsed)
	}

	for i := 0; i < jobs; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("background worker did not finish job %d in time", i)
		}
	}
}

// TestDrainAsyncPublish_WaitsForPendingJobs verifies the shutdown-safety guarantee:
// DrainAsyncPublish must block until in-flight jobs (including ones still
// retrying) finish, so main.go can safely call it before closing the Redis
// connection those jobs depend on.
func TestDrainAsyncPublish_WaitsForPendingJobs(t *testing.T) {
	h := New(nil, nil, nil, &flakyPublisher{}, "")

	var finished atomic.Bool
	h.enqueueAsyncPublish(func() {
		time.Sleep(200 * time.Millisecond)
		finished.Store(true)
	})

	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h.DrainAsyncPublish(drainCtx)

	if !finished.Load() {
		t.Fatal("DrainAsyncPublish returned before the pending job finished")
	}
}

// TestDrainAsyncPublish_TimesOutInsteadOfHanging verifies that a stuck job
// (e.g. Redis permanently down) can't hang shutdown forever. The drain must
// respect the caller's context deadline.
func TestDrainAsyncPublish_TimesOutInsteadOfHanging(t *testing.T) {
	h := New(nil, nil, nil, &flakyPublisher{}, "")
	h.enqueueAsyncPublish(func() {
		time.Sleep(5 * time.Second)
	})

	drainCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	h.DrainAsyncPublish(drainCtx)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("DrainAsyncPublish took %v, want to respect ~100ms context deadline", elapsed)
	}
}

// TestEnqueueAsyncPublish_DropsWhenQueueFull verifies the backpressure guarantee:
// when workers can't keep up, enqueue must drop and return rather than block
// the request goroutine indefinitely (the failure mode we were explicitly
// trying to avoid instead of trading one latency bug for unbounded goroutines).
func TestEnqueueAsyncPublish_DropsWhenQueueFull(t *testing.T) {
	h := &Handler{asyncPublishCh: make(chan func(), 1)} // no workers draining it
	h.asyncPublishCh <- func() {}                       // fill the only slot

	done := make(chan struct{})
	go func() {
		h.enqueueAsyncPublish(func() {}) // queue full -> must hit default branch, not block
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("enqueueAsyncPublish blocked when queue was full — backpressure guarantee violated")
	}
}
