package tool

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"strings"
)

const apiKeyLength = 32 // 32 bytes = 64 hex chars

// HashApiKey คืน SHA-256 hex ของ raw key ใช้เป็น Redis lookup key
func HashApiKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// GenerateApiKey สร้าง API key แบบ random 64 ตัวอักษร (hex)
func GenerateApiKey() (string, error) {
	b := make([]byte, apiKeyLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// MaskApiKey ซ่อนกลางของ API key โดยแสดงเฉพาะ 4 ตัวแรกและ 4 ตัวท้าย
func MaskApiKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// IsIPAllowed ตรวจสอบว่า clientIP อยู่ใน allowedIPs หรือไม่ รองรับ CIDR notation
func IsIPAllowed(clientIP string, allowedIPs []string) bool {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	for _, allowed := range allowedIPs {
		if strings.Contains(allowed, "/") {
			_, ipNet, err := net.ParseCIDR(allowed)
			if err == nil && ipNet.Contains(ip) {
				return true
			}
		} else {
			if clientIP == allowed {
				return true
			}
		}
	}
	return false
}

// IsRefererAllowed ตรวจสอบว่า referer อยู่ใน allowedReferers หรือไม่
// รองรับ: full URL, domain-only, wildcard subdomain (*.example.com), wildcard path (/path/*)
func IsRefererAllowed(referer string, allowedReferers []string) bool {
	parsedRef, err := url.Parse(referer)
	if err != nil {
		return false
	}

	for _, allowed := range allowedReferers {
		if matchReferer(parsedRef, allowed) {
			return true
		}
	}
	return false
}

func matchReferer(ref *url.URL, allowed string) bool {
	if strings.HasPrefix(allowed, "http://") || strings.HasPrefix(allowed, "https://") {
		return matchFullURL(ref, allowed)
	}

	// wildcard subdomain: *.example.com
	if strings.HasPrefix(allowed, "*.") {
		domain := allowed[2:]
		return strings.HasSuffix(ref.Hostname(), "."+domain)
	}

	// domain-only
	return allowed == ref.Hostname()
}

func matchFullURL(ref *url.URL, allowed string) bool {
	parsedAllowed, err := url.Parse(allowed)
	if err != nil {
		return false
	}

	if parsedAllowed.Hostname() != ref.Hostname() {
		return false
	}
	if parsedAllowed.Scheme != "" && parsedAllowed.Scheme != ref.Scheme {
		return false
	}

	return matchPath(ref.Path, parsedAllowed.Path)
}

func matchPath(refPath, allowedPath string) bool {
	if strings.HasSuffix(allowedPath, "/*") {
		base := strings.TrimSuffix(allowedPath, "/*")
		return strings.HasPrefix(refPath, base)
	}
	return allowedPath == "" || allowedPath == "/" || allowedPath == refPath
}
