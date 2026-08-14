package transform

import (
	"regexp"
	"sync"
)

// regexCache holds compiled regexes keyed by their pattern — one compile per pattern.
// *regexp.Regexp is safe to share across goroutines, and patterns come from the transform
// config, so the set stays small.
var regexCache sync.Map

// cachedRegexp returns the compiled regex, compiling it on first use
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
