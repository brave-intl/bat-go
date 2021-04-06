package stringutils

import "strings"

// SplitAndTrim splits a string along a delimiter and trims it
func SplitAndTrim(base string, options ...string) []string {
	delimiter := ","
	if len(options) > 0 {
		delimiter = options[0]
	}
	split := strings.Split(base, delimiter)
	splitAndTrimmed := []string{}
	for _, s := range split {
		if s == "" {
			continue
		}
		splitAndTrimmed = append(
			splitAndTrimmed,
			strings.TrimSpace(s),
		)
	}
	return splitAndTrimmed
}
