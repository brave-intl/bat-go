package inputs

import (
	"context"
	"errors"

	uuid "github.com/satori/go.uuid"
)

var (
	// ErrIDDecodeNotUUID - an error that tells caller the id is not a uuid
	ErrIDDecodeNotUUID = errors.New("failed to decode id: id is not a uuid")
	// ErrIDDecodeEmpty - an error that tells caller the id is empty and should not be
	ErrIDDecodeEmpty = errors.New("failed to decode id: id cannot be empty")
)

// ID - a generic ID type that can be used for common id based things
type ID struct {
	uuid *uuid.UUID
	raw  string
}

// UUID - return the UUID representation of the ID
func (id *ID) UUID() *uuid.UUID {
	return id.uuid
}

// String - return the String representation of the ID
func (id *ID) String() string {
	return id.raw
}

// Validate - take raw []byte input and populate id with the ID
func (id *ID) Validate(ctx context.Context) error {
	// this should be overloaded to validate ids are real...
	return nil
}

// Decode - take raw []byte input and populate id with the ID
func (id *ID) Decode(ctx context.Context, input []byte) error {
	var err error

	if len(input) == 0 {
		return ErrIDDecodeEmpty
	}
	id.raw = string(input)

	var parsed uuid.UUID
	if parsed, err = uuid.FromString(id.raw); err != nil {
		return ErrIDDecodeNotUUID
	}
	id.uuid = &parsed
	return nil
}
