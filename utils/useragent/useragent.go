package useragent

import (
	"strings"

	"github.com/mssola/user_agent"
)

var (
	checks = [][]string{
		{"iphone", "ios"},
		{"android", "android"},
		{"windows", "windows"},
		{"mac os x", "osx"},
		{"linux", "linux"},
	}
)

// ParsePlatform parses a platform known to grants from ua
func ParsePlatform(ua string) string {
	if ua == "" {
		return ""
	}
	parsed := user_agent.New(ua)
	if parsed == nil {
		return ""
	}
	os := strings.ToLower(parsed.OS())
	for _, check := range checks {
		if strings.Contains(os, check[0]) {
			return check[1]
		}
	}
	return ""
}
