package transform

import (
	"sync"
	"testing"
)

// TestCachedRegexp — pattern เดิมต้องได้ instance เดียวกัน (compile ครั้งเดียว)
func TestCachedRegexp(t *testing.T) {
	re1, err := cachedRegexp(`(<foo[^>]*>)([^<]*)(<foo>)`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	re2, err := cachedRegexp(`(<foo[^>]*>)([^<]*)(<foo>)`)
	if err != nil {
		t.Fatalf("compile 2nd: %v", err)
	}
	if re1 != re2 {
		t.Error("same pattern must return cached instance")
	}
}

// TestCachedRegexpInvalid — pattern ผิดต้องคืน error ไม่ panic
func TestCachedRegexpInvalid(t *testing.T) {
	if _, err := cachedRegexp(`([invalid`); err == nil {
		t.Error("invalid pattern must return error")
	}
}

// TestCachedRegexpConcurrent — เรียกพร้อมกันหลาย goroutine ต้องปลอดภัย
func TestCachedRegexpConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			re, err := cachedRegexp(`\sattr="[^"]*"`)
			if err != nil || re == nil {
				t.Errorf("concurrent compile: %v", err)
			}
			_ = re.ReplaceAllString(`<a attr="x">`, "")
		}()
	}
	wg.Wait()
}
