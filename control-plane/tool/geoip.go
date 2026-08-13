package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type geoIPResponse struct {
	Status      string `json:"status"`
	City        string `json:"city"`
	RegionName  string `json:"regionName"`
	CountryCode string `json:"countryCode"`
}

// LookupLocation returns "City, Region, CountryCode" for the given IP address.
// Returns empty string on failure (best-effort, never blocks the caller for long).
func LookupLocation(ctx context.Context, ip string) string {
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return ""
	}

	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "http://ip-api.com/json/"+ip+"?fields=status,city,regionName,countryCode", nil)
	if err != nil {
		return ""
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var geo geoIPResponse
	if err := json.NewDecoder(resp.Body).Decode(&geo); err != nil || geo.Status != "success" {
		return ""
	}

	parts := make([]string, 0, 3)
	if geo.City != "" {
		parts = append(parts, geo.City)
	}
	if geo.RegionName != "" && geo.RegionName != geo.City {
		parts = append(parts, geo.RegionName)
	}
	if geo.CountryCode != "" {
		parts = append(parts, geo.CountryCode)
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
