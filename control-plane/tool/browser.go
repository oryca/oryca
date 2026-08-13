package tool

import "strings"

// ParseBrowser extracts a human-readable browser name from a User-Agent string.
func ParseBrowser(ua string) string {
	if ua == "" {
		return ""
	}
	switch {
	case strings.Contains(ua, "Edg/") || strings.Contains(ua, "Edge/"):
		return "Edge"
	case strings.Contains(ua, "OPR/") || strings.Contains(ua, "Opera"):
		return "Opera"
	case strings.Contains(ua, "Chrome/") && strings.Contains(ua, "Safari/"):
		return "Chrome"
	case strings.Contains(ua, "Firefox/"):
		return "Firefox"
	case strings.Contains(ua, "Safari/") && strings.Contains(ua, "Version/"):
		return "Safari"
	case strings.Contains(ua, "curl/"):
		return "curl"
	case strings.Contains(ua, "PostmanRuntime"):
		return "Postman"
	default:
		return "Other"
	}
}
