package logger

import (
	"fmt"
	"runtime/debug"
	"time"
)

// Go runs fn in a new goroutine, recovering any panic so it can't crash the whole
// process. A single bad request/job must not take every other goroutine down with it.
func Go(name string, fn func()) {
	go func() {
		defer recoverOnce(name)
		fn()
	}()
}

// GoLoop runs fn in a new goroutine expected to loop until some external signal (e.g. a
// cancelled context) makes it return normally. If fn panics, it's recovered, logged, and
// restarted after a short backoff instead of silently dying on the first panic. A bug
// triggered by one bad sync cycle shouldn't permanently stop that sync going forward.
func GoLoop(name string, fn func()) {
	go func() {
		for {
			if !runRecovered(name, fn) {
				return
			}
			time.Sleep(time.Second)
		}
	}()
}

// runRecovered runs fn and reports whether it panicked (true) or returned normally (false).
func runRecovered(name string, fn func()) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			Error(name + ": recovered from panic: " + fmt.Sprint(r) + "\n" + string(debug.Stack()))
		}
	}()
	fn()
	return false
}

func recoverOnce(name string) {
	if r := recover(); r != nil {
		Error(name + ": recovered from panic: " + fmt.Sprint(r) + "\n" + string(debug.Stack()))
	}
}
