package useragent

import (
	"strings"

	"github.com/mssola/user_agent"
)

var (
	checks = [][]string{
		[]string{"iphone", "ios"},
		[]string{"android", "android"},
		[]string{"windows", "windows"},
		[]string{"mac os x", "osx"},
		[]string{"linux", "linux"},
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
