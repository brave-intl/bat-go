package time

import (
	"strings"
	"time"
)

// JustDate returns just the date in format "2021-01-01"
func JustDate(t time.Time) string {
	return strings.Split(t.Format(time.RFC3339), "T")[0]
}
