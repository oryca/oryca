package transform

import (
	"regexp"
	"sync"
)

// regexCache cache compiled regex ตาม pattern string — compile ครั้งเดียวต่อ pattern
// *regexp.Regexp ใช้ข้าม goroutine ได้ปลอดภัย; pattern มาจาก transform config จึงมีจำนวนจำกัด
var regexCache sync.Map

// cachedRegexp คืน compiled regex จาก cache หรือ compile ใหม่ถ้ายังไม่มี
func cachedRegexp(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	actual, _ := regexCache.LoadOrStore(pattern, re)
	return actual.(*regexp.Regexp), nil
}
