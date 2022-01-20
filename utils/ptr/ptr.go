package ptr

import uuid "github.com/satori/go.uuid"

// FromUUID returns pointer to uuid
func FromUUID(u uuid.UUID) *uuid.UUID {
	return &u
}

// FromString returns pointer to string
func FromString(s string) *string {
	return &s
}
