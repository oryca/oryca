package handler

import (
	"fmt"
	"github.com/oryca/oryca/gateway/cache"
	"github.com/oryca/oryca/gateway/model"
	"net/http"
	"strings"
	"time"
)

func extractBearer(req *http.Request) string {
	if auth := req.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return req.URL.Query().Get("token")
}

func resolveAuthToken(req *http.Request) string {
	if v := extractApiKey(req); v != "" {
		return v
	}
	return extractBearer(req)
}

func resolveAuthType(req *http.Request) string {
	if extractApiKey(req) != "" {
		return "api_key"
	}
	if extractBearer(req) != "" {
		return "bearer"
	}
	return ""
}

func extractApiKey(req *http.Request) string {
	if v := req.Header.Get("X-Api-Key"); v != "" {
		return v
	}
	return req.URL.Query().Get("api_key")
}

func checkApiKeyRestriction(ak *model.ApiKey, req *http.Request, clientIP string) error {
	if ak.Restriction == nil || ak.Restriction.Type == "none" || ak.Restriction.Type == "" {
		return nil
	}
	switch ak.Restriction.Type {
	case "ip_address":
		for _, ip := range ak.Restriction.Items {
			if ip == clientIP {
				return nil
			}
		}
		return fmt.Errorf("IP not allowed")
	case "http_referer":
		referer := req.Header.Get("Referer")
		for _, r := range ak.Restriction.Items {
			if strings.Contains(referer, r) {
				return nil
			}
		}
		return fmt.Errorf("referer not allowed")
	}
	return nil
}

func timeNow() time.Time {
	return time.Now().UTC()
}

// toRateLimitTiers แปลง RateLimit map จาก resource ให้เป็น tiers ของ packageID ที่ระบุ
// คืน nil ถ้าไม่มี rate limit config สำหรับ package นั้น
func toRateLimitTiers(rateLimitMap map[string][]model.RateLimitTier, packageID string) []cache.RateLimitTier {
	tiers, ok := rateLimitMap[packageID]
	if !ok || len(tiers) == 0 {
		return nil
	}
	result := make([]cache.RateLimitTier, len(tiers))
	for i, t := range tiers {
		result[i] = cache.RateLimitTier{Limit: t.Limit, WindowSec: t.WindowSec}
	}
	return result
}
