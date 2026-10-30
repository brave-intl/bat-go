package ptr

import (
	"time"
)

// FromString returns pointer to string
func FromString(s string) *string {
	return new(s)
}

// String returns value of pointer or empty string
func String(s *string) string {
	return StringOr(s, "")
}

// StringOr returns value of pointer or alternative value
func StringOr(s *string, or string) string {
	if s == nil {
		return or
	}
	return *s
}

// FromTime - get the address of the time
func FromTime(t time.Time) *time.Time {
	return new(t)
}

func To[T any](v T) *T {
	return new(v)
}
