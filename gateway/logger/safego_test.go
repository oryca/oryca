package logger

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGo_RecoversPanicWithoutCrashing(t *testing.T) {
	done := make(chan struct{})
	Go("test", func() {
		defer close(done)
		panic("boom")
	})
	select {
	case <-done:
		// fn ran to its own defer despite panicking. Recover() didn't stop that,
		// it just kept the panic from escaping the goroutine.
	case <-time.After(time.Second):
		t.Fatal("goroutine never completed")
	}
}

func TestGo_RunsNormallyWithoutPanic(t *testing.T) {
	var ran atomic.Bool
	done := make(chan struct{})
	Go("test", func() {
		ran.Store(true)
		close(done)
	})
	select {
	case <-done:
		assert.True(t, ran.Load())
	case <-time.After(time.Second):
		t.Fatal("goroutine never completed")
	}
}

func TestGoLoop_RestartsAfterPanicThenStopsOnNormalReturn(t *testing.T) {
	var calls atomic.Int32
	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		GoLoop("test", func() {
			n := calls.Add(1)
			if n <= 2 {
				panic("boom")
			}
			// third call: return normally. GoLoop must not restart it again
			<-stop
		})
		close(done)
	}()

	// GoLoop backs off 1s per restart. 2 panics before the 3rd (non-panicking) call
	// means at least ~2s of backoff, so give this plenty of margin.
	require.Eventually(t, func() bool { return calls.Load() >= 3 }, 4*time.Second, 10*time.Millisecond,
		"expected fn to be restarted after each panic")

	close(stop)
	// give the (now-normally-returned) fn's iteration a moment, then confirm no 4th call happens
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(3), calls.Load(), "GoLoop must not restart fn once it returns normally")
}

func TestRunRecovered_ReportsWhetherItPanicked(t *testing.T) {
	panicked := runRecovered("test", func() { panic("boom") })
	assert.True(t, panicked)

	panicked = runRecovered("test", func() {})
	assert.False(t, panicked)
}
