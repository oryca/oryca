package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBrowser(t *testing.T) {
	tests := []struct {
		name     string
		ua       string
		expected string
	}{
		{"ว่างเปล่า คืน empty", "", ""},
		{"Chrome", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", "Chrome"},
		{"Firefox", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0", "Firefox"},
		{"Safari", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", "Safari"},
		{"Edge", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0", "Edge"},
		{"Opera", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 OPR/106.0.0.0", "Opera"},
		{"curl", "curl/8.1.2", "curl"},
		{"Postman", "PostmanRuntime/7.35.0", "Postman"},
		{"ไม่รู้จัก คืน Other", "SomeUnknownBot/1.0", "Other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ParseBrowser(tt.ua))
		})
	}
}
