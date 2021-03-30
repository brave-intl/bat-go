package inputs

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrTimeDecodeNotValid - an error that tells caller the id is not a uuid
	ErrTimeDecodeNotValid = errors.New("failed to decode timestamp: time is not a valid timestamp")
	// ErrTimeDecodeEmpty - an error that tells caller the id is empty and should not be
	ErrTimeDecodeEmpty = errors.New("failed to decode timestamp: timestamp cannot be empty")
)

// ID - a generic ID type that can be used for common id based things
type Time struct {
	time   *time.Time
	raw    string
	layout string
}

func NewTime(layout string) *Time {
	t := new(Time)
	t.layout = layout
	return t
}

// UUID - return the UUID representation of the ID
func (id *Time) Time() *time.Time {
	return id.time
}

// String - return the String representation of the ID
func (id *Time) String() string {
	return id.raw
}

// Validate - take raw []byte input and populate id with the ID
func (id *Time) Validate(ctx context.Context) error {
	// this should be overloaded to validate ids are real...
	return nil
}

// Decode - take raw []byte input and populate id with the ID
func (id *Time) Decode(ctx context.Context, input []byte) error {
	var err error

	if len(input) == 0 {
		return ErrTimeDecodeEmpty
	}
	id.raw = string(input)

	var parsed time.Time
	if parsed, err = time.Parse(id.layout, id.raw); err != nil {
		return ErrTimeDecodeNotValid
	}
	id.time = &parsed
	return nil
}
