package ptr

import (
	"time"

	uuid "github.com/satori/go.uuid"
)

// FromUUID returns pointer to uuid
func FromUUID(u uuid.UUID) *uuid.UUID {
	return &u
}

// FromString returns pointer to string
func FromString(s string) *string {
	return &s
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
	return &t
}

// From gets the address of type [T]
func From[T any](t T) *T {
	return &t
}
