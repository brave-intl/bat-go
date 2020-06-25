package useragent

import (
	"github.com/varstr/uaparser"
)

// ParsePlatform parses a platform known to grants from ua
func ParsePlatform(ua string) string {
	if ua == "" {
		return ""
	}
	parsed := uaparser.Parse(ua)
	if parsed == nil {
		return ""
	}
	// {"ios", "android", "osx", "windows", "linux", "desktop"}
	switch parsed.OS.Name {
	case "iOS":
		return "ios"
	case "Android":
		return "android"
	case "Windows":
		return "windows"
	case "Linux":
		return "linux"
	case "Mac OS":
		return "osx"
	}
	return "desktop"
}
